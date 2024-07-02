package tools

import "github.com/gptscript-ai/gptscript/pkg/types"

func SummariseText() types.Tool {
	return types.Tool{
		ID: "summarise-text",
		ToolDef: types.ToolDef{
			Parameters: types.Parameters{
				Name:        "summarise-text",
				ModelName:   "gpt-4o",
				Description: "Summarise some given text",
				Arguments:   types.ObjectSchema("text", "The text the summarise"),
			},
			Instructions: "Summarise the given text. Include enough information and examples if provided.",
		},
	}
}

func SummariseURL() types.Tool {
	return types.Tool{
		ID: "summarise-url",
		ToolDef: types.ToolDef{
			Parameters: types.Parameters{
				Name:        "summarise-url",
				ModelName:   "gpt-4o",
				Description: "Summarise a web page",
				Arguments:   types.ObjectSchema("url", "The webpage URL to summarise"),
			},
			Instructions: "Summarise the page at the given URL. Include enough information to answer the users question.",
		},
	}
}
