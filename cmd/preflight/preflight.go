package preflight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"

	"github.com/buildkite/cli/v3/internal/build/watch"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/preflight"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type PreflightCmd struct {
	Pipeline string  `help:"The pipeline to build. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Watch    bool    `help:"Watch the build until completion." default:"true" negatable:""`
	Interval float64 `help:"Polling interval in seconds when watching." default:"2"`
}

func (c *PreflightCmd) Help() string {
	return `Snapshots your working tree (uncommitted, staged, and untracked changes) and pushes it to a bk/preflight/<id> branch. If there are no local changes, pushes HEAD directly.`
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
		for _, file := range result.Files {
			fmt.Printf("  %s %s\n", file.StatusSymbol(), file.Path)
		}
	}

	// Resolve the pipeline to create a build against.
	ctx := context.Background()
	resolvers := resolver.NewAggregateResolver(
		resolver.ResolveFromFlag(c.Pipeline, f.Config),
		resolver.ResolveFromConfig(f.Config, resolver.PickOneWithFactory(f)),
		resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOneWithFactory(f))),
	)

	resolvedPipeline, err := resolvers.Resolve(ctx)
	if err != nil {
		return bkErrors.NewValidationError(err, "could not resolve a pipeline",
			"Specify a pipeline in .bk.yaml or link your repository to a pipeline",
		)
	}

	fmt.Printf("Creating build on %s/%s...\n", resolvedPipeline.Org, resolvedPipeline.Name)
	build, _, err := f.RestAPIClient.Builds.Create(ctx, resolvedPipeline.Org, resolvedPipeline.Name, buildkite.CreateBuild{
		Message: fmt.Sprintf("Preflight %s", preflightID),
		Commit:  result.Commit,
		Branch:  result.Branch,
	})
	if err != nil {
		return bkErrors.WrapAPIError(err, "creating preflight build")
	}

	fmt.Printf("Build:  %s\n", build.WebURL)

	if !c.Watch {
		return nil
	}

	fmt.Println()

	tty := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	interval := time.Duration(c.Interval * float64(time.Second))
	var lastLine string
	var lastLineLen int
	finalBuild, err := watch.WatchBuild(ctx, f.RestAPIClient, resolvedPipeline.Org, resolvedPipeline.Name, build.Number, interval, func(b buildkite.Build) {
		s := watch.Summarize(b)
		var parts []string
		if s.Passed > 0 {
			parts = append(parts, fmt.Sprintf("%d passed", s.Passed))
		}
		if s.Failed > 0 {
			parts = append(parts, fmt.Sprintf("%d failed", s.Failed))
		}
		if s.Canceled > 0 {
			parts = append(parts, fmt.Sprintf("%d canceled", s.Canceled))
		}
		if s.Running > 0 {
			parts = append(parts, fmt.Sprintf("%d running", s.Running))
		}
		if s.Scheduled > 0 {
			parts = append(parts, fmt.Sprintf("%d scheduled", s.Scheduled))
		}
		if s.Blocked > 0 {
			parts = append(parts, fmt.Sprintf("%d blocked", s.Blocked))
		}
		if s.Skipped > 0 {
			parts = append(parts, fmt.Sprintf("%d skipped", s.Skipped))
		}
		if s.Waiting > 0 {
			parts = append(parts, fmt.Sprintf("%d waiting", s.Waiting))
		}
		line := fmt.Sprintf("Build #%d %s — %s", b.Number, b.State, strings.Join(parts, ", "))
		if tty {
			// Clear previous line and overwrite in place every iteration
			display := fmt.Sprintf("[%s] %s", time.Now().Format(time.TimeOnly), line)
			padding := ""
			if len(display) < lastLineLen {
				padding = strings.Repeat(" ", lastLineLen-len(display))
			}
			fmt.Printf("\r%s%s", display, padding)
			lastLineLen = len(display)
		} else if line != lastLine {
			fmt.Printf("[%s] %s\n", time.Now().Format(time.TimeOnly), line)
			lastLine = line
		}
	})
	if tty {
		fmt.Println() // move past the replaced line
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	if err != nil {
		return bkErrors.NewInternalError(err, "watching build failed",
			"Buildkite API may be unavailable or your network may be unstable",
			"Retry the preflight command once connectivity is restored",
		)
	}

	fmt.Println()
	if finalBuild.State == "passed" {
		fmt.Println("✅ Preflight passed!")
		return nil
	}
	fmt.Printf("❌ Preflight %s\n", finalBuild.State)
	return fmt.Errorf("preflight build %s", finalBuild.State)
}
