package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	buildstate "github.com/buildkite/cli/v3/internal/build/state"
	"github.com/buildkite/cli/v3/internal/build/view/shared"
	"github.com/buildkite/cli/v3/internal/build/watch"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/programstatus"
	"github.com/buildkite/cli/v3/internal/validation"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	pkgValidation "github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-isatty"
)

type WatchCmd struct {
	BuildNumber string        `arg:"" optional:"" help:"Build number to watch (omit for most recent build)"`
	Pipeline    string        `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Branch      string        `help:"The branch to watch builds for." short:"b"`
	Interval    int           `help:"Polling interval in seconds" default:"1"`
	Timeout     time.Duration `help:"Maximum time to resolve and watch the build (e.g. 30s, 20m). 0 means no timeout. Timing out does not cancel the build." default:"0"`
}

func (c *WatchCmd) Validate() error {
	if c.Timeout < 0 {
		return fmt.Errorf("timeout must not be negative")
	}
	return nil
}

func (c *WatchCmd) Help() string {
	return `
Examples:
  # Watch the most recent build for the current branch
  $ bk build watch --pipeline my-pipeline

  # Watch a specific build
  $ bk build watch 429 --pipeline my-pipeline

  # Watch the most recent build on a specific branch
  $ bk build watch -b feature-x --pipeline my-pipeline

  # Watch a build on a specific pipeline
  $ bk build watch --pipeline my-pipeline

  # Set a custom polling interval (in seconds)
  $ bk build watch --interval 5 --pipeline my-pipeline

  # Stop watching after 20 minutes without canceling the build
  $ bk build watch 429 --pipeline my-pipeline --timeout 20m`
}

func (c *WatchCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := pkgValidation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	tty := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	// Validate command options
	v := validation.New()
	v.AddRule("Interval", validation.MinValue(1))
	if err := v.Validate(map[string]interface{}{
		"Interval": c.Interval,
	}); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
	)

	optionsResolver := options.AggregateResolver{
		options.ResolveBranchFromFlag(c.Branch),
		options.ResolveBranchFromRepository(f.GitRepository),
	}

	args := []string{}
	if c.BuildNumber != "" {
		args = []string{c.BuildNumber}
	}
	buildRes := buildResolver.NewAggregateResolver(
		buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
		buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...),
	)

	bld, err := buildRes.Resolve(ctx)
	if err == nil && bld == nil {
		err = fmt.Errorf("no running builds found")
	}
	if err != nil {
		programstatus.Report(watchStoppedStatus(ctx, err))
		return err
	}

	fmt.Printf("Watching build %d on %s/%s\n", bld.BuildNumber, bld.Organization, bld.Pipeline)

	interval := time.Duration(c.Interval) * time.Second

	_, err = watch.WatchBuild(ctx, f.RestAPIClient, bld.Organization, bld.Pipeline, bld.BuildNumber, interval, func(b buildkite.Build) error {
		b.Jobs = watchJobs(b.Jobs)
		programstatus.Report(watchStatus(bld.Pipeline, b))
		summary := shared.BuildSummaryWithJobs(&b, bld.Organization, bld.Pipeline)
		if tty {
			fmt.Print("\033[H\033[2J")
			fmt.Printf("%s\n", summary)
		} else {
			fmt.Printf("[%s] %s\n", time.Now().Format(time.RFC3339), summary)
		}
		return nil
	})
	if err != nil {
		programstatus.Report(watchStoppedStatus(ctx, err))
	}
	if errors.Is(err, context.Canceled) {
		// Exit as if interrupted so "bk build watch && deploy" does not deploy.
		return bkErrors.NewUserAbortedError(err, "build watch canceled by user")
	}

	return err
}

// watchStoppedStatus describes a watch that ended with err before the build
// finished. If ctx is done the user stopped the watch, which does not stop the
// build, so bk is idle. Check ctx rather than err: repeated poll request
// timeouts also wrap context.DeadlineExceeded but mean the watch failed.
func watchStoppedStatus(ctx context.Context, err error) ansi.ProgramStatus {
	if ctx.Err() != nil {
		return ansi.ProgramStatus{State: ansi.ProgramStateIdle}
	}
	return ansi.ProgramStatus{State: ansi.ProgramStateError, Message: err.Error()}
}

// watchStatus describes a watched build for the terminal: working with job
// progress until the build finishes, then done, or error if it failed or was
// canceled.
func watchStatus(pipeline string, b buildkite.Build) ansi.ProgramStatus {
	s := ansi.ProgramStatus{
		State:   ansi.ProgramStateWorking,
		Message: fmt.Sprintf("%s #%d %s", pipeline, b.Number, b.State),
	}

	if b.FinishedAt != nil || buildstate.IsTerminal(buildstate.State(b.State)) {
		s.State = ansi.ProgramStateDone
		if b.State == string(buildstate.Failed) || b.State == string(buildstate.Canceled) {
			s.State = ansi.ProgramStateError
		}
		return s
	}

	var total, finished int
	for _, j := range b.Jobs {
		if j.Type != "script" || j.Retried {
			continue
		}
		total++
		switch j.State {
		case "passed", "failed", "timed_out", "canceled", "expired", "skipped":
			finished++
		}
	}
	if total > 0 {
		s.Progress = finished * 100 / total
		s.HasProgress = true
	}
	return s
}

func watchJobs(jobs []buildkite.Job) []buildkite.Job {
	out := make([]buildkite.Job, 0, len(jobs))
	for _, j := range jobs {
		if j.State == "broken" {
			continue
		}
		out = append(out, j)
	}
	return out
}
