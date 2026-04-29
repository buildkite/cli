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
	internalconfig "github.com/buildkite/cli/v3/internal/config"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	bkhttp "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type RunCmd struct {
	Pipeline         string                         `help:"The pipeline to build. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Watch            bool                           `help:"Watch the build until completion." default:"true" negatable:""`
	ExitOn           []internalpreflight.ExitPolicy `help:"Exit when a condition is met. Options: build-failing (default, exits when a build enters the failing state), build-terminal (exits when the build reaches a terminal state)."`
	Interval         float64                        `help:"Polling interval in seconds when watching." default:"2"`
	NoCleanup        bool                           `help:"Skip cleanup after completion or early exit. The preflight branch remains and the build keeps running if exiting early."`
	AwaitTestResults awaitTestResultsFlag           `name:"await-test-results" help:"After the build finishes, wait for test results to be processed by Test Engine. Provide a duration like 10s, or omit the value to wait 30s."`
	Text             bool                           `help:"Use plain text output instead of interactive terminal UI." xor:"output"`
	JSON             bool                           `help:"Emit one JSON object per event (JSONL)." xor:"output"`
}

var (
	notifyContext   = signal.NotifyContext
	newFactory      = factory.New
	rendererFactory = newRenderer

	errExitOnBuildFailing = errors.New("exit-on build-failing")
)

const defaultAwaitTestResultsDuration = 30 * time.Second

func HelpText() string {
	return `Preflight is an experimental preview and subject to change without notice.

Snapshots your working tree (staged, unstaged, and untracked files) to a temporary commit on a bk/preflight/<id> branch, triggers a build on the selected pipeline, monitors failures, exits as soon as the build starts failing, and cleans up the temporary branch when finished.`
}

type summaryMeta struct {
	Incomplete    bool
	StopReason    string
	BuildCanceled bool
}

func preflightUserAgentSuffix() string {
	major := strings.TrimPrefix(version.Version, "v")
	if i := strings.IndexByte(major, '.'); i >= 0 {
		major = major[:i]
	}
	if major == "" || major == "DEV" {
		major = "DEV"
	}
	return "buildkite-cli-preflight/" + major + ".x"
}

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

func (c *RunCmd) Help() string {
	return HelpText()
}

func (c *RunCmd) Validate() error {
	if c.Interval <= 0 {
		return bkErrors.NewValidationError(fmt.Errorf("interval must be greater than 0"), "invalid polling interval")
	}
	return internalpreflight.ValidateExitPolicies(c.ExitOn, c.Watch)
}

func (c *RunCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	if err := c.Validate(); err != nil {
		return err
	}

	exitPolicy := internalpreflight.EffectiveExitPolicy(c.ExitOn)

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

	sourceContext, err := internalpreflight.ResolveSourceContext(repoRoot, globals.EnableDebug())
	if err != nil {
		return bkErrors.NewValidationError(
			err,
			"failed to resolve preflight source git context",
			"Ensure the repository has at least one commit",
		)
	}

	renderer := rendererFactory(os.Stdout, c.JSON, c.Text, stop)
	defer renderer.Close()

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

	var opts []internalpreflight.SnapshotOption
	if globals.EnableDebug() {
		opts = append(opts, internalpreflight.WithDebug())
	}

	result, err := internalpreflight.Snapshot(repoRoot, preflightID, opts...)
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
		for _, retryPassed := range status.NewlyRetryPassed {
			if err := renderer.Render(Event{
				Type:        EventJobRetryPassed,
				Time:        time.Now(),
				PreflightID: preflightID.String(),
				Pipeline:    pipelineName,
				BuildNumber: build.Number,
				Job:         &retryPassed,
			}); err != nil {
				return err
			}
		}
		if err := renderer.Render(Event{
			Type:        EventBuildStatus,
			Time:        time.Now(),
			PreflightID: preflightID.String(),
			Pipeline:    pipelineName,
			BuildNumber: build.Number,
			BuildURL:    build.WebURL,
			BuildState:  b.State,
			Jobs:        &status.Summary,
		}); err != nil {
			return err
		}
		if exitPolicy == internalpreflight.ExitOnBuildFailing && buildstate.State(b.State) == buildstate.Failing {
			return errExitOnBuildFailing
		}
		return nil
	}, watch.WithRetriedJobs())

	finalErr := NewResult(finalBuild).Error()
	cleanupBranch := func() {
		if c.NoCleanup {
			return
		}
		cleanupRemoteBranch(renderer, repoRoot, result.Branch, result.Ref, preflightID.String(), globals.EnableDebug())
	}

	if errors.Is(err, context.Canceled) {
		cleanupBranch()
		if finalBuild.FinishedAt == nil && !buildstate.IsTerminal(buildstate.State(finalBuild.State)) && !c.NoCleanup {
			cancelBuild(f, renderer, resolvedPipeline.Org, resolvedPipeline.Name, build.Number, preflightID.String(), globals.EnableDebug())
		}
		return bkErrors.NewUserAbortedError(context.Canceled, "preflight canceled by user")
	}

	if errors.Is(err, errExitOnBuildFailing) {
		buildCanceled := false
		if !c.NoCleanup {
			buildCanceled = cancelBuild(f, renderer, resolvedPipeline.Org, resolvedPipeline.Name, build.Number, preflightID.String(), globals.EnableDebug())
		}

		summaryEvent := newBuildSummaryEvent(preflightID.String(), pipelineName, build.Number, build.WebURL, finalBuild, startedAt)
		summaryEvent.ApplySummaryMeta(summaryMeta{Incomplete: true, StopReason: "build-failing", BuildCanceled: buildCanceled})
		summaryEvent.ApplyJobResults(finalBuild, tracker)
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
		_ = renderer.Render(summaryEvent)
		cleanupBranch()
		return finalErr
	}

	// Emit a final summary showing pass/fail, passed jobs (if ≤10), or hard-failed jobs.
	if buildstate.IsTerminal(buildstate.State(finalBuild.State)) {
		summaryEvent := newBuildSummaryEvent(preflightID.String(), pipelineName, build.Number, build.WebURL, finalBuild, startedAt)
		summaryEvent.ApplySummaryMeta(summaryMeta{})
		summaryEvent.ApplyJobResults(finalBuild, tracker)

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
		_ = renderer.Render(summaryEvent)
	}

	cleanupBranch()

	if err != nil {
		return bkErrors.NewInternalError(err, "watching build failed",
			"Buildkite API may be unavailable or your network may be unstable",
			"Retry the preflight command once connectivity is restored",
		)
	}

	return finalErr
}

func cleanupRemoteBranch(renderer renderer, repoRoot, branch, ref, preflightID string, debug bool) {
	_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID, Title: fmt.Sprintf("Cleaning up remote branch %s...", branch)})
	if cleanupErr := internalpreflight.Cleanup(repoRoot, ref, debug); cleanupErr != nil {
		_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID, Title: fmt.Sprintf("Warning: failed to delete remote branch %s: %v", ref, cleanupErr)})
		return
	}
	_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID, Title: fmt.Sprintf("Deleted remote branch %s", branch)})
}

func cancelBuild(f *factory.Factory, renderer renderer, org, pipeline string, buildNumber int, preflightID string, debug bool) bool {
	cancelCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStop()

	if _, err := f.RestAPIClient.Builds.Cancel(cancelCtx, org, pipeline, strconv.Itoa(buildNumber)); err != nil {
		var apiErr *buildkite.ErrorResponse
		if errors.As(err, &apiErr) && apiErr.Response.StatusCode == http.StatusUnprocessableEntity && apiErr.Message == "Build can't be canceled because it's already finished." {
			if debug {
				_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID, Title: fmt.Sprintf("Debug: build #%d already finished, skipping cancel", buildNumber)})
			}
			return false
		}

		_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID, Title: fmt.Sprintf("Warning: failed to cancel build #%d: %v", buildNumber, err)})
		return false
	}

	_ = renderer.Render(Event{Type: EventOperation, Time: time.Now(), PreflightID: preflightID, Title: fmt.Sprintf("Cancelled build #%d", buildNumber)})
	return true
}

func (c *RunCmd) loadFinalResult(ctx context.Context, client *buildkite.Client, org, pipeline string, buildNumber int) (internalpreflight.SummaryResult, error) {
	buildWithTests, _, buildErr := client.Builds.Get(ctx, org, pipeline, strconv.Itoa(buildNumber), &buildkite.BuildGetOptions{IncludeTestEngine: true})
	expectTestSummary := buildErr == nil && buildWithTests.TestEngine != nil && len(buildWithTests.TestEngine.Runs) > 0

	if buildErr != nil {
		return internalpreflight.SummaryResult{}, buildErr
	}

	if !c.AwaitTestResults.Enabled || c.AwaitTestResults.Duration <= 0 {
		return c.loadSummary(ctx, client, org, buildWithTests.ID)
	}
	if !expectTestSummary {
		return internalpreflight.SummaryResult{Tests: internalpreflight.SummaryTests{Runs: map[string]internalpreflight.SummaryTestRun{}, Failures: []internalpreflight.SummaryTestFailure{}}}, nil
	}

	timer := time.NewTimer(c.AwaitTestResults.Duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return internalpreflight.SummaryResult{}, ctx.Err()
	case <-timer.C:
	}

	return c.loadSummary(ctx, client, org, buildWithTests.ID)
}

func (c *RunCmd) loadSummary(ctx context.Context, client *buildkite.Client, org, buildID string) (internalpreflight.SummaryResult, error) {
	if buildID == "" {
		return internalpreflight.SummaryResult{Tests: internalpreflight.SummaryTests{Runs: map[string]internalpreflight.SummaryTestRun{}, Failures: []internalpreflight.SummaryTestFailure{}}}, nil
	}

	summary, err := internalpreflight.NewRunSummaryService(client).Get(ctx, org, buildID, &internalpreflight.RunSummaryGetOptions{
		Result:          "^failed",
		State:           "enabled",
		IncludeFailures: true,
	})
	if err != nil {
		return internalpreflight.SummaryResult{}, err
	}

	return summary.SummaryResult(), nil
}

// preflightContext holds the common dependencies for preflight subcommands.
type preflightContext struct {
	Factory            *factory.Factory
	RepoRoot           string
	Pipeline           *pipeline.Pipeline
	Ctx                context.Context
	Stop               context.CancelFunc
	RateLimitTransport *bkhttp.RateLimitTransport
}

// setup initializes the common preflight dependencies: factory, experiment
// gate, repository root, signal context, and pipeline resolution.
func setup(pipelineFlag string, globals cli.GlobalFlags) (*preflightContext, error) {
	rlTransport := bkhttp.NewRateLimitTransport(http.DefaultTransport)
	f, err := newFactory(
		factory.WithDebug(globals.EnableDebug()),
		factory.WithTransport(rlTransport),
		factory.WithUserAgentSuffix(preflightUserAgentSuffix()),
	)
	if err != nil {
		return nil, bkErrors.NewInternalError(err, "failed to initialize CLI", "This is likely a bug", "Report to Buildkite")
	}

	if !f.Config.HasExperiment(internalconfig.ExperimentPreflight) {
		return nil, bkErrors.NewValidationError(
			fmt.Errorf("experiment not enabled"),
			"preflight is disabled by the current experiments override. Add `preflight` to `BUILDKITE_EXPERIMENTS` or run `bk config set experiments preflight` to re-enable it")
	}

	repoRoot, err := resolveRepositoryRoot(f, globals.EnableDebug())
	if err != nil {
		return nil, bkErrors.NewValidationError(
			fmt.Errorf("not in a git repository: %w", err),
			"preflight must be run from a git repository",
			"Run this command from inside a git repository",
		)
	}

	ctx, stop := notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	resolvers := resolver.NewAggregateResolver(
		resolver.ResolveFromFlag(pipelineFlag, f.Config),
		resolver.ResolveFromConfig(f.Config, resolver.PickOneWithFactory(f)),
		resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOneWithFactory(f))),
	)

	resolvedPipeline, err := resolvers.Resolve(ctx)
	if err != nil {
		stop()
		return nil, bkErrors.NewValidationError(err, "could not resolve a pipeline",
			"Specify a pipeline with --pipeline or link your repository to a pipeline",
		)
	}

	return &preflightContext{
		Factory:            f,
		RepoRoot:           repoRoot,
		Pipeline:           resolvedPipeline,
		Ctx:                ctx,
		Stop:               stop,
		RateLimitTransport: rlTransport,
	}, nil
}

func resolveRepositoryRoot(f *factory.Factory, debug bool) (string, error) {
	if f.GitRepository != nil {
		wt, err := f.GitRepository.Worktree()
		if err == nil {
			return wt.Filesystem.Root(), nil
		}
	}

	return internalpreflight.RepositoryRoot(".", debug)
}
