package preflight

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"

	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/preflight"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	pkgValidation "github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
)

type PreflightCmd struct {
	Pipeline string `help:"The pipeline to build. This can be a {pipeline slug} or {org slug}/{pipeline slug}." short:"p"`
	Web      bool   `help:"Open the build in a web browser after creation." short:"w"`
	Watch    bool   `help:"Watch the build until completion." default:"true" negatable:""`
	Interval float64 `help:"Polling interval in seconds when watching." default:"0.5"`
}

const maxPollingErrors = 10

func (c *PreflightCmd) Help() string {
	return `Create a preflight build on a pipeline to validate your current changes before merging.
It snapshots your working tree (including untracked files), creates a temporary commit, and pushes it to the configured repository origin on a bk-preflight/* branch.
The build is watched until completion and the final status is reported.

Examples:
  # Run a preflight build on the current branch
  $ bk preflight -p my-pipeline

  # Run a preflight build with org/pipeline
  $ bk preflight -p my-org/my-pipeline

  # Run a preflight build without watching
  $ bk preflight -p my-pipeline --no-watch

  # Open the build in the browser
  $ bk preflight -p my-pipeline --web`
}

func (c *PreflightCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to initialize CLI", "This is likely a bug", "Report to Buildkite")
	}

	if !f.Config.HasExperiment("preflight") {
		return bkErrors.NewValidationError(
			fmt.Errorf("experiment not enabled"),
			"the preflight command requires the 'preflight' experiment to be enabled",
			"Run: bk config set experiments preflight",
			"Or set BUILDKITE_EXPERIMENTS=preflight",
		)
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := pkgValidation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()
	if err := requireGitRepository(f.GitRepository); err != nil {
		return bkErrors.NewValidationError(
			err,
			"preflight must be run from a git repository",
			"Run this command from inside a git repository",
			"Ensure you are in a repository with a .git directory",
		)
	}

	resolvers := resolver.NewAggregateResolver(
		resolver.ResolveFromFlag(c.Pipeline, f.Config),
		resolver.ResolveFromConfig(f.Config, resolver.PickOneWithFactory(f)),
		resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOneWithFactory(f))),
	)

	resolvedPipeline, err := resolvers.Resolve(ctx)
	if err != nil {
		return err
	}
	if resolvedPipeline == nil {
		return bkErrors.NewResourceNotFoundError(
			nil,
			"could not resolve a pipeline",
			"Specify a pipeline with --pipeline (-p)",
			"Run 'bk pipeline list' to see available pipelines",
		)
	}

	preflightID := uuid.New().String()

	// Snapshot the working tree into a temporary commit and push it
	fmt.Println()
	fmt.Println("Preparing preflight with uncommitted changes...")
	fmt.Println()

	// TODO: Snapshot should return branch.
	result, err := preflight.Snapshot(preflightID)
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to create preflight snapshot",
			"Ensure you have uncommitted or committed changes to snapshot",
			"Ensure you have push access to the remote repository",
		)
	}
	preflightBranch := "bk-preflight/" + preflightID

	printSnapshotSummary(result, preflightBranch)
	fmt.Printf("  Pushing to origin...\n")

	// Wait for the webhook-triggered build to appear
	fmt.Printf("  Waiting for build...\n")

	var build buildkite.Build
	pollTimeout := 30 * time.Second
	pollInterval := 1 * time.Second
	deadline := time.Now().Add(pollTimeout)
	pollErrorCount := 0

	for {
		if time.Now().After(deadline) {
			return bkErrors.NewInternalError(
				fmt.Errorf("timed out after %s", pollTimeout),
				"no build appeared for branch "+preflightBranch,
				"Check that the pipeline has a webhook configured for push events",
				"Check that branch filtering allows bk-preflight/* branches",
			)
		}

		builds, _, err := f.RestAPIClient.Builds.ListByPipeline(ctx, resolvedPipeline.Org, resolvedPipeline.Name, &buildkite.BuildsListOptions{
			Branch:      []string{preflightBranch},
			Commit:      result.Commit,
			ListOptions: buildkite.ListOptions{PerPage: 1},
		})
		if err != nil {
			if pollErr := recordPollingError(err, &pollErrorCount, "polling for preflight build"); pollErr != nil {
				return pollErr
			}
			time.Sleep(pollInterval)
			continue
		}
		_ = recordPollingError(nil, &pollErrorCount, "")
		if len(builds) > 0 {
			build = builds[0]
			break
		}

		time.Sleep(pollInterval)
	}

	fmt.Printf("  Build:   #%d → %s\n", build.Number, build.WebURL)

	if err := util.OpenInWebBrowser(c.Web, build.WebURL); err != nil {
		return bkErrors.NewInternalError(err, "failed to open web browser")
	}

	if !c.Watch {
		return nil
	}

	// Watch the build until it completes
	fmt.Println()

	tty := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	ticker := time.NewTicker(time.Duration(c.Interval * float64(time.Second)))
	defer ticker.Stop()
	watchPollErrorCount := 0

	if tty {
		return c.watchLive(ctx, f, resolvedPipeline, build, ticker, &watchPollErrorCount)
	}
	return c.watchPlain(ctx, f, resolvedPipeline, build, ticker, &watchPollErrorCount)
}

// watchLive uses a LiveWriter to render an in-place updating display on TTYs.
// Completed jobs are promoted to permanent scrollback; in-progress jobs are
// redrawn in a live region at the bottom.
func (c *PreflightCmd) watchLive(
	ctx context.Context,
	f *factory.Factory,
	pl *pipeline.Pipeline,
	build buildkite.Build,
	ticker *time.Ticker,
	pollErrorCount *int,
) error {
	lw := preflight.NewLiveWriter(os.Stdout)
	promoted := map[string]bool{}
	promotedGroups := map[string]bool{}
	promotedTests := map[string]bool{}
	testHeaderShown := false
	tick := 0
	totalPassed := 0

	for {
		select {
		case <-ticker.C:
			b, _, err := f.RestAPIClient.Builds.Get(ctx, pl.Org, pl.Name, fmt.Sprint(build.Number), nil)
			if err != nil {
				if pollErr := recordPollingError(err, pollErrorCount, "fetching build status"); pollErr != nil {
					return pollErr
				}
				continue
			}
			_ = recordPollingError(nil, pollErrorCount, "")

			var live []string
			var scheduled, waiting int

			// Collect parallel groups by display name.
			groups := map[string]*preflight.ParallelGroup{}
			groupOrder := 0

			for _, j := range b.Jobs {
				// Skip non-command types that aren't interesting once terminal.
				if j.Type == "waiter" || j.Type == "manual" || j.Type == "trigger" {
					if preflight.IsJobTerminal(j) {
						continue
					}
				}

				// Never show broken jobs (pipeline upload failures etc.)
				if j.State == "broken" {
					continue
				}

				name := preflight.JobDisplayName(j)

				// Parallel jobs get aggregated into a group summary.
				if preflight.IsParallelJob(j) {
					g, ok := groups[name]
					if !ok {
						g = &preflight.ParallelGroup{Name: name, Order: groupOrder}
						groups[name] = g
						groupOrder++
					}
					g.Total++
					switch {
					case j.State == "passed":
						g.Passed++
					case j.State == "failed" || j.State == "timed_out" || j.SoftFailed:
						g.Failed++
						g.FailedJobs = append(g.FailedJobs, j)
					case preflight.IsJobActive(j):
						g.Running++
					default:
						g.Waiting++
					}
					continue
				}

				// Non-parallel jobs handled individually.
				if preflight.IsJobTerminal(j) && !promoted[j.ID] {
					promoted[j.ID] = true
					if j.State == "passed" {
						totalPassed++
					} else {
						lw.Println(preflight.FormatTerminalJob(j))
					}
				} else if preflight.IsJobActive(j) {
					live = append(live, preflight.FormatLiveJob(j))
				} else if !preflight.IsJobTerminal(j) {
					if j.State == "scheduled" || j.State == "assigned" || j.State == "accepted" {
						scheduled++
					} else {
						waiting++
					}
				}
			}

			// Render parallel groups sorted by insertion order.
			sortedGroups := make([]*preflight.ParallelGroup, 0, len(groups))
			for _, g := range groups {
				sortedGroups = append(sortedGroups, g)
			}
			sort.Slice(sortedGroups, func(i, j int) bool {
				return sortedGroups[i].Order < sortedGroups[j].Order
			})
			for _, g := range sortedGroups {
				done := g.Passed + g.Failed
				if done == g.Total && !promotedGroups[g.Name] {
					// Fully complete — promote to scrollback.
					promotedGroups[g.Name] = true
					if g.Failed > 0 {
						for _, line := range preflight.FormatParallelGroupTerminal(g) {
							lw.Println(line)
						}
					} else {
						totalPassed += g.Total
					}
				} else if g.Running > 0 || g.Failed > 0 {
					// Show in live region only if jobs are active or have failures.
					live = append(live, preflight.FormatParallelGroupLive(g)...)
				}
			}

			// TODO: Replace mock with real API call to
			// GET /v2/analytics/organizations/{org}/builds/{build-uuid}/tests?state=enabled
			mockTests := preflight.MockFailedTests()
			var newTests []preflight.FailedTest
			for _, t := range mockTests {
				if !promotedTests[t.ID] {
					newTests = append(newTests, t)
				}
			}
			if len(newTests) > 0 {
				if !testHeaderShown {
					testHeaderShown = true
					lw.Println(preflight.FormatFailedTestsHeader(len(mockTests)))
				}
				display := newTests
				if len(promotedTests) + len(newTests) > preflight.MaxDisplayedTests {
					remaining := preflight.MaxDisplayedTests - len(promotedTests)
					if remaining > 0 {
						display = newTests[:remaining]
					} else {
						display = nil
					}
				}
				if len(display) > 0 {
					for _, line := range preflight.FormatFailedTestsTable(display) {
						lw.Println(line)
					}
				}
				for _, t := range newTests {
					promotedTests[t.ID] = true
				}
				overflow := len(mockTests) - preflight.MaxDisplayedTests
				if overflow > 0 {
					lw.Println(preflight.FormatFailedTestsOverflow(overflow, b.Number))
				}
			}

			// Show a collapsed summary for passed/scheduled/waiting jobs.
			var parts []string
			if totalPassed > 0 {
				parts = append(parts, fmt.Sprintf("\033[32m%d passed\033[0m", totalPassed))
			}
			if scheduled > 0 {
				parts = append(parts, fmt.Sprintf("%d scheduled", scheduled))
			}
			if waiting > 0 {
				parts = append(parts, fmt.Sprintf("%d waiting", waiting))
			}
			if len(parts) > 0 {
				live = append(live,
					fmt.Sprintf("  \033[90m◌\033[0m … %s", strings.Join(parts, ", ")))
			}

			live = append(live, "")
			live = append(live,
				fmt.Sprintf("  %s Watching build #%d…",
					preflight.Spinner(tick), b.Number))
			live = append(live,
				"\033[90m  Use `bk log view <job-id>` to view logs\033[0m")

			lw.SetLines(live)

			if b.FinishedAt != nil {
				lw.Flush()
				return printBuildResult(b)
			}

			tick++

		case <-ctx.Done():
			return nil
		}
	}
}

// watchPlain is the non-TTY fallback that prints a one-line status on each
// poll without ANSI escape codes.
func (c *PreflightCmd) watchPlain(
	ctx context.Context,
	f *factory.Factory,
	pl *pipeline.Pipeline,
	build buildkite.Build,
	ticker *time.Ticker,
	pollErrorCount *int,
) error {
	var lastLine string
	reported := map[string]bool{}
	for {
		select {
		case <-ticker.C:
			b, _, err := f.RestAPIClient.Builds.Get(ctx, pl.Org, pl.Name, fmt.Sprint(build.Number), nil)
			if err != nil {
				if pollErr := recordPollingError(err, pollErrorCount, "fetching build status"); pollErr != nil {
					return pollErr
				}
				continue
			}
			_ = recordPollingError(nil, pollErrorCount, "")

			var running, passed, failed int
			for _, j := range b.Jobs {
				if j.Type != "script" || j.State == "broken" {
					continue
				}
				switch j.State {
				case "running":
					running++
				case "passed":
					passed++
				case "failed", "timed_out":
					failed++
				}
				if (j.State == "failed" || j.State == "timed_out" || j.SoftFailed) && !reported[j.ID] {
					reported[j.ID] = true
					fmt.Printf("  ✗ %s  %s  %s\n", preflight.JobDisplayName(j), j.State, j.ID)
				}
			}
			line := fmt.Sprintf("Build #%d %s — %d passed, %d failed, %d running",
				b.Number, b.State, passed, failed, running)
			if line != lastLine {
				fmt.Printf("[%s] %s\n", time.Now().Format(time.RFC3339), line)
				lastLine = line
			}

			if b.FinishedAt != nil {
				return printBuildResult(b)
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func printBuildResult(b buildkite.Build) error {
	fmt.Println()
	if b.State == "passed" {
		fmt.Println("✅ Preflight passed!")
		return nil
	}
	fmt.Printf("❌ Preflight %s\n", b.State)
	return fmt.Errorf("preflight build %s", b.State)
}

func requireGitRepository(repo *git.Repository) error {
	if repo == nil {
		return fmt.Errorf("not in a git repository")
	}

	return nil
}

func printSnapshotSummary(result *preflight.SnapshotResult, branch string) {
	width := terminalWidth()

	// File list box
	if len(result.Files) > 0 {
		label := fmt.Sprintf(" Files  %d file", len(result.Files))
		if len(result.Files) != 1 {
			label += "s"
		}
		label += " "

		// Top border: ┌─ Files  N files ─────...─┐
		topInner := width - 4 // subtract ┌─ and ─┐
		dashesAfterLabel := topInner - len(label)
		if dashesAfterLabel < 1 {
			dashesAfterLabel = 1
		}
		fmt.Printf("┌─%s%s┐\n", label, strings.Repeat("─", dashesAfterLabel))

		// File rows
		for _, f := range result.Files {
			line := fmt.Sprintf("   %s %s", f.StatusSymbol(), f.Path)
			padding := width - 2 - len(line) // subtract │ and │
			if padding < 0 {
				padding = 0
			}
			fmt.Printf("│%s%s│\n", line, strings.Repeat(" ", padding))
		}

		// Bottom border
		fmt.Printf("└%s┘\n", strings.Repeat("─", width-2))
	}

	fmt.Println()
	fmt.Printf("  Commit:  %s\n", result.Commit[:10])
	fmt.Printf("  Ref:     %s\n", result.Ref)
}

func terminalWidth() int {
	fd := os.Stdout.Fd()
	if isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd) {
		if w, _, err := term.GetSize(int(fd)); err == nil && w > 0 {
			return w
		}
	}
	return 80
}

func recordPollingError(err error, errorCount *int, operation string) error {
	if err == nil {
		*errorCount = 0
		return nil
	}

	*errorCount++
	if *errorCount >= maxPollingErrors {
		return bkErrors.NewInternalError(
			err,
			fmt.Sprintf("%s failed %d times", operation, maxPollingErrors),
			"Buildkite API may be unavailable or your network may be unstable",
			"Retry the preflight command once connectivity is restored",
		)
	}

	fmt.Fprintf(os.Stderr, "WARNING: %s failed (%d/%d): %v\n", operation, *errorCount, maxPollingErrors, err)
	return nil
}
