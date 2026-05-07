package preflight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"

	buildstate "github.com/buildkite/cli/v3/internal/build/state"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	internalpreflight "github.com/buildkite/cli/v3/internal/preflight"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type MonitorCmd struct {
	Pipeline         string                         `help:"The pipeline to monitor. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	ExitOn           []internalpreflight.ExitPolicy `help:"Exit when a condition is met. Options: build-failing (default, exits when a build enters the failing state), build-terminal (exits when the build reaches a terminal state)."`
	Interval         float64                        `help:"Polling interval in seconds when looking for and watching the build." default:"2"`
	WaitForBuild     time.Duration                  `name:"wait-for-build" help:"How long to wait for the pushed commit's build to appear." default:"2m"`
	AwaitTestResults awaitTestResultsFlag           `name:"await-test-results" help:"After the build finishes, wait for test results to be processed by Test Engine. Provide a duration like 10s, or omit the value to wait 30s."`
	Text             bool                           `help:"Use plain text output instead of interactive terminal UI." xor:"output"`
	JSON             bool                           `help:"Emit one JSON object per event (JSONL)." xor:"output"`
}

func (c *MonitorCmd) Help() string {
	return `Monitor the existing CI build for the current pushed branch and commit.

Unlike preflight run, monitor does not create a snapshot branch or a new build. It watches the normal Buildkite build created after git push, so it cannot set PREFLIGHT=true.`
}

func (c *MonitorCmd) Validate() error {
	if c.Interval <= 0 {
		return bkErrors.NewValidationError(fmt.Errorf("interval must be greater than 0"), "invalid polling interval")
	}
	if c.WaitForBuild <= 0 {
		return bkErrors.NewValidationError(fmt.Errorf("wait-for-build must be greater than 0"), "invalid build lookup timeout")
	}
	return internalpreflight.ValidateExitPolicies(c.ExitOn, true)
}

func (c *MonitorCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	sourceContext, err := internalpreflight.ResolveSourceContext(repoRoot, globals.EnableDebug())
	if err != nil {
		return bkErrors.NewValidationError(
			err,
			"failed to resolve source git context",
			"Ensure the repository has at least one commit",
		)
	}
	if sourceContext.Branch == "" {
		return bkErrors.NewValidationError(
			fmt.Errorf("detached HEAD"),
			"could not determine branch for build lookup",
			"Detached HEAD cannot be matched to a Buildkite branch automatically",
			"Check out a branch before running `bk preflight monitor`",
		)
	}

	renderer := rendererFactory(os.Stdout, c.JSON, c.Text, stop)
	defer renderer.Close()

	rlTransport.OnRateLimit = func(attempt int, delay time.Duration) {
		if globals.EnableDebug() {
			_ = renderer.Render(Event{
				Type:  EventOperation,
				Time:  time.Now(),
				Title: fmt.Sprintf("Rate limited by API, waiting %s before retrying (attempt %d/%d)...", delay.Truncate(time.Second), attempt+1, rlTransport.MaxRetries),
			})
		}
	}

	interval := time.Duration(c.Interval * float64(time.Second))
	startedAt := time.Now()
	pipelineName := fmt.Sprintf("%s/%s", resolvedPipeline.Org, resolvedPipeline.Name)

	_ = renderer.Render(Event{
		Type:  EventOperation,
		Time:  time.Now(),
		Title: fmt.Sprintf("Looking for build on %s for branch %s at %s...", pipelineName, sourceContext.Branch, shortCommit(sourceContext.Commit)),
	})

	build, err := c.waitForBuild(ctx, f.RestAPIClient, resolvedPipeline.Org, resolvedPipeline.Name, sourceContext, interval)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return bkErrors.NewUserAbortedError(context.Canceled, "preflight monitor canceled by user")
		}
		return err
	}

	_ = renderer.Render(Event{
		Type:        EventOperation,
		Time:        time.Now(),
		Pipeline:    pipelineName,
		BuildNumber: build.Number,
		BuildURL:    build.WebURL,
		Title:       fmt.Sprintf("Found build #%d on %s...", build.Number, pipelineName),
		Detail:      fmt.Sprintf("Build:  %s", build.WebURL),
	})

	watchCtx := buildWatchContext{
		Context:          ctx,
		Client:           f.RestAPIClient,
		Renderer:         renderer,
		PipelineName:     pipelineName,
		Org:              resolvedPipeline.Org,
		Pipeline:         resolvedPipeline.Name,
		Build:            build,
		StartedAt:        startedAt,
		Interval:         interval,
		ExitPolicy:       exitPolicy,
		AwaitTestResults: c.AwaitTestResults,
		Debug:            globals.EnableDebug(),
	}
	finalBuild, tracker, err := watchCtx.watch()
	finalErr := NewResult(finalBuild).Error()

	if errors.Is(err, context.Canceled) {
		return bkErrors.NewUserAbortedError(context.Canceled, "preflight monitor canceled by user")
	}

	if errors.Is(err, errExitOnBuildFailing) {
		watchCtx.renderSummary(finalBuild, tracker, summaryMeta{Incomplete: true, StopReason: "build-failing", BuildCanceled: false})
		return finalErr
	}

	if buildstate.IsTerminal(buildstate.State(finalBuild.State)) {
		watchCtx.renderSummary(finalBuild, tracker, summaryMeta{})
	}

	if err != nil {
		return bkErrors.NewInternalError(
			err, "watching build failed",
			"Buildkite API may be unavailable or your network may be unstable",
			"Retry the preflight monitor command once connectivity is restored",
		)
	}

	return finalErr
}

func (c *MonitorCmd) waitForBuild(ctx context.Context, client *buildkite.Client, org, pipeline string, source internalpreflight.SourceContext, interval time.Duration) (buildkite.Build, error) {
	deadline := time.NewTimer(c.WaitForBuild)
	defer deadline.Stop()

	for {
		build, found, err := findBuildForSource(ctx, client, org, pipeline, source)
		if err != nil {
			return buildkite.Build{}, err
		}
		if found {
			return build, nil
		}

		select {
		case <-ctx.Done():
			return buildkite.Build{}, ctx.Err()
		case <-deadline.C:
			return buildkite.Build{}, noBuildFoundError(source)
		case <-time.After(interval):
		}
	}
}

func findBuildForSource(ctx context.Context, client *buildkite.Client, org, pipeline string, source internalpreflight.SourceContext) (buildkite.Build, bool, error) {
	builds, _, err := client.Builds.ListByPipeline(ctx, org, pipeline, &buildkite.BuildsListOptions{
		Branch:      []string{source.Branch},
		Commit:      source.Commit,
		ListOptions: buildkite.ListOptions{PerPage: 1},
	})
	if err != nil {
		return buildkite.Build{}, false, bkErrors.WrapAPIError(err, "looking up build")
	}
	if len(builds) == 0 {
		return buildkite.Build{}, false, nil
	}
	return builds[0], true, nil
}

func noBuildFoundError(source internalpreflight.SourceContext) error {
	return bkErrors.NewValidationError(
		fmt.Errorf("no build found for branch %s at %s", source.Branch, source.Commit),
		fmt.Sprintf("no Buildkite build found for branch %s at %s", source.Branch, shortCommit(source.Commit)),
		"Ensure the commit has been pushed",
		"Ensure the selected pipeline runs for this branch",
		"Try again in a few seconds if the push just happened",
	)
}

func shortCommit(commit string) string {
	if len(commit) >= 10 {
		return commit[:10]
	}
	return commit
}
