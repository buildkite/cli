package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
)

type ChatCompleter interface {
	CreateChatCompletion(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

type CompletionHandler struct {
	Tools     EnabledTools
	Completer ChatCompleter
}

// Complete will send the given messages to OpenAI API to have it complete them. Returns the final string content from
// the API.
//
// This supports recursive calls to handle calling custom defined Tools and responding to AI again.
func (ch *CompletionHandler) Complete(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error) {
	req := openai.ChatCompletionRequest{
		Model:    openai.GPT4o,
		Messages: messages,
		Tools:    ch.Tools.GetDefinitions(),
	}
	resp, err := ch.Completer.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("received no response choices from open ai")
	}
	switch message := resp.Choices[0].Message; resp.Choices[0].FinishReason {
	case openai.FinishReasonStop: // AI has said this is the last message, return the string content
		return message.Content, nil
	case openai.FinishReasonToolCalls: // AI wants to call a tool and return its response
		// append the current message and then call the custom tool to handle
		messages = append(messages, message)
		for _, toolCall := range message.ToolCalls {
			tool := ch.Tools.GetTool(toolCall.Function.Name)
			if tool == nil {
				return "", fmt.Errorf("ai returned function name that doesn't exist: %s", toolCall.Function.Name)
			}
			output, err := (*tool).Execute(toolCall.Function.Arguments)
			if err != nil {
				return "", err
			}

			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    strings.Join(output.([]string), "\n"),
			})
		}
		// recurse again to pass the tool output back to AI and generate a response
		return ch.Complete(ctx, messages)
	}

	// if we get here, something has gone wrong
	return "", fmt.Errorf("unknown finish reason from ai: %s", resp.Choices[0].FinishReason)
}
