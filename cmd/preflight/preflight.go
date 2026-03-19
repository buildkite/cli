package preflight

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/uuid"

	"github.com/buildkite/cli/v3/internal/build/view/shared"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/preflight"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	pkgValidation "github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
	"github.com/mattn/go-isatty"
)

type PreflightCmd struct {
	Pipeline string `help:"The pipeline to build. This can be a {pipeline slug} or {org slug}/{pipeline slug}." short:"p"`
	Branch   string `help:"The branch to build. Defaults to the current git branch." short:"b"`
	Commit   string `help:"The commit to build." short:"c" default:"HEAD"`
	Web      bool   `help:"Open the build in a web browser after creation." short:"w"`
	Watch    bool   `help:"Watch the build until completion." default:"true" negatable:""`
	Interval int    `help:"Polling interval in seconds when watching." default:"5"`
}

func (c *PreflightCmd) Help() string {
	return `Create a preflight build on a pipeline to validate your current changes before merging.
By default, it uses the current branch and HEAD commit from your local repository.
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

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := pkgValidation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

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

	// Resolve branch from flag or git repo
	branch := c.Branch
	if branch == "" {
		branch, err = currentBranch(f.GitRepository)
		if err != nil {
			return bkErrors.NewValidationError(
				err,
				"could not determine current branch",
				"Specify a branch with --branch (-b)",
				"Ensure you are in a git repository",
			)
		}
	}

	preflightID := uuid.New().String()

	// Snapshot the working tree into a temporary commit and push it
	fmt.Printf("Creating snapshot of working tree...\n")
	commit, err := preflight.Snapshot(branch, preflightID)
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to create preflight snapshot",
			"Ensure you have uncommitted or committed changes to snapshot",
			"Ensure you have push access to the remote repository",
		)
	}
	preflightBranch := "bk-preflight/" + preflightID
	fmt.Printf("Pushed snapshot %s → origin/%s\n", commit[:12], preflightBranch)

	// Wait for the webhook-triggered build to appear
	fmt.Printf("Waiting for build on %s (branch: %s)...\n", resolvedPipeline.Name, preflightBranch)

	var build buildkite.Build
	pollTimeout := 2 * time.Minute
	pollInterval := 3 * time.Second
	deadline := time.Now().Add(pollTimeout)

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
			Commit:      commit,
			ListOptions: buildkite.ListOptions{PerPage: 1},
		})
		if err != nil {
			return bkErrors.WrapAPIError(err, "polling for preflight build")
		}
		if len(builds) > 0 {
			build = builds[0]
			break
		}

		time.Sleep(pollInterval)
	}

	fmt.Printf("Preflight build found: %s\n", build.WebURL)

	if err := util.OpenInWebBrowser(c.Web, build.WebURL); err != nil {
		return bkErrors.NewInternalError(err, "failed to open web browser")
	}

	if !c.Watch {
		return nil
	}

	// Watch the build until it completes
	fmt.Printf("Watching build #%d...\n", build.Number)

	tty := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	ticker := time.NewTicker(time.Duration(c.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b, _, err := f.RestAPIClient.Builds.Get(ctx, resolvedPipeline.Org, resolvedPipeline.Name, fmt.Sprint(build.Number), nil)
			if err != nil {
				return bkErrors.WrapAPIError(err, "fetching build status")
			}

			summary := shared.BuildSummaryWithFailedJobs(&b, resolvedPipeline.Org, resolvedPipeline.Name)
			if tty {
				fmt.Print("\033[H\033[2J")
				fmt.Printf("%s\n", summary)
			} else {
				fmt.Printf("[%s] %s\n", time.Now().Format(time.RFC3339), summary)
			}

			if b.FinishedAt != nil {
				if b.State == "passed" {
					return nil
				}
				return fmt.Errorf("preflight build %s", b.State)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func currentBranch(repo *git.Repository) (string, error) {
	if repo == nil {
		return "", fmt.Errorf("not in a git repository")
	}
	head, err := repo.Head()
	if err != nil {
		return "", err
	}
	return head.Name().Short(), nil
}
