package preflight

import (
	"context"
	"errors"
	"fmt"
<<<<<<< HEAD
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
=======
	"strings"
>>>>>>> 24534ac (Add other job states)
	"time"
	"unicode/utf8"

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
	Pipeline  string  `help:"The pipeline to build. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Watch     bool    `help:"Watch the build until completion." default:"true" negatable:""`
	Interval  float64 `help:"Polling interval in seconds when watching." default:"2"`
	NoCleanup bool    `help:"Skip deleting the remote preflight branch after the build finishes."`
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

	if c.Interval <= 0 {
		return bkErrors.NewValidationError(fmt.Errorf("interval must be greater than 0"), "invalid polling interval")
	}
	// Resolve the pipeline to create a build against.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
	var lastWidth int
	finalBuild, err := watch.WatchBuild(ctx, f.RestAPIClient, resolvedPipeline.Org, resolvedPipeline.Name, build.Number, interval, func(b buildkite.Build) {
		line := fmt.Sprintf("Build #%d %s", b.Number, b.State)
		if summary := watch.Summarize(b).String(); summary != "" {
			line += " — " + summary
		}
		if tty {
			display := fmt.Sprintf("[%s] %s", time.Now().Format(time.TimeOnly), line)
			width := utf8.RuneCountInString(display)
			padding := ""
			if width < lastWidth {
				padding = strings.Repeat(" ", lastWidth-width)
			}
			fmt.Printf("\r%s%s", display, padding)
			lastWidth = width
		} else if line != lastLine {
			fmt.Printf("[%s] %s\n", time.Now().Format(time.TimeOnly), line)
			lastLine = line
		}
	})
	if tty {
		fmt.Println()
	}

	if !c.NoCleanup {
		if cleanupErr := preflight.Cleanup(wt.Filesystem.Root(), result.Ref, globals.EnableDebug()); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete remote branch %s: %v\n", result.Ref, cleanupErr)
		} else {
			fmt.Printf("Deleted remote branch %s\n", result.Branch)
		}
	}

	if errors.Is(err, context.Canceled) {
		if finalBuild.FinishedAt == nil && !watch.IsTerminalBuildState(finalBuild.State) {
			cancelCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelStop()
			if _, cancelErr := f.RestAPIClient.Builds.Cancel(cancelCtx, resolvedPipeline.Org, resolvedPipeline.Name, strconv.Itoa(build.Number)); cancelErr != nil {
				var apiErr *buildkite.ErrorResponse
				if errors.As(cancelErr, &apiErr) && apiErr.Response.StatusCode == http.StatusUnprocessableEntity && apiErr.Message == "Build can't be canceled because it's already finished." {
					if globals.EnableDebug() {
						fmt.Fprintf(os.Stderr, "Debug: build #%d already finished, skipping cancel\n", build.Number)
					}
				} else {
					fmt.Fprintf(os.Stderr, "Warning: failed to cancel build #%d: %v\n", build.Number, cancelErr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "Cancelled build #%d\n", build.Number)
			}
		}
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
