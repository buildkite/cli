package agent

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type ResumeCmd struct {
	AgentID string `arg:"" help:"Agent ID to resume"`
}

func (c *ResumeCmd) Help() string {
	return `Resume a paused Buildkite agent.

When an agent is resumed, it will start accepting new jobs again.

Examples:
  # Resume an agent
  $ bk agent resume 0198d108-a532-4a62-9bd7-b2e744bf5c45`
}

func (c *ResumeCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()
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

	_, err = f.RestAPIClient.Agents.Resume(ctx, f.Config.OrganizationSlug(), c.AgentID)
	if err != nil {
		return fmt.Errorf("failed to resume agent: %w", err)
	}

	fmt.Printf("Agent %s resumed successfully\n", c.AgentID)
	return nil
}
