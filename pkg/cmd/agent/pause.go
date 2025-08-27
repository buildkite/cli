package agent

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/cobra"
)

type AgentPauseOptions struct {
	note             string
	timeoutInMinutes int
	f                *factory.Factory
}

func NewCmdAgentPause(f *factory.Factory) *cobra.Command {
	options := AgentPauseOptions{
		f: f,
	}

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "pause <agent-id>",
		Args:                  cobra.ExactArgs(1),
		Short:                 "Pause a Buildkite agent",
		Long: heredoc.Doc(`
			Pause a Buildkite agent with an optional note and timeout.

			When an agent is paused, it will stop accepting new jobs but will continue
			running any jobs it has already started. You can optionally provide a note
			explaining why the agent is being paused and set a timeout for automatic resumption.
		`),
		Example: heredoc.Doc(`
			# Pause an agent
			$ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45

			# Pause an agent with a note
			$ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45 --note "Maintenance scheduled"

			# Pause an agent with a note and 60 minute timeout
			$ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45 --note "too many llamas" --timeout-in-minutes 60
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunPause(cmd, args, &options)
		},
	}

	cmd.Flags().StringVar(&options.note, "note", "", "A descriptive note to record why the agent is paused")
	cmd.Flags().IntVar(&options.timeoutInMinutes, "timeout-in-minutes", 0, "Timeout after which the agent is automatically resumed, in minutes (default: 0 = no timeout)")

	return &cmd
}

func RunPause(cmd *cobra.Command, args []string, opts *AgentPauseOptions) error {
	agentID := args[0]

	var pauseOpts *buildkite.AgentPauseOptions
	if opts.note != "" || opts.timeoutInMinutes > 0 {
		pauseOpts = &buildkite.AgentPauseOptions{
			Note:             opts.note,
			TimeoutInMinutes: opts.timeoutInMinutes,
		}
	}

	_, err := opts.f.RestAPIClient.Agents.Pause(cmd.Context(), opts.f.Config.OrganizationSlug(), agentID, pauseOpts)
	if err != nil {
		return fmt.Errorf("failed to pause agent: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Agent %s paused successfully\n", agentID)
	return nil
}
