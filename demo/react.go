package main

import (
	"context"
	"github.com/birdy/agent/pkg/t_eino"
	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func main() {
	ctx := context.Background()

	llm, err := ark.NewChatModel(context.Background(), &ark.ChatModelConfig{
		// Add Ark-specific configuration from environment variables
		APIKey:  "{APIKey}",
		Model:   "{Model}",
		BaseURL: "{BaseURL}",
	})

	exmapleTool, err := utils.InferTool("tool", "desc", func(ctx context.Context, input any) (output string, err error) {
		return "", nil
	})

	exmaple2Tool, err := utils.InferTool("tool", "desc", func(ctx context.Context, input any) (output string, err error) {
		return "", nil
	})

	if err != nil {
		return
	}

	reactAgent, err := t_eino.NewAgent(ctx, &t_eino.AgentConfig{
		ToolCallingModel: llm,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: []tool.BaseTool{exmapleTool}},
	})

	opt, err := t_eino.WithTools(ctx, exmaple2Tool)
	if err != nil {
		return
	}

	var input []*schema.Message

	reactAgent.Stream(ctx, input, opt)
	//下面就是输出
}
