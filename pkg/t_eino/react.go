package t_eino

import (
	"context"
	"errors"
	"github.com/cloudwego/eino/flow/agent/react"
	"io"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/schema"
)

type state struct {
	Messages                 []*schema.Message
	ReturnDirectlyToolCallID string
	toolCallIDMap            map[string]string //tool_call_id映射对应的tool_name
	lock                     sync.RWMutex
}

func init() {
	schema.RegisterName[*state]("_my_eino_react_state")
}

const (
	nodeKeyTools = "tools"
	nodeKeyModel = "chat"
)

var (
	StopRunErr = errors.New("stop run") //特殊错误，用于直接终止运行图
)

// MessageModifier modify the input messages before the model is called.
type MessageModifier func(ctx context.Context, input []*schema.Message) []*schema.Message

// AgentConfig is the config for ReAct t_eino.
type AgentConfig struct {
	// ToolCallingModel is the chat model to be used for handling user messages with tool calling capability.
	// This is the recommended model field to use.
	ToolCallingModel model.ToolCallingChatModel

	ToolsConfig compose.ToolsNodeConfig

	// MessageModifier.
	// modify the input messages before the model is called, it's useful when you want to add some system prompt or other messages.
	MessageModifier MessageModifier

	// MessageRewriter modifies message in the state, before the ChatModel is called.
	// It takes the messages stored accumulated in state, modify them, and put the modified version back into state.
	// Useful for compressing message history to fit the model context window,
	// or if you want to make changes to messages that take effect across multiple model calls.
	// NOTE: if both MessageModifier and MessageRewriter are set, MessageRewriter will be called before MessageModifier.
	MessageRewriter MessageModifier

	// MaxStep.
	// default 12 of steps in pregel (node num + 10).
	MaxStep int `json:"max_step"`

	// Tools that will make t_eino return directly when the tool is called.
	// When multiple tools are called and more than one tool is in the return directly list, only the first one will be returned.
	ToolReturnDirectly map[string]struct{}

	// StreamOutputHandler is a function to determine whether the model's streaming output contains tool calls.
	// Different models have different ways of outputting tool calls in streaming mode:
	// - Some models (like OpenAI) output tool calls directly
	// - Others (like Claude) output text first, then tool calls
	// This handler allows custom logic to check for tool calls in the stream.
	// It should return:
	// - true if the output contains tool calls and t_eino should continue processing
	// - false if no tool calls and t_eino should stop
	// Note: This field only needs to be configured when using streaming mode
	// Note: The handler MUST close the modelOutput stream before returning
	// Optional. By default, it checks if the first chunk contains tool calls.
	// Note: The default implementation does not work well with Claude, which typically outputs tool calls after text content.
	// Note: If your ChatModel doesn't output tool calls first, you can try adding prompts to constrain the model from generating extra text during the tool call.
	StreamToolCallChecker func(ctx context.Context, modelOutput *schema.StreamReader[*schema.Message]) (bool, error)

	// GraphName is the graph name of the ReAct Agent.
	// Optional. Default `ReActAgent`.
	GraphName string
	// ModelNodeName is the node name of the model node in the ReAct Agent graph.
	// Optional. Default `ChatModel`.
	ModelNodeName string
	// ToolsNodeName is the node name of the tools node in the ReAct Agent graph.
	// Optional. Default `Tools`.
	ToolsNodeName string
}

func NormalStop(err error) bool {
	return errors.As(err, &io.EOF) || errors.As(err, &StopRunErr)
}

func getToolName(ctx context.Context, toolCallID string) string {
	var name string
	compose.ProcessState[*state](ctx, func(ctx context.Context, s *state) error {
		s.lock.RLock()
		defer s.lock.RUnlock()
		name = s.toolCallIDMap[toolCallID]
		return nil
	})
	return name
}

func checkChunkStreamToolCallChecker(_ context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
	defer sr.Close()
	for {
		msg, err := sr.Recv()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		if len(msg.ToolCalls) > 0 {
			return true, nil
		}
	}

	return false, nil
}

const (
	GraphName     = "ReActAgent"
	ModelNodeName = "ChatModel"
	ToolsNodeName = "Tools"
)

// SetReturnDirectly is a helper function that can be called within a tool's execution.
// It signals the ReAct t_eino to stop further processing and return the result of the current tool call directly.
// This is useful when the tool's output is the final answer and no more steps are needed.
// Note: If multiple tools call this function in the same step, only the last call will take effect.
// This setting has a higher priority than the AgentConfig.ToolReturnDirectly.
func SetReturnDirectly(ctx context.Context) error {
	return compose.ProcessState(ctx, func(ctx context.Context, s *state) error {
		s.ReturnDirectlyToolCallID = compose.GetToolCallID(ctx)
		return nil
	})
}

// Agent is the ReAct t_eino.
// ReAct t_eino is a simple t_eino that handles user messages with a chat model and tools.
// ReAct will call the chat model, if the message contains tool calls, it will call the tools.
// if the tool is configured to return directly, ReAct will return directly.
// otherwise, ReAct will continue to call the chat model until the message contains no tool calls.
// e.g.
//
//	t_eino, err := ReAct.NewAgent(ctx, &t_eino.AgentConfig{})
//	if err != nil {...}
//	msg, err := t_eino.Generate(ctx, []*agui_schema.Message{{Role: agui_schema.User, Content: "how to build t_eino with eino"}})
//	if err != nil {...}
//	println(msg.Content)
type Agent struct {
	toolList         *ToolList
	runnable         compose.Runnable[[]*schema.Message, *schema.Message]
	graph            *compose.Graph[[]*schema.Message, *schema.Message]
	graphAddNodeOpts []compose.GraphAddNodeOpt
}

// NewAgent creates a ReAct t_eino that feeds tool response into next round of Chat Model generation.
//
// IMPORTANT!! For models that don't output tool calls in the first streaming chunk (e.g. Claude)
// the default StreamToolCallChecker may not work properly since it only checks the first chunk for tool calls.
// In such cases, you need to implement a custom StreamToolCallChecker that can properly detect tool calls.
func NewAgent(ctx context.Context, config *AgentConfig) (_ *Agent, err error) {
	graph, t, opts, err := GetReactGraph(ctx, config)
	if err != nil {
		return nil, err
	}
	runnable, err := graph.Compile(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &Agent{
		toolList:         t,
		runnable:         runnable,
		graph:            graph,
		graphAddNodeOpts: []compose.GraphAddNodeOpt{compose.WithGraphCompileOptions(opts...)},
	}, nil
}

func GetReactGraph(ctx context.Context, config *AgentConfig) (graph *compose.Graph[[]*schema.Message, *schema.Message], t *ToolList, opts []compose.GraphCompileOption, err error) {
	var (
		chatModel       model.BaseChatModel
		toolsNode       *compose.ToolsNode
		toolsConfig     compose.ToolsNodeConfig
		toolCallChecker = config.StreamToolCallChecker
		messageModifier = config.MessageModifier
	)

	graphName := GraphName
	if config.GraphName != "" {
		graphName = config.GraphName
	}

	modelNodeName := ModelNodeName
	if config.ModelNodeName != "" {
		modelNodeName = config.ModelNodeName
	}

	toolsNodeName := ToolsNodeName
	if config.ToolsNodeName != "" {
		toolsNodeName = config.ToolsNodeName
	}

	if toolCallChecker == nil {
		toolCallChecker = checkChunkStreamToolCallChecker
	}

	chatModel = NewLearnModel(ctx, config.ToolCallingModel)
	t, err = NewToolList(ctx, config.ToolsConfig.Tools, chatModel)
	if err != nil {
		return nil, nil, nil, err
	}

	if toolsNode, err = compose.NewToolNode(ctx, &toolsConfig); err != nil {
		return nil, nil, nil, err
	}

	graph = compose.NewGraph[[]*schema.Message, *schema.Message](compose.WithGenLocalState(func(ctx context.Context) *state {
		return &state{Messages: make([]*schema.Message, 0, config.MaxStep+1)}
	}))

	modelPreHandle := func(ctx context.Context, input []*schema.Message, state *state) ([]*schema.Message, error) {
		state.Messages = append(state.Messages, input...)

		if config.MessageRewriter != nil {
			state.Messages = config.MessageRewriter(ctx, state.Messages)
		}

		if messageModifier == nil {
			return state.Messages, nil
		}

		modifiedInput := make([]*schema.Message, len(state.Messages))
		copy(modifiedInput, state.Messages)
		return messageModifier(ctx, modifiedInput), nil
	}

	if err = graph.AddChatModelNode(nodeKeyModel, chatModel, compose.WithStatePreHandler(modelPreHandle), compose.WithNodeName(modelNodeName)); err != nil {
		return nil, nil, nil, err
	}

	if err = graph.AddEdge(compose.START, nodeKeyModel); err != nil {
		return nil, nil, nil, err
	}

	toolsNodePreHandle := func(ctx context.Context, input *schema.Message, state *state) (*schema.Message, error) {
		state.lock.Lock()
		defer state.lock.Unlock()
		if input == nil {
			return state.Messages[len(state.Messages)-1], nil // used for rerun interrupt resume
		}
		if state.toolCallIDMap == nil {
			state.toolCallIDMap = make(map[string]string)
		}
		for _, toolCall := range input.ToolCalls {
			state.toolCallIDMap[toolCall.ID] = toolCall.Function.Name
		}
		state.Messages = append(state.Messages, input)
		state.ReturnDirectlyToolCallID = getReturnDirectlyToolCallID(input, config.ToolReturnDirectly)
		return input, nil
	}
	if err = graph.AddToolsNode(nodeKeyTools, toolsNode, compose.WithStatePreHandler(toolsNodePreHandle), compose.WithNodeName(toolsNodeName)); err != nil {
		return nil, nil, nil, err
	}

	modelPostBranchCondition := func(ctx context.Context, sr *schema.StreamReader[*schema.Message]) (endNode string, err error) {
		if isToolCall, err := toolCallChecker(ctx, sr); err != nil {
			return "", err
		} else if isToolCall {
			return nodeKeyTools, nil
		}
		return compose.END, nil
	}

	if err = graph.AddBranch(nodeKeyModel, compose.NewStreamGraphBranch(modelPostBranchCondition, map[string]bool{nodeKeyTools: true, compose.END: true})); err != nil {
		return nil, nil, nil, err
	}

	if err = buildReturnDirectly(graph); err != nil {
		return nil, nil, nil, err
	}

	opts = []compose.GraphCompileOption{compose.WithMaxRunSteps(config.MaxStep), compose.WithNodeTriggerMode(compose.AnyPredecessor), compose.WithGraphName(graphName)}
	return
}

func buildReturnDirectly(graph *compose.Graph[[]*schema.Message, *schema.Message]) (err error) {
	directReturn := func(ctx context.Context, msgs *schema.StreamReader[[]*schema.Message]) (*schema.StreamReader[*schema.Message], error) {
		return schema.StreamReaderWithConvert(msgs, func(msgs []*schema.Message) (*schema.Message, error) {
			var msg *schema.Message
			err = compose.ProcessState[*state](ctx, func(_ context.Context, state *state) error {
				for i := range msgs {
					if msgs[i] != nil && msgs[i].ToolCallID == state.ReturnDirectlyToolCallID {
						msg = msgs[i]
						return nil
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			if msg == nil {
				return nil, schema.ErrNoValue
			}
			return msg, nil
		}), nil
	}

	nodeKeyDirectReturn := "direct_return"
	if err = graph.AddLambdaNode(nodeKeyDirectReturn, compose.TransformableLambda(directReturn)); err != nil {
		return err
	}

	// this branch checks if the tool called should return directly. It either leads to END or back to ChatModel
	err = graph.AddBranch(nodeKeyTools, compose.NewStreamGraphBranch(func(ctx context.Context, msgsStream *schema.StreamReader[[]*schema.Message]) (endNode string, err error) {
		msgsStream.Close()

		err = compose.ProcessState[*state](ctx, func(_ context.Context, state *state) error {
			if len(state.ReturnDirectlyToolCallID) > 0 {
				endNode = nodeKeyDirectReturn
			} else {
				endNode = nodeKeyModel
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		return endNode, nil
	}, map[string]bool{nodeKeyModel: true, nodeKeyDirectReturn: true}))
	if err != nil {
		return err
	}

	return graph.AddEdge(nodeKeyDirectReturn, compose.END)
}

func genToolInfos(ctx context.Context, config compose.ToolsNodeConfig) ([]*schema.ToolInfo, error) {
	toolInfos := make([]*schema.ToolInfo, 0, len(config.Tools))
	for _, t := range config.Tools {
		tl, err := t.Info(ctx)
		if err != nil {
			return nil, err
		}

		toolInfos = append(toolInfos, tl)
	}

	return toolInfos, nil
}

func getReturnDirectlyToolCallID(input *schema.Message, toolReturnDirectly map[string]struct{}) string {
	if len(toolReturnDirectly) == 0 {
		return ""
	}

	for _, toolCall := range input.ToolCalls {
		if _, ok := toolReturnDirectly[toolCall.Function.Name]; ok {
			return toolCall.ID
		}
	}

	return ""
}

// Generate generates a response from the t_eino.
func (r *Agent) Generate(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.Message, error) {
	r.toolList.Init()
	option, err := r.getAgentOption(opts...)
	if err != nil {
		return nil, err
	}
	return r.runnable.Invoke(ctx, input, agent.GetComposeOptions(option...)...)
}

// Stream calls the t_eino and returns a stream response.
func (r *Agent) Stream(ctx context.Context, input []*schema.Message, options ...Option) (output *react.Iterator[*schema.StreamReader[*schema.Message]], err error) {
	//return r.runnable.Stream(ctx, input, agent.GetComposeOptions(opts...)...)
	r.toolList.Init()
	opts, err := r.getAgentOption(options...)
	if err != nil {
		return nil, err
	}
	opt, mf := react.WithMessageFuture()
	opts = append(opts, opt)
	go r.runnable.Stream(ctx, input, agent.GetComposeOptions(opts...)...)
	output = mf.GetMessageStreams()
	return
}

// ExportGraph exports the underlying graph from Agent, along with the []compose.GraphAddNodeOpt to be used when adding this graph to another graph.
func (r *Agent) ExportGraph() (compose.AnyGraph, []compose.GraphAddNodeOpt) {
	return r.graph, r.graphAddNodeOpts
}

func (a *Agent) getAgentOption(options ...Option) ([]agent.AgentOption, error) {
	agentOpts := make([]agent.AgentOption, len(options))
	for _, o := range options {
		ao, err := o(a)
		if err != nil {
			return nil, err
		}
		agentOpts = append(agentOpts, ao...)
	}
	return agentOpts, nil
}
