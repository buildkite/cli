package agent

import (
	"net/url"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/spf13/cobra"
)

func NewCmdAgent(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "agent <command>",
		Short: "Manage agents",
		Long:  "Work with Buildkite agents.",
		Example: heredoc.Doc(`
			# To stop an agent
			$ bk agent stop buildkite/018a2b90-ba7f-4220-94ca-4903fa0ba410
			# To view agent details
			$ bk agent view 018a2b90-ba7f-4220-94ca-4903fa0ba410
		`),
		PersistentPreRunE: validation.CheckValidConfiguration(f.Config),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				An agent can be supplied as an argument in any of the following formats:
				- "ORGANIZATION_SLUG/UUID"
				- "UUID"
				- by URL, e.g. "https://buildkite.com/organizations/buildkite/agents/018a2b90-ba7f-4220-94ca-4903fa0ba410"
			`),
		},
	}

	cmd.AddCommand(NewCmdAgentList(f))
	cmd.AddCommand(NewCmdAgentStop(f))
	cmd.AddCommand(NewCmdAgentView(f))

	return &cmd
}

func parseAgentArg(agent string, conf *config.Config) (string, string) {
	var org, id string
	agentIsURL := strings.Contains(agent, ":")
	agentIsSlug := !agentIsURL && strings.Contains(agent, "/")

	if agentIsURL {
		url, err := url.Parse(agent)
		if err != nil {
			return "", ""
		}
		// eg: url.Path = organizations/buildkite/agents/018a2b90-ba7f-4220-94ca-4903fa0ba410
		// or for clustered agents, url.Path = organizations/buildkite/clusters/840b09eb-d325-482f-9ff4-0c3abf38560b/queues/fb85c9e4-5531-47a2-90f3-5540dc698811/agents/018c3d27-147b-4faa-94d0-f4c8ce613e5c
		part := strings.Split(url.Path, "/")
		if part[3] == "agents" {
			org, id = part[2], part[4]
		} else {
			org, id = part[2], part[len(part)-1]
		}
	} else {
		if agentIsSlug {
			part := strings.Split(agent, "/")
			org, id = part[0], part[1]
		} else {
			org = conf.Organization
			id = agent
		}
	}

	return org, id
}
