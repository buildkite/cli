package preflight

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"

	buildstate "github.com/buildkite/cli/v3/internal/build/state"
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

type renderStatusError struct {
	err error
}

var notifyContext = signal.NotifyContext

func (e renderStatusError) Error() string {
	return e.err.Error()
}

func (e renderStatusError) Unwrap() error {
	return e.err
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
	ctx, stop := notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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

	tty := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	snapshotOutput := []string{"Creating snapshot of working tree..."}

	var opts []preflight.SnapshotOption
	if globals.EnableDebug() {
		opts = append(opts, preflight.WithDebug())
	}

	result, err := preflight.Snapshot(wt.Filesystem.Root(), preflightID, opts...)
	if err != nil {
		return bkErrors.NewSnapshotError(err, "failed to create preflight snapshot",
			"Ensure you have uncommitted or committed changes to snapshot",
			"Ensure you have push access to the remote repository",
		)
	}
	snapshotOutput = append(snapshotOutput, snapshotLines(result)...)
	snapshotOutput = append(snapshotOutput, "")
	snapshotOutput = append(snapshotOutput, fmt.Sprintf("Creating build on %s/%s...", resolvedPipeline.Org, resolvedPipeline.Name))
	build, _, err := f.RestAPIClient.Builds.Create(ctx, resolvedPipeline.Org, resolvedPipeline.Name, buildkite.CreateBuild{
		Message: fmt.Sprintf("Preflight %s", preflightID),
		Commit:  result.Commit,
		Branch:  result.Branch,
		Env: map[string]string{
			"BUILDKITE_PREFLIGHT": "true",
		},
	})
	if err != nil {
		return bkErrors.WrapAPIError(err, "creating preflight build")
	}

	renderer := newRenderer(os.Stdout, tty, resolvedPipeline.Name, build.Number)
	for _, line := range snapshotOutput {
		renderer.appendSnapshotLine(line)
	}
	renderer.appendSnapshotLine(fmt.Sprintf("Build:  %s", build.WebURL))

	if !c.Watch {
		return nil
	}

	interval := time.Duration(c.Interval * float64(time.Second))
	tracker := watch.NewJobTracker()

	finalBuild, err := watch.WatchBuild(ctx, f.RestAPIClient, resolvedPipeline.Org, resolvedPipeline.Name, build.Number, interval, func(b buildkite.Build) error {
		if err := renderer.renderStatus(tracker.Update(b), b.State); err != nil {
			return renderStatusError{err: err}
		}
		return nil
	})
	if err != nil {
		var renderErr renderStatusError
		if errors.As(err, &renderErr) {
			return bkErrors.NewInternalError(renderErr.err, "rendering build status failed")
		}
	}

	// Flush the screen so final output is not overwritten.
	renderer.flush()

	buildResult := NewResult(finalBuild)
	failedJobs := tracker.FailedJobs()
	finalErr := buildResult.Error()
	renderer.renderFinalFailures(buildResult, failedJobs)

	if !c.NoCleanup {
		fmt.Fprintf(os.Stderr, "Cleaning up remote branch %s...\n", result.Branch)
		if cleanupErr := preflight.Cleanup(wt.Filesystem.Root(), result.Ref, globals.EnableDebug()); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete remote branch %s: %v\n", result.Ref, cleanupErr)
		} else {
			fmt.Printf("Deleted remote branch %s\n", result.Branch)
		}
	}

	if errors.Is(err, context.Canceled) {
		if finalBuild.FinishedAt == nil && !buildstate.IsTerminal(buildstate.State(finalBuild.State)) {
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
		return bkErrors.NewUserAbortedError(context.Canceled, "preflight canceled by user")
	}
	if err != nil {
		return bkErrors.NewInternalError(err, "watching build failed",
			"Buildkite API may be unavailable or your network may be unstable",
			"Retry the preflight command once connectivity is restored",
		)
	}

	return finalErr
}
