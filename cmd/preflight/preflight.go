package preflight

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"

	"github.com/buildkite/cli/v3/cmd/version"
	buildstate "github.com/buildkite/cli/v3/internal/build/state"
	"github.com/buildkite/cli/v3/internal/build/watch"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	bkhttp "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/preflight"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type PreflightCmd struct {
	Pipeline         string               `help:"The pipeline to build. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Watch            bool                 `help:"Watch the build until completion." default:"true" negatable:""`
	Interval         float64              `help:"Polling interval in seconds when watching." default:"2"`
	NoCleanup        bool                 `help:"Skip deleting the remote preflight branch after the build finishes."`
	AwaitTestResults awaitTestResultsFlag `name:"await-test-results" help:"After the build finishes, wait for tests results to be processed by Test Engine. Provide a duration like 10s, or omit the value to wait 30s."`
	Text             bool                 `help:"Use plain text output instead of interactive terminal UI." xor:"output"`
	JSON             bool                 `help:"Emit one JSON object per event (JSONL)." xor:"output"`
	Default          PreflightDefaultCmd  `cmd:"" optional:"" hidden:"" default:"1"`
}

type PreflightDefaultCmd struct{}

var (
	notifyContext = signal.NotifyContext
	newFactory    = factory.New
)

const defaultAwaitTestResultsDuration = 30 * time.Second

type awaitTestResultsFlag struct {
	Enabled  bool
	Duration time.Duration
}

func (f *awaitTestResultsFlag) Decode(ctx *kong.DecodeContext) error {
	var value string
	if err := ctx.Scan.PopValueInto("duration", &value); err != nil {
		f.Enabled = true
		f.Duration = defaultAwaitTestResultsDuration
		return nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}

	f.Enabled = true
	f.Duration = duration
	return nil
}

func (f awaitTestResultsFlag) IsBool() bool { return true }

func (c *PreflightCmd) Help() string {
	return `Snapshots your working tree (uncommitted, staged, and untracked changes) and pushes it to a bk/preflight/<id> branch. If there are no local changes, pushes HEAD directly.`
}

func (c *PreflightCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	rlTransport := bkhttp.NewRateLimitTransport(http.DefaultTransport)
	f, err := newFactory(factory.WithDebug(globals.EnableDebug()), factory.WithTransport(rlTransport))
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to initialize CLI", "This is likely a bug", "Report to Buildkite")
	}

	pCtx, err := setup(c.Pipeline, globals)
	if err != nil {
		return err
	}
	defer pCtx.Stop()

	f := pCtx.Factory
	repoRoot := pCtx.RepoRoot
	resolvedPipeline := pCtx.Pipeline
	ctx := pCtx.Ctx
	stop := pCtx.Stop
	rlTransport := pCtx.RateLimitTransport

	preflightID, err := uuid.NewV7()
	if err != nil {
		return bkErrors.NewInternalError(err, "UUIDv7 generation failed")
	}
	startedAt := time.Now()

	sourceContext, err := preflight.ResolveSourceContext(repoRoot, globals.EnableDebug())
	if err != nil {
		return bkErrors.NewValidationError(
			err,
			"failed to resolve preflight source git context",
			"Ensure the repository has at least one commit",
		)
	}

	renderer := newRenderer(os.Stdout, c.JSON, c.Text, stop)

	rlTransport.OnRateLimit = func(attempt int, delay time.Duration) {
		if globals.EnableDebug() {
			_ = renderer.Render(Event{
				Type:        EventOperation,
				Time:        time.Now(),
				PreflightID: preflightID.String(),
				Title:       fmt.Sprintf("Rate limited by API, waiting %s before retrying (attempt %d/%d)...", delay.Truncate(time.Second), attempt+1, rlTransport.MaxRetries),
			})
		}
	}

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

	env := map[string]string{
		"PREFLIGHT":               "true",
		"BUILDKITE_PREFLIGHT":     "true", // deprecated
		"PREFLIGHT_SOURCE_COMMIT": sourceContext.Commit,
	}
	if sourceContext.Branch != "" {
		env["PREFLIGHT_SOURCE_BRANCH"] = sourceContext.Branch
	}

	build, _, err := f.RestAPIClient.Builds.Create(ctx, resolvedPipeline.Org, resolvedPipeline.Name, buildkite.CreateBuild{
		Message: fmt.Sprintf("Preflight %s", preflightID),
		Commit:  result.Commit,
		Branch:  result.Branch,
		Env:     env,
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

		showResult, showErr := c.loadFinalResult(ctx, f.RestAPIClient, resolvedPipeline.Org, resolvedPipeline.Name, build.Number)
		if showErr == nil {
			summaryEvent.Tests = showResult.Tests
		} else if globals.EnableDebug() {
			_ = renderer.Render(Event{
				Type:        EventOperation,
				Time:        time.Now(),
				PreflightID: preflightID.String(),
				Title:       fmt.Sprintf("Debug: failed to load final test summary: %v", showErr),
			})
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

func (c *PreflightCmd) loadFinalResult(ctx context.Context, client *buildkite.Client, org, pipeline string, buildNumber int) (preflight.SummaryResult, error) {
	buildWithTests, _, buildErr := client.Builds.Get(ctx, org, pipeline, strconv.Itoa(buildNumber), &buildkite.BuildGetOptions{IncludeTestEngine: true})
	expectTestSummary := buildErr == nil && buildWithTests.TestEngine != nil && len(buildWithTests.TestEngine.Runs) > 0

	if buildErr != nil {
		return preflight.SummaryResult{}, buildErr
	}

	if !c.AwaitTestResults.Enabled || c.AwaitTestResults.Duration <= 0 {
		return c.loadSummary(ctx, client, org, buildWithTests.ID)
	}
	if !expectTestSummary {
		return preflight.SummaryResult{Tests: preflight.SummaryTests{Runs: map[string]preflight.SummaryTestRun{}, Failures: []preflight.SummaryTestFailure{}}}, nil
	}

	timer := time.NewTimer(c.AwaitTestResults.Duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return preflight.SummaryResult{}, ctx.Err()
	case <-timer.C:
	}

	return c.loadSummary(ctx, client, org, buildWithTests.ID)
}

func (c *PreflightCmd) loadSummary(ctx context.Context, client *buildkite.Client, org, buildID string) (preflight.SummaryResult, error) {
	if buildID == "" {
		return preflight.SummaryResult{Tests: preflight.SummaryTests{Runs: map[string]preflight.SummaryTestRun{}, Failures: []preflight.SummaryTestFailure{}}}, nil
	}

	summary, err := preflight.NewRunSummaryService(client).Get(ctx, org, buildID, &preflight.RunSummaryGetOptions{
		Result:    "^failed",
		IncludeFailures: true,
	})
	if err != nil {
		return preflight.SummaryResult{}, err
	}

	return summary.SummaryResult(), nil
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
