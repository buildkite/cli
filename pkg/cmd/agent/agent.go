package agent

import (
	"errors"
	"net/url"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// CheckValidConfiguration returns a function that checks the viper configuration is valid to execute the command
func CheckValidConfiguration(v *viper.Viper) func(cmd *cobra.Command, args []string) error {
	var err error

	// ensure the configuration has an API token set
	if !v.IsSet(config.APITokenConfigKey) {
		err = errors.New("You must set a valid API token. Run `bk configure`.")
	}

	return func(cmd *cobra.Command, args []string) error {
		return err
	}
}

func NewCmdAgent(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "agent <command>",
		Short: "Manage agents",
		Long:  "Work with Buildkite agents.",
		Example: heredoc.Doc(`
			$ bk agent stop buildkite/018a2b90-ba7f-4220-94ca-4903fa0ba410
		`),
		PersistentPreRunE: CheckValidConfiguration(f.Config),
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

func parseAgentArg(agent string, v *viper.Viper) (string, string) {
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
			org = v.GetString(config.OrganizationSlugConfigKey)
			id = agent
		}
	}

	return org, id
}
