package agent

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

type AgentResumeOptions struct {
	f *factory.Factory
}

func NewCmdAgentResume(f *factory.Factory) *cobra.Command {
	options := AgentResumeOptions{
		f: f,
	}

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "resume <agent-id>",
		Args:                  cobra.ExactArgs(1),
		Short:                 "Resume a Buildkite agent",
		Long: heredoc.Doc(`
			Resume a paused Buildkite agent.

			When an agent is resumed, it will start accepting new jobs again.
		`),
		Example: heredoc.Doc(`
			# Resume an agent
			$ bk agent resume 0198d108-a532-4a62-9bd7-b2e744bf5c45
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunResume(cmd, args, &options)
		},
	}

	return &cmd
}

func RunResume(cmd *cobra.Command, args []string, opts *AgentResumeOptions) error {
	agentID := args[0]

	_, err := opts.f.RestAPIClient.Agents.Resume(cmd.Context(), opts.f.Config.OrganizationSlug(), agentID)
	if err != nil {
		return fmt.Errorf("failed to resume agent: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Agent %s resumed successfully\n", agentID)
	return nil
}
