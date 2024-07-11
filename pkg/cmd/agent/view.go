package agent

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/huh/spinner"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func NewCmdAgentView(f *factory.Factory) *cobra.Command {
	var web bool

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view <agent>",
		Args:                  cobra.ExactArgs(1),
		Short:                 "View details of an agent",
		Long: heredoc.Doc(`
			View details of an agent.

			If the "ORGANIZATION_SLUG/" portion of the "ORGANIZATION_SLUG/UUID" agent argument
			is omitted, it uses the currently selected organization.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, id := parseAgentArg(args[0], f.Config)

			if web {
				url := fmt.Sprintf("https://buildkite.com/organizations/%s/agents/%s", org, id)
				fmt.Printf("Opening %s in your browser\n", url)
				return browser.OpenURL(url)
			}

			var err error
			var agentData *buildkite.Agent
			spinErr := spinner.New().
				Title("Loading agent").
				Action(func() {
					agentData, _, err = f.RestAPIClient.Agents.Get(org, id)
				}).
				Run()
			if spinErr != nil {
				return spinErr
			}
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", agent.AgentDataTable(agentData))

			return err
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open agent in a browser")

	return &cmd
}
