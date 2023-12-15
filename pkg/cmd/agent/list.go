package agent

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func NewCmdAgentList(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list",
		Args:                  cobra.NoArgs,
		Short:                 "List agents",
		Long: heredoc.Doc(`
			List all connected agents for the current organization.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			l := io.NewPendingCommand(func() tea.Msg {
				agents, _, err := f.RestAPIClient.Agents.List(f.Config.Organization, nil)
				if err != nil {
					return err
				}
				fmt.Printf("%+v\n", agents)
				return tea.Quit()
			}, "Loading agents")

			p := tea.NewProgram(l)
			_, err := p.Run()
			return err
		},
	}

	return &cmd
}
