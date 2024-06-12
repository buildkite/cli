package ai

import (
	"errors"
	"io"
	"strings"

	"github.com/MakeNowJust/heredoc"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
)

func NewCmdAIAsk(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "ask [prompt]",
		Short: "Ask Buildkite AI a question.",
		Long: heredoc.Doc(`
			AI for Buildkite. You can interact with AI to help solve errors in your builds or surface documentation.
		`),
		Args:    cobra.MaximumNArgs(1),
		PreRunE: validation.OpenAITokenConfigured(f.Config),
		Example: heredoc.Doc(`
			$ bk ai ask "How can I run a buildkite-agent on Ubuntu?"
			$ echo "Why did my job fail with exit code 255?" | bk ai ask
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			var input string
			if bk_io.HasDataAvailable(cmd.InOrStdin()) {
				stdin := new(strings.Builder)
				_, err := io.Copy(stdin, cmd.InOrStdin())
				if err != nil {
					return err
				}
				input = stdin.String()
			} else if len(args) >= 0 {
				input = args[0]
			} else {
				return errors.New("must supply a prompt")
			}

			client := *f.OpenAIClient
			resp, err := client.CreateChatCompletion(cmd.Context(), openai.ChatCompletionRequest{
				Model: openai.GPT3Dot5Turbo,
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleUser,
						Content: input,
					},
				},
			})
			if err != nil {
				return err
			}
			if len(resp.Choices) == 0 {
				return errors.New("no response received")
			}
			var msg strings.Builder
			for _, c := range resp.Choices {
				msg.WriteString(c.Message.Content)
				msg.WriteString("\n")
			}

			_, err = cmd.OutOrStdout().Write([]byte(msg.String()))
			return err
		},
	}

	return &cmd
}
