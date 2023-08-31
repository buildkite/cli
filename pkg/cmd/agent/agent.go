package agent

import (
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdAgent(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "agent <command>",
		Short: "Manage agents",
		Long:  "Work with Buildkite agents.",
		Example: heredoc.Doc(`
			$ bk agent stop buildkite/018a2b90-ba7f-4220-94ca-4903fa0ba410
		`),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				An agent can be supplied as an argument in any of the following formats:
				- "ORGANIZATION_SLUG/UUID"
				- "UUID"
				- by URL, e.g. "https://buildkite.com/organizations/buildkite/agents/018a2b90-ba7f-4220-94ca-4903fa0ba410"
			`),
		},
	}

	cmd.AddCommand(NewCmdAgentStop(f))

	return &cmd
}

func parseAgentArg(agent string) (string, string) {
	part := strings.Split(agent, "/")
	return part[0], part[1]
}
