package ai

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/ai/tools"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/gptscript-ai/gptscript/pkg/builtin"
	"github.com/gptscript-ai/gptscript/pkg/gptscript"
	"github.com/gptscript-ai/gptscript/pkg/openai"
	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/spf13/cobra"
)

func NewCmdAIDocs(f *factory.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "docs <query>",
		Short: "Search the docs",
		Long:  "Search the Buildkite documentation from https://buildkite.com/docs",
		Example: heredoc.Doc(`
			# To configure your AI token
			$ bk docs "step dependencies"
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := gptscript.Options{
				OpenAI: openai.Options{
					APIKey: f.Config.GetOpenAIToken(),
				},
			}
			gpt, err := gptscript.New(opts)
			if err != nil {
				return err
			}
			defer gpt.Close(true)

			// NOTES: see assemble.go Header usage for "custom"
			// NOTES: prepend user input thing to tools before passing to below
			prg := types.Program{
				ToolSet: types.ToolSet{},
			}
			prg.ToolSet[tools.AlgoliaTool().Name] = tools.AlgoliaTool()
			prg.ToolSet["sys.http.html2text"], _ = builtin.Builtin("sys.http.html2text")
			prg.ToolSet["bk"] = types.Tool{
				ID: "bk",
				ToolDef: types.ToolDef{
					Parameters: types.Parameters{
						ModelName: "gpt-4o",
						Name:      "bk",
						Tools: []string{
							tools.AlgoliaTool().Name,
							"sys.http.html2text",
						},
					},
					Instructions: "Search the buildkite documentation website for results matching the users query. Parse the returned URL and use that content to answer the users question",
				},
				ToolMapping: map[string][]types.ToolReference{
					"algolia": {
						{
							Reference: "aloglia",
							ToolID:    "algolia",
						},
					},
					"sys.http.html2text": {
						{
							Reference: "sys.http.html2text",
							ToolID:    "sys.http.html2text",
						},
					},
				},
			}
			prg.EntryToolID = "bk"
			// prg needs to be populated with a toolset that combines all tools and the main entrypoint type thing
			// inner tools dont necessarily need their mappings, but the entrypoint and custom ones do
			if err != nil {
				return err
			}

			res, err := gpt.Run(cmd.Context(), prg, nil, args[0])
			if err != nil {
				return err
			}

			fmt.Println(res)

			return nil
		},
	}
}
