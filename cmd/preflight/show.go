package preflight

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"

	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	pkgValidation "github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
)

type ShowCmd struct {
	PreflightID string `arg:"" name:"uuid" help:"The preflight UUID to inspect."`
	Failures    bool   `help:"Include detailed test failures in the output."`
}

func (c *ShowCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := newFactory(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to initialize CLI", "This is likely a bug", "Report to Buildkite")
	}

	if !f.Config.HasExperiment("preflight") {
		return bkErrors.NewValidationError(
			fmt.Errorf("experiment not enabled"),
			"the preflight command is under development and requires the 'preflight' experiment to opt in. Run: bk config set experiments preflight or set BUILDKITE_EXPERIMENTS=preflight")
	}

	commandPath := "preflight show"
	if kongCtx != nil {
		commandPath = kongCtx.Command()
	}
	if err := pkgValidation.ValidateConfiguration(f.Config, commandPath); err != nil {
		return err
	}

	org := f.Config.OrganizationSlug()
	if org == "" {
		return bkErrors.NewValidationError(
			fmt.Errorf("organization not configured"),
			"preflight show requires an organization",
			"Run bk auth login or bk use to select an organization",
		)
	}

	preflightID, err := uuid.Parse(c.PreflightID)
	if err != nil {
		return bkErrors.NewValidationError(err, "invalid preflight UUID")
	}

	result, err := internalpreflight.LoadShowResult(context.Background(), f.RestAPIClient, org, preflightID.String(), internalpreflight.ShowOptions{
		IncludeFailures: c.Failures,
	})
	if err != nil {
		return err
	}

	return output.Write(os.Stdout, result, output.FormatJSON)
}
