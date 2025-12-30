package t_eino

import (
	"context"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var _ model.ToolCallingChatModel = &LearnModel{}

type LearnToolFunc func([]*schema.ToolInfo)

type LearnModel struct {
	tChatModel        model.ToolCallingChatModel
	originalChatModel model.ToolCallingChatModel
}

func NewLearnModel(ctx context.Context, llm model.ToolCallingChatModel) *LearnModel {
	return &LearnModel{
		tChatModel:        llm,
		originalChatModel: llm,
	}
}

func (l *LearnModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return l.tChatModel.Generate(ctx, input, opts...)
}

func (l *LearnModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return l.tChatModel.Stream(ctx, input, opts...)
}

func (l *LearnModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	newModel, err := l.originalChatModel.WithTools(tools)
	if err != nil {
		return nil, err
	}
	l.tChatModel = newModel
	return l, err
}
