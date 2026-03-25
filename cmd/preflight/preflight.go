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

	tty := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	// Create a Screen with regions for live terminal output.
	screen := preflight.NewScreen(os.Stdout)
	snapshotRegion := screen.AddRegion("snapshot")
	failedRegion := screen.AddRegion("failed")
	jobsRegion := screen.AddRegion("jobs")
	resultRegion := screen.AddRegion("result")

	var opts []preflight.SnapshotOption
	if globals.EnableDebug() {
		opts = append(opts, preflight.WithDebug())
	}

	snapshotRegion.AppendLine("Creating snapshot of working tree...")
	result, err := preflight.Snapshot(wt.Filesystem.Root(), preflightID, opts...)
	if err != nil {
		return bkErrors.NewSnapshotError(err, "failed to create preflight snapshot",
			"Ensure you have uncommitted or committed changes to snapshot",
			"Ensure you have push access to the remote repository",
		)
	}

	snapshotLines := []string{
		fmt.Sprintf("Commit: %s", result.Commit[:10]),
		fmt.Sprintf("Ref:    %s", result.Ref),
	}
	if len(result.Files) > 0 {
		snapshotLines = append(snapshotLines, fmt.Sprintf("Files:  %d changed", len(result.Files)))
		for _, file := range result.Files {
			snapshotLines = append(snapshotLines, fmt.Sprintf("  %s %s", file.StatusSymbol(), file.Path))
		}
	}
	snapshotRegion.SetLines(snapshotLines)

	snapshotRegion.AppendLine(fmt.Sprintf("Creating build on %s/%s...", resolvedPipeline.Org, resolvedPipeline.Name))
	build, _, err := f.RestAPIClient.Builds.Create(ctx, resolvedPipeline.Org, resolvedPipeline.Name, buildkite.CreateBuild{
		Message: fmt.Sprintf("Preflight %s", preflightID),
		Commit:  result.Commit,
		Branch:  result.Branch,
	})
	if err != nil {
		return bkErrors.WrapAPIError(err, "creating preflight build")
	}

	snapshotRegion.AppendLine(fmt.Sprintf("Build:  %s", build.WebURL))

	if !c.Watch {
		return nil
	}

	interval := time.Duration(c.Interval * float64(time.Second))
	tracker := watch.NewJobTracker()

	var lastLine string
	finalBuild, err := watch.WatchBuild(ctx, f.RestAPIClient, resolvedPipeline.Org, resolvedPipeline.Name, build.Number, interval, func(b buildkite.Build) {
		status := tracker.Update(b)

		if tty {
			for _, fj := range status.NewlyFailed {
				failedRegion.AppendLine(formatFailedJob(fj))
			}

			var lines []string
			for _, j := range status.Running {
				dur := watch.JobDuration(j)
				durStr := ""
				if dur > 0 {
					durStr = " " + dur.String()
				}
				lines = append(lines, fmt.Sprintf("  \033[36m●\033[0m %s  \033[36mrunning\033[0m%s", watch.JobDisplayName(j), durStr))
			}
			if status.TotalRunning > len(status.Running) {
				lines = append(lines, fmt.Sprintf("  \033[90m… and %d more running\033[0m", status.TotalRunning-len(status.Running)))
			}
			lines = append(lines, formatSummaryLine(status.Summary))
			lines = append(lines, fmt.Sprintf("  Watching build #%d…", b.Number))
			jobsRegion.SetLines(lines)
		} else {
			for _, fj := range status.NewlyFailed {
				fmt.Printf("  ✗ %s  %s  %s\n", fj.Name, fj.State, fj.ID)
			}
			line := fmt.Sprintf("Build #%d %s", b.Number, b.State)
			if summary := status.Summary.String(); summary != "" {
				line += " — " + summary
			}
			if line != lastLine {
				fmt.Printf("[%s] %s\n", time.Now().Format(time.TimeOnly), line)
				lastLine = line
			}
		}
	})

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

	// Flush the screen so final output is not overwritten.
	screen.Flush()

	// Plain mode: print a recap of all failed jobs at the end.
	if !tty {
		finalStatus := tracker.Update(finalBuild)
		if finalStatus.Summary.Failed > 0 {
			fmt.Println()
			fmt.Printf("Failed jobs (%d):\n", finalStatus.Summary.Failed)
			for _, j := range finalBuild.Jobs {
				if j.Type != "script" {
					continue
				}
				if j.State == "failed" || j.State == "timed_out" || j.SoftFailed {
					fmt.Printf("  ✗ %s  %s  %s\n", watch.JobDisplayName(j), j.State, j.ID)
				}
			}
		}
	}

	if finalBuild.State == "passed" {
		resultRegion.SetLines([]string{"", "✅ Preflight passed!"})
		return nil
	}
	resultRegion.SetLines([]string{"", fmt.Sprintf("❌ Preflight %s", finalBuild.State)})
	return fmt.Errorf("preflight build %s", finalBuild.State)
}

func formatFailedJob(fj watch.FailedJob) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("  \033[31m✗\033[0m %s", fj.Name))

	if fj.SoftFailed {
		parts = append(parts, " \033[33msoft failed\033[0m")
	} else {
		parts = append(parts, fmt.Sprintf("  \033[31m%s\033[0m", fj.State))
	}
	if fj.ExitStatus != nil && *fj.ExitStatus != 0 {
		parts = append(parts, fmt.Sprintf("  \033[31mexit %d\033[0m", *fj.ExitStatus))
	}
	if fj.Duration > 0 {
		parts = append(parts, fmt.Sprintf("  \033[2m(%s)\033[0m", fj.Duration))
	}
	parts = append(parts, fmt.Sprintf("  \033[2m%s\033[0m", fj.ID))
	return strings.Join(parts, "")
}

func formatSummaryLine(s watch.JobSummary) string {
	var parts []string
	if s.Passed > 0 {
		parts = append(parts, fmt.Sprintf("\033[32m%d passed\033[0m", s.Passed))
	}
	if s.Failed > 0 {
		parts = append(parts, fmt.Sprintf("\033[31m%d failed\033[0m", s.Failed))
	}
	if s.Scheduled > 0 {
		parts = append(parts, fmt.Sprintf("%d scheduled", s.Scheduled))
	}
	if s.Waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting", s.Waiting))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("  \033[90m◌\033[0m … %s", strings.Join(parts, ", "))
}
