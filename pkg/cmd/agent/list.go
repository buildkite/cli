package agent

import (
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func NewCmdAgentList(f *factory.Factory) *cobra.Command {
	var name, version, hostname string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list",
		Args:                  cobra.NoArgs,
		Short:                 "List agents",
		Long: heredoc.Doc(`
			List all connected agents for the current organization.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			model, err := agent.ObtainAgents(f, name, version, hostname)

			if err != nil {
				return err
			}

			p := tea.NewProgram(model)

			if _, err := p.Run(); err != nil {
				os.Exit(1)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter agents by their name")
	cmd.Flags().StringVar(&version, "version", "", "Filter agents by their agent version")
	cmd.Flags().StringVar(&hostname, "hostname", "", "Filter agents by their hostname")

	return &cmd
}
