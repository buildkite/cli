package preflight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"

	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/internal/preflight"
)

// CleanupCmd deletes remote bk/preflight/* branches whose builds have completed.
type CleanupCmd struct {
	Pipeline      string `help:"The pipeline to check builds against. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	PreflightUUID string `help:"Target a single preflight branch by its UUID (bk/preflight/<uuid>)." name:"preflight-uuid"`
	DryRun        bool   `help:"Show which branches would be deleted without actually deleting them." name:"dry-run"`
	Text          bool   `help:"Use plain text output instead of interactive terminal UI." xor:"output"`
	JSON          bool   `help:"Emit one JSON object per event (JSONL)." xor:"output"`
}

func (c *CleanupCmd) Help() string {
	return `Deletes remote bk/preflight/* branches whose builds have completed (passed, failed, canceled). Branches with in-progress builds are left untouched to avoid interrupting concurrent preflight runs. Pass --preflight-uuid to target a single preflight branch.`
}

func (c *CleanupCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	if c.PreflightUUID != "" {
		if _, err := uuid.Parse(c.PreflightUUID); err != nil {
			return bkErrors.NewValidationError(err, fmt.Sprintf("invalid preflight UUID %q", c.PreflightUUID))
		}
	}

	pCtx, err := setup(c.Pipeline, globals)
	if err != nil {
		return err
	}
	defer pCtx.Stop()

	ctx := pCtx.Ctx
	repoRoot := pCtx.RepoRoot
	resolvedPipeline := pCtx.Pipeline

	var branches []preflight.BranchBuild
	if c.PreflightUUID != "" {
		branch, err := preflight.LookupRemotePreflightBranch(repoRoot, c.PreflightUUID, globals.EnableDebug())
		if err != nil {
			return bkErrors.NewInternalError(err, "failed to look up preflight branch")
		}
		if branch != nil {
			branches = []preflight.BranchBuild{*branch}
		}
	} else {
		branches, err = preflight.ListRemotePreflightBranches(repoRoot, globals.EnableDebug())
		if err != nil {
			return bkErrors.NewInternalError(err, "failed to list remote preflight branches")
		}
	}

	if len(branches) == 0 {
		if c.PreflightUUID != "" {
			fmt.Fprintf(os.Stdout, "No preflight branch found for UUID %s\n", c.PreflightUUID)
		} else {
			fmt.Fprintln(os.Stdout, "No preflight branches found")
		}
		return nil
	}

	renderer := newRenderer(os.Stdout, c.JSON, c.Text, pCtx.Stop)
	defer renderer.Close()

	_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), Title: fmt.Sprintf("Found %d preflight branch(es), checking build status...", len(branches))})

	if err := preflight.ResolveBuilds(ctx, pCtx.Factory.RestAPIClient, resolvedPipeline.Org, resolvedPipeline.Name, branches); err != nil {
		if errors.Is(err, context.Canceled) {
			_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), Title: "Cleanup interrupted"})
			return nil
		}
		return bkErrors.NewInternalError(err, "failed to check build status for preflight branches")
	}

	var toDelete []string
	var deleted, skipped int
	for i := range branches {
		bb := branches[i]
		if !bb.IsCompleted() {
			_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), Title: fmt.Sprintf("Skipping %s (build state: %s)", bb.Branch, bb.Build.State)})
			skipped++
			continue
		}

		state := "no build found"
		if bb.Build != nil {
			state = bb.Build.State
		}

		if c.DryRun {
			_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), Title: fmt.Sprintf("Would delete %s (%s)", bb.Branch, state)})
		} else {
			_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), Title: fmt.Sprintf("Deleting %s (%s)", bb.Branch, state)})
			toDelete = append(toDelete, bb.Ref)
		}
		deleted++
	}

	if !c.DryRun && len(toDelete) > 0 {
		if err := preflight.CleanupRefs(repoRoot, toDelete, globals.EnableDebug()); err != nil {
			return bkErrors.NewInternalError(err, "failed to delete preflight branches from remote")
		}
	}

	verb := "deleted"
	if c.DryRun {
		verb = "would delete"
	}
	_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), Title: fmt.Sprintf("Cleanup complete: %d %s, %d skipped", deleted, verb, skipped)})
	return nil
}
