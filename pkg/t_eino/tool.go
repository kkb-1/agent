package t_eino

import (
	"context"
	"fmt"
	utils2 "github.com/birdy/agent/utils"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

const (
	SpecialGetToolToolName = "special_get_tool"
)

// 全局 tool
type ToolList struct {
	chatModel     model.ToolCallingChatModel
	aliveTools    []tool.BaseTool //被大模型“看到”的工具列表
	originalTools map[string]tool.BaseTool
	aliveToolsMap map[string]tool.BaseTool
	extraToolsMap map[string]tool.BaseTool
}

type getToolArguments struct {
	Name string `json:"name"`
}

func toolsToMap(ctx context.Context, tools []tool.BaseTool) (map[string]tool.BaseTool, error) {
	m := make(map[string]tool.BaseTool, len(tools))
	for _, tool := range tools {
		info, err := tool.Info(ctx)
		if err != nil {
			return nil, err
		}
		m[info.Name] = tool
	}
	return m, nil
}

func NewToolList(ctx context.Context, originalTools []tool.BaseTool, chatModel model.ToolCallingChatModel) (*ToolList, error) {
	tm, err := toolsToMap(ctx, originalTools)
	if err != nil {
		return nil, err
	}
	t := &ToolList{
		chatModel:     chatModel,
		originalTools: tm,
	}
	specialTool, err := getSpecialTool(t)
	if err != nil {
		return nil, err
	}
	t.originalTools[SpecialGetToolToolName] = specialTool
	return t, nil
}

// 每次运行开始前初始化
func (t *ToolList) Init() {
	t.aliveTools = utils2.MapToSlice[tool.BaseTool](t.originalTools)
	t.aliveToolsMap = t.originalTools
}

func (t *ToolList) SetExtraTools(ctx context.Context, tools ...tool.BaseTool) error {
	tm, err := toolsToMap(ctx, tools)
	if err != nil {
		return err
	}
	t.extraToolsMap = tm
	return nil
}

func (t *ToolList) GetTools() []tool.BaseTool {
	return t.aliveTools
}

func (t *ToolList) GetToolByName(name string) (tool.BaseTool, bool) {
	if tool, ok := t.aliveToolsMap[name]; ok {
		return tool, true
	}

	if tool, ok := t.extraToolsMap[name]; ok {
		delete(t.extraToolsMap, name)
		t.aliveToolsMap[name] = tool
		t.aliveTools = append(t.aliveTools, tool)
		return tool, true
	}

	return nil, false
}

func getSpecialTool(t *ToolList) (tool.BaseTool, error) {
	inferTool, err := utils.InferTool(SpecialGetToolToolName, GetToolToolDescription, func(ctx context.Context, input getToolArguments) (output string, err error) {
		_, ok := t.GetToolByName(input.Name)
		if !ok {
			return "", fmt.Errorf("tool %s is not exist", input.Name)
		}
		tools := t.GetTools()
		infos := make([]*schema.ToolInfo, len(tools))
		for _, baseTool := range tools {
			info, err := baseTool.Info(ctx)
			if err != nil {
				return "", err
			}
			infos = append(infos, info)
		}
		_, err = t.chatModel.WithTools(infos)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("get tool %s success", input.Name), nil
	})
	if err != nil {
		return nil, err
	}
	return inferTool, nil
}
