package t_eino

import (
	"context"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/schema"
)

type Option func(agent *Agent) ([]agent.AgentOption, error)

// 注入额外的tool，不会覆盖config的tools
func WithTools(ctx context.Context, tools ...tool.BaseTool) (Option, error) {
	m, err := toolsToMap(ctx, tools)
	if err != nil {
		return nil, err
	}

	o := func(a *Agent) ([]agent.AgentOption, error) {
		a.toolList.extraToolsMap = m
		tools := a.toolList.GetTools()
		toolInfos := make([]*schema.ToolInfo, 0, len(tools))
		for _, tl := range tools {
			info, err := tl.Info(ctx)
			if err != nil {
				return nil, err
			}
			toolInfos = append(toolInfos, info)
		}
		opts := make([]agent.AgentOption, 2)
		opts[0] = agent.WithComposeOptions(compose.WithChatModelOption(model.WithTools(toolInfos)))
		opts[1] = agent.WithComposeOptions(compose.WithToolsNodeOption(compose.WithToolList(tools...)))
		return opts, nil
	}

	return o, nil
}
