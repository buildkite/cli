package ai

import (
	"errors"
	"io"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/ai"
	"github.com/buildkite/cli/v3/internal/algolia"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/charmbracelet/glamour"
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

			// build up the open ai request including our custom tools, system prompt and user input
			tools := ai.EnabledTools{
				&ai.DocumentationTool{Search: algolia.Search},
			}
			messages := []openai.ChatCompletionMessage{
				// {
				// 	Role:    openai.ChatMessageRoleSystem,
				// 	Content: "Respond in markdown format",
				// },
				{
					Role:    openai.ChatMessageRoleUser,
					Content: input,
				},
			}

			// now handle the ai chat until we get a string response we can output for the user
			handler := ai.CompletionHandler{
				Completer: f.OpenAIClient,
				Tools:     tools,
			}
			output, err := handler.Complete(cmd.Context(), messages)
			if err != nil {
				return err
			}

			// render output as markdown
			output, err = glamour.Render(output, glamour.AutoStyle)
			if err != nil {
				return err
			}

			_, err = cmd.OutOrStdout().Write([]byte(output))
			return err
		},
	}

	return &cmd
}
