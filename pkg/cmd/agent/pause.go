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

			The timeout must be between 1 and 1440 minutes (24 hours). If no timeout is
			specified, the agent will pause for 5 minutes by default.
		`),
		Example: heredoc.Doc(`
			# Pause an agent for 5 minutes (default)
			$ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45

			# Pause an agent with a note
			$ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45 --note "Maintenance scheduled"

			# Pause an agent with a note and 60 minute timeout
			$ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45 --note "too many llamas" --timeout-in-minutes 60

			# Pause for a short time (15 minutes) during deployment
			$ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45 --note "Deploy in progress" --timeout-in-minutes 15
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunPause(cmd, args, &options)
		},
	}

	cmd.Flags().StringVar(&options.note, "note", "", "A descriptive note to record why the agent is paused")
	cmd.Flags().IntVar(&options.timeoutInMinutes, "timeout-in-minutes", 5, "Timeout after which the agent is automatically resumed, in minutes (default: 5)")

	return &cmd
}

func RunPause(cmd *cobra.Command, args []string, opts *AgentPauseOptions) error {
	agentID := args[0]

	if opts.timeoutInMinutes <= 0 {
		return fmt.Errorf("timeout-in-minutes must be 1 or more")
	}
	if opts.timeoutInMinutes > 1440 {
		return fmt.Errorf("timeout-in-minutes cannot exceed 1440 minutes (1 day)")
	}

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

	message := fmt.Sprintf("Agent %s paused successfully", agentID)
	if opts.note != "" {
		message += fmt.Sprintf(" with note: %s", opts.note)
	}
	if opts.timeoutInMinutes > 0 {
		message += fmt.Sprintf(" (auto-resume in %d minutes)", opts.timeoutInMinutes)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", message)
	return nil
}
