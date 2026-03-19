package preflight

import (
	"context"
	"fmt"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	bkIO "github.com/buildkite/cli/v3/internal/io"
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

	// Snapshot the working tree into a temporary commit and push it
	fmt.Printf("Creating snapshot of working tree...\n")
	commit, err := preflight.Snapshot(branch)
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to create preflight snapshot",
			"Ensure you have uncommitted or committed changes to snapshot",
			"Ensure you have push access to the remote repository",
		)
	}
	fmt.Printf("Pushed snapshot %s → origin/%s\n", commit[:12], branch)

	// Create the build
	var build buildkite.Build
	var actionErr error
	spinErr := bkIO.SpinWhile(f, fmt.Sprintf("Starting preflight build on %s (branch: %s)", resolvedPipeline.Name, branch), func() {
		newBuild := buildkite.CreateBuild{
			Message: fmt.Sprintf("Preflight build for %s", branch),
			Commit:  commit,
			Branch:  branch,
		}
		var createErr error
		build, _, createErr = f.RestAPIClient.Builds.Create(ctx, resolvedPipeline.Org, resolvedPipeline.Name, newBuild)
		if createErr != nil {
			actionErr = bkErrors.WrapAPIError(createErr, "creating preflight build")
		}
	})
	if spinErr != nil {
		return bkErrors.NewInternalError(spinErr, "error in spinner UI")
	}
	if actionErr != nil {
		return actionErr
	}

	fmt.Printf("Preflight build created: %s\n", build.WebURL)

	if err := util.OpenInWebBrowser(c.Web, build.WebURL); err != nil {
		return bkErrors.NewInternalError(err, "failed to open web browser")
	}

	if !c.Watch {
		return nil
	}

	// Watch the build until it completes
	fmt.Printf("Watching build #%d...\n", build.Number)

	ticker := time.NewTicker(time.Duration(c.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b, _, err := f.RestAPIClient.Builds.Get(ctx, resolvedPipeline.Org, resolvedPipeline.Name, fmt.Sprint(build.Number), nil)
			if err != nil {
				return bkErrors.WrapAPIError(err, "fetching build status")
			}

			if b.FinishedAt != nil {
				if b.State == "passed" {
					fmt.Printf("✅ Preflight build #%d passed\n", build.Number)
					return nil
				}
				fmt.Printf("❌ Preflight build #%d %s\n", build.Number, b.State)
				fmt.Printf("   %s\n", build.WebURL)
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
