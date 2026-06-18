package browse

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/cli"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	git "github.com/go-git/go-git/v5"
	"github.com/pkg/browser"
)

// BrowseCmd opens Buildkite resources in a web browser, resolving the pipeline
// from the current project (or flags) the same way other commands do.
//
// When the pipeline page is opened (no build number, no --settings), the
// builds list is filtered to the current git branch by default. Use --branch to
// filter to a specific branch, or --all-branches to show the unfiltered list.
type BrowseCmd struct {
	// Build is an optional build reference. It may be a build number, an
	// "org/pipeline/number" slug, or a full buildkite.com build URL. When
	// omitted, the resolved pipeline page is opened instead.
	Build       string `arg:"" optional:"" help:"Build number, org/pipeline/number slug, or build URL to open. Omit to open the pipeline page."`
	Pipeline    string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Branch      string `help:"Filter the pipeline builds list to this branch. Defaults to the current git branch." short:"b"`
	AllBranches bool   `help:"Open the pipeline builds list without a branch filter." name:"all-branches"`
	Settings    bool   `help:"Open the pipeline's settings page." short:"s"`
	NoBrowser   bool   `help:"Print destination URL instead of opening the browser." short:"n" name:"no-browser"`
}

func (c *BrowseCmd) Help() string {
	return `Open Buildkite resources in your web browser.

Without arguments, the pipeline for the current project is resolved and opened,
with its builds list filtered to the current git branch.

Examples:
  # Open the current project's pipeline, filtered to the current branch
  $ bk browse

  # Open build #420 on the current project's pipeline
  $ bk browse 420

  # Open a build on a specific pipeline
  $ bk browse 420 -p monolith

  # Open a build by slug (bypasses project pipeline resolution)
  $ bk browse my-org/my-pipeline/420

  # Open the pipeline settings page
  $ bk browse -s

  # Filter the pipeline builds list to a specific branch
  $ bk browse -b main

  # Open the pipeline builds list across all branches
  $ bk browse --all-branches

  # Print the URL instead of opening a browser
  $ bk browse 420 -n
`
}

func (c *BrowseCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	// --settings targets the pipeline settings, so it cannot be combined with a
	// build reference (which targets a specific build).
	if c.Settings && c.Build != "" {
		return fmt.Errorf("--settings/-s opens the pipeline settings page and cannot be used with a build number; pass only one of them")
	}
	// Branch filtering only applies to the pipeline builds list page, not a
	// specific build or the settings page.
	if c.Branch != "" && c.Build != "" {
		return fmt.Errorf("--branch/-b filters the pipeline builds list and cannot be used with a build number; pass only one of them")
	}
	if c.AllBranches && c.Build != "" {
		return fmt.Errorf("--all-branches filters the pipeline builds list and cannot be used with a build number; pass only one of them")
	}
	if c.Branch != "" && c.AllBranches {
		return fmt.Errorf("--branch and --all-branches cannot be used together")
	}
	if (c.Branch != "" || c.AllBranches) && c.Settings {
		return fmt.Errorf("branch flags only apply to the pipeline builds list and cannot be used with --settings")
	}

	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Resolve pipeline using the same chain as `bk build view`: explicit flag
	// first, then configured pipeline, then the current repository.
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(c.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f))),
	)

	var url string
	if c.Build != "" {
		// A build reference was given. The build resolver handles plain build
		// numbers (resolving the pipeline via pipelineRes), as well as
		// org/pipeline/number slugs and full URLs.
		buildRes := buildResolver.NewAggregateResolver(
			buildResolver.ResolveFromPositionalArgument([]string{c.Build}, 0, pipelineRes.Resolve, f.Config),
		)
		bld, err := buildRes.Resolve(ctx)
		if err != nil {
			return err
		}
		if bld == nil {
			return fmt.Errorf("could not resolve build %q", c.Build)
		}
		url = buildURL(bld.Organization, bld.Pipeline, bld.BuildNumber)
	} else {
		p, err := pipelineRes.Resolve(ctx)
		if err != nil {
			return err
		}
		if p == nil {
			return fmt.Errorf("failed to resolve a pipeline; pass one with --pipeline/-p")
		}
		switch {
		case c.Settings:
			url = settingsURL(p.Org, p.Name)
		case c.Branch != "":
			url = pipelineBranchURL(p.Org, p.Name, c.Branch)
		case c.AllBranches:
			url = pipelineURL(p.Org, p.Name)
		default:
			// Default: filter to the current git branch. If it can't be
			// resolved (e.g. not in a repo), fall back to the unfiltered list
			// rather than failing.
			branch, berr := currentBranch(ctx, f.GitRepository)
			if berr != nil || branch == "" {
				if berr != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not determine current git branch, showing all builds: %v\n", berr)
				}
				url = pipelineURL(p.Org, p.Name)
			} else {
				url = pipelineBranchURL(p.Org, p.Name, branch)
			}
		}
	}

	if c.NoBrowser {
		fmt.Println(url)
		return nil
	}

	fmt.Printf("Opening %s in your browser\n", url)
	return browser.OpenURL(url)
}

// currentBranch resolves the current git branch, preferring the in-process
// repository handle and falling back to invoking git directly.
func currentBranch(ctx context.Context, repo *git.Repository) (string, error) {
	if repo != nil {
		head, err := repo.Head()
		if err != nil {
			return "", err
		}
		return head.Name().Short(), nil
	}

	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "--quiet", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func pipelineURL(org, pipeline string) string {
	return fmt.Sprintf("https://buildkite.com/%s/%s", org, pipeline)
}

func buildURL(org, pipeline string, number int) string {
	return fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d", org, pipeline, number)
}

func settingsURL(org, pipeline string) string {
	return fmt.Sprintf("https://buildkite.com/%s/%s/settings", org, pipeline)
}

func pipelineBranchURL(org, pipeline, branch string) string {
	return fmt.Sprintf("https://buildkite.com/%s/%s?branch=%s", org, pipeline, branch)
}
