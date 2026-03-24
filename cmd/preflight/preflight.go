package preflight

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"

	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/internal/preflight"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type PreflightCmd struct{}

func (c *PreflightCmd) Help() string {
	return `Snapshots your working tree (uncommitted, staged, and untracked changes) and pushes it to a bk/preflight/$USER/<id> branch. If there are no local changes, pushes HEAD directly.`
}

func (c *PreflightCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to initialize CLI", "This is likely a bug", "Report to Buildkite")
	}

	if !f.Config.HasExperiment("preflight") {
		return bkErrors.NewValidationError(
			fmt.Errorf("experiment not enabled"),
			"the preflight command is under development and requires the 'preflight' experiment to opt in. Run: bk config set experiments preflight or set BUILDKITE_EXPERIMENTS=preflight")
	}

	if f.GitRepository == nil {
		return bkErrors.NewValidationError(
			fmt.Errorf("not in a git repository"),
			"preflight must be run from a git repository",
			"Run this command from inside a git repository",
		)
	}

	wt, err := f.GitRepository.Worktree()
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to get git worktree")
	}

	preflightID, err := uuid.NewV7()
	if err != nil {
		return bkErrors.NewInternalError(err, "UUIDv7 generation failed")
	}

	var opts []preflight.SnapshotOption
	if globals.EnableDebug() {
		opts = append(opts, preflight.WithDebug())
	}

	fmt.Println("Creating snapshot of working tree...")
	result, err := preflight.Snapshot(wt.Filesystem.Root(), preflightID, opts...)
	if err != nil {
		return bkErrors.NewSnapshotError(err, "failed to create preflight snapshot",
			"Ensure you have uncommitted or committed changes to snapshot",
			"Ensure you have push access to the remote repository",
		)
	}

	fmt.Printf("Commit: %s\n", result.Commit[:10])
	fmt.Printf("Ref:    %s\n", result.Ref)
	if len(result.Files) > 0 {
		fmt.Printf("Files:  %d changed\n", len(result.Files))
		for _, f := range result.Files {
			fmt.Printf("  %s %s\n", f.StatusSymbol(), f.Path)
		}
	}

	return nil
}
