package agent

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type PauseCmd struct {
	AgentID          string `arg:"" help:"Agent ID to pause"`
	Note             string `help:"A descriptive note to record why the agent is paused"`
	TimeoutInMinutes int    `help:"Timeout after which the agent is automatically resumed, in minutes" default:"5"`
}

func (c *PauseCmd) Help() string {
	return `When an agent is paused, it will stop accepting new jobs but will continue
running any jobs it has already started. You can optionally provide a note
explaining why the agent is being paused and set a timeout for automatic resumption.

The timeout must be between 1 and 1440 minutes (24 hours). If no timeout is
specified, the agent will pause for 5 minutes by default.

Examples:
  # Pause an agent for 5 minutes (default)
  $ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45

  # Pause an agent with a note
  $ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45 --note "Maintenance scheduled"

  # Pause an agent with a note and 60 minute timeout
  $ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45 --note "too many llamas" --timeout-in-minutes 60

  # Pause for a short time (15 minutes) during deployment
  $ bk agent pause 0198d108-a532-4a62-9bd7-b2e744bf5c45 --note "Deploy in progress" --timeout-in-minutes 15`
}

func (c *PauseCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(version.Version)
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	if c.TimeoutInMinutes <= 0 {
		return fmt.Errorf("timeout-in-minutes must be 1 or more")
	}
	if c.TimeoutInMinutes > 1440 {
		return fmt.Errorf("timeout-in-minutes cannot exceed 1440 minutes (1 day)")
	}

	var pauseOpts *buildkite.AgentPauseOptions
	if c.Note != "" || c.TimeoutInMinutes > 0 {
		pauseOpts = &buildkite.AgentPauseOptions{
			Note:             c.Note,
			TimeoutInMinutes: c.TimeoutInMinutes,
		}
	}

	_, err = f.RestAPIClient.Agents.Pause(ctx, f.Config.OrganizationSlug(), c.AgentID, pauseOpts)
	if err != nil {
		return fmt.Errorf("failed to pause agent: %w", err)
	}

	message := fmt.Sprintf("Agent %s paused successfully", c.AgentID)
	if c.Note != "" {
		message += fmt.Sprintf(" with note: %s", c.Note)
	}
	if c.TimeoutInMinutes > 0 {
		message += fmt.Sprintf(" (auto-resume in %d minutes)", c.TimeoutInMinutes)
	}

	fmt.Printf("%s\n", message)
	return nil
}
