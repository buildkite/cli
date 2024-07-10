package ai

import (
	"github.com/sashabaranov/go-openai"
)

type Tool interface {
	ToolDefinition() openai.Tool
	Execute(string) (any, error)
}

type EnabledTools []Tool

func (et EnabledTools) GetTool(name string) *Tool {
	for _, v := range et {
		if v.ToolDefinition().Function.Name == name {
			return &v
		}
	}

	return nil
}

func (et EnabledTools) GetDefinitions() []openai.Tool {
	tools := make([]openai.Tool, len(et))
	for i, v := range et {
		tools[i] = v.ToolDefinition()
	}

	return tools
}
