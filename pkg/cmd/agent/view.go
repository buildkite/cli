package agent

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
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
			format, err := output.GetFormat(cmd.Flags())
			if err != nil {
				return err
			}

			org, id := parseAgentArg(args[0], f.Config)

			if web {
				url := fmt.Sprintf("https://buildkite.com/organizations/%s/agents/%s", org, id)
				fmt.Printf("Opening %s in your browser\n", url)
				return browser.OpenURL(url)
			}

			if err != nil {
				return err
			}

			if format != output.FormatText {
				var agentData buildkite.Agent
				spinErr := bk_io.SpinWhile(f, "Loading agent", func() {
					agentData, _, err = f.RestAPIClient.Agents.Get(cmd.Context(), org, id)
				})
				if spinErr != nil {
					return spinErr
				}
				if err != nil {
					return err
				}
				return output.Write(cmd.OutOrStdout(), agentData, format)
			}

			var agentData buildkite.Agent
			spinErr := bk_io.SpinWhile(f, "Loading agent", func() {
				agentData, _, err = f.RestAPIClient.Agents.Get(cmd.Context(), org, id)
			})
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

	output.AddFlags(cmd.Flags())
	return &cmd
}
