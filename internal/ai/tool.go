package ai

import "github.com/sashabaranov/go-openai"

type Tool interface {
	ToolDefinition() openai.Tool
	Execute(string) (any, error)
}
