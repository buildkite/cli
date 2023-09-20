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
			$ bk agent stop buildkite/018a2b90-ba7f-4220-94ca-4903fa0ba410
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

	cmd.AddCommand(NewCmdAgentStop(f))
	cmd.AddCommand(NewCmdAgentList(f))
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
		part := strings.Split(url.Path, "/")
		org, id = part[2], part[4]
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
