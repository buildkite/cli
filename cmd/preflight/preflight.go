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
	Text      bool    `help:"Use plain text output instead of interactive terminal UI." xor:"output"`
	JSON      bool    `help:"Emit one JSON object per event (JSONL)." xor:"output"`
}

var (
	notifyContext = signal.NotifyContext
	newFactory    = factory.New
)

func (c *PreflightCmd) Help() string {
	return `Snapshots your working tree (uncommitted, staged, and untracked changes) and pushes it to a bk/preflight/<id> branch. If there are no local changes, pushes HEAD directly.`
}

func (c *PreflightCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := newFactory(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to initialize CLI", "This is likely a bug", "Report to Buildkite")
	}

	if !f.Config.HasExperiment("preflight") {
		return bkErrors.NewValidationError(
			fmt.Errorf("experiment not enabled"),
			"the preflight command is under development and requires the 'preflight' experiment to opt in. Run: bk config set experiments preflight or set BUILDKITE_EXPERIMENTS=preflight")
	}

	repoRoot, err := resolveRepositoryRoot(f, globals.EnableDebug())
	if err != nil {
		return bkErrors.NewValidationError(
			fmt.Errorf("not in a git repository: %w", err),
			"preflight must be run from a git repository",
			"Run this command from inside a git repository",
		)
	}

	preflightID, err := uuid.NewV7()
	if err != nil {
		return bkErrors.NewInternalError(err, "UUIDv7 generation failed")
	}
	startedAt := time.Now()

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

	renderer := newRenderer(os.Stdout, c.JSON, c.Text, stop)

	_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: "Pushing snapshot of working tree..."})

	var opts []preflight.SnapshotOption
	if globals.EnableDebug() {
		opts = append(opts, preflight.WithDebug())
	}

	result, err := preflight.Snapshot(repoRoot, preflightID, opts...)
	if err != nil {
		return bkErrors.NewSnapshotError(err, "failed to create preflight snapshot",
			"Ensure you have uncommitted or committed changes to snapshot",
			"Ensure you have push access to the remote repository",
		)
	}

	snapshotDetail := fmt.Sprintf("Commit: %s\nRef: %s", result.ShortCommit(), result.Ref)
	if len(result.Files) > 0 {
		snapshotDetail += fmt.Sprintf("\nFiles:  %d changed", len(result.Files))
		for _, file := range result.Files {
			snapshotDetail += fmt.Sprintf("\n %s %s", file.StatusSymbol(), file.Path)
		}
	}
	_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: "Pushed snapshot of working tree...", Detail: snapshotDetail})

	_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: fmt.Sprintf("Creating build on %s/%s...", resolvedPipeline.Org, resolvedPipeline.Name)})

	build, _, err := f.RestAPIClient.Builds.Create(ctx, resolvedPipeline.Org, resolvedPipeline.Name, buildkite.CreateBuild{
		Message: fmt.Sprintf("Preflight %s", preflightID),
		Commit:  result.Commit,
		Branch:  result.Branch,
		Env: map[string]string{
			"PREFLIGHT":           "true",
			"BUILDKITE_PREFLIGHT": "true", // deprecated
		},
	})
	if err != nil {
		return bkErrors.WrapAPIError(err, "creating preflight build")
	}

	pipelineName := fmt.Sprintf("%s/%s", resolvedPipeline.Org, resolvedPipeline.Name)
	_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: fmt.Sprintf("Created build on %s/%s...", resolvedPipeline.Org, resolvedPipeline.Name), Detail: fmt.Sprintf("Build:  %s", build.WebURL)})

	if !c.Watch {
		_ = renderer.Close()
		return nil
	}

	interval := time.Duration(c.Interval * float64(time.Second))
	tracker := watch.NewJobTracker()

	finalBuild, err := watch.WatchBuild(ctx, f.RestAPIClient, resolvedPipeline.Org, resolvedPipeline.Name, build.Number, interval, func(b buildkite.Build) error {
		status := tracker.Update(b)
		for _, failed := range status.NewlyFailed {
			if err := renderer.Render(Event{
				Type:        EventJobFailure,
				Time:        time.Now(),
				PreflightID: preflightID.String(),
				Pipeline:    pipelineName,
				BuildNumber: build.Number,
				Job:         &failed,
			}); err != nil {
				return err
			}
		}
		return renderer.Render(Event{
			Type:        EventBuildStatus,
			Time:        time.Now(),
			PreflightID: preflightID.String(),
			Pipeline:    pipelineName,
			BuildNumber: build.Number,
			BuildURL:    build.WebURL,
			BuildState:  b.State,
			Jobs:        &status.Summary,
		})
	}, watch.WithTestTracking(func(newTestChanges []buildkite.BuildTest) error {
		return renderer.Render(Event{
			Type:         EventTestFailure,
			Time:         time.Now(),
			PreflightID:  preflightID.String(),
			Pipeline:     pipelineName,
			BuildNumber:  build.Number,
			TestFailures: newTestChanges,
		})
	}))

	buildResult := NewResult(finalBuild)
	finalErr := buildResult.Error()

	// Emit a final summary showing pass/fail, passed jobs (if ≤10), or hard-failed jobs.
	if buildstate.IsTerminal(buildstate.State(finalBuild.State)) {
		summaryEvent := Event{
			Type:        EventBuildSummary,
			Time:        time.Now(),
			PreflightID: preflightID.String(),
			Pipeline:    pipelineName,
			BuildNumber: build.Number,
			BuildURL:    build.WebURL,
			BuildState:  finalBuild.State,
			Duration:    time.Since(startedAt),
		}
		if buildResult.Passed() {
			if passed := tracker.PassedJobs(); len(passed) <= 10 {
				summaryEvent.PassedJobs = passed
			}
		} else {
			summaryEvent.FailedJobs = tracker.FailedJobs()
		}
		_ = renderer.Render(summaryEvent)
	}

	if !c.NoCleanup {
		_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: fmt.Sprintf("Cleaning up remote branch %s...", result.Branch)})
		if cleanupErr := preflight.Cleanup(repoRoot, result.Ref, globals.EnableDebug()); cleanupErr != nil {
			_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: fmt.Sprintf("Warning: failed to delete remote branch %s: %v", result.Ref, cleanupErr)})
		} else {
			_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: fmt.Sprintf("Deleted remote branch %s", result.Branch)})
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
						_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: fmt.Sprintf("Debug: build #%d already finished, skipping cancel", build.Number)})
					}
				} else {
					_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: fmt.Sprintf("Warning: failed to cancel build #%d: %v", build.Number, cancelErr)})
				}
			} else {
				_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID.String(), Title: fmt.Sprintf("Cancelled build #%d", build.Number)})
			}
		}
		_ = renderer.Close()
		return bkErrors.NewUserAbortedError(context.Canceled, "preflight canceled by user")
	}
	_ = renderer.Close()

	if err != nil {
		return bkErrors.NewInternalError(err, "watching build failed",
			"Buildkite API may be unavailable or your network may be unstable",
			"Retry the preflight command once connectivity is restored",
		)
	}

	return finalErr
}

func resolveRepositoryRoot(f *factory.Factory, debug bool) (string, error) {
	if f.GitRepository != nil {
		wt, err := f.GitRepository.Worktree()
		if err == nil {
			return wt.Filesystem.Root(), nil
		}
	}

	return preflight.RepositoryRoot(".", debug)
}
