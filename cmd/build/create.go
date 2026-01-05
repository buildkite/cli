package build

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type CreateCmd struct {
	Message             string   `help:"Description of the build. If left blank, the commit message will be used once the build starts." short:"m"`
	Commit              string   `help:"The commit to build." short:"c" default:"HEAD"`
	Branch              string   `help:"The branch to build. Defaults to the default branch of the pipeline." short:"b"`
	Author              string   `help:"Author of the build. Supports: \"Name <email>\", \"email@domain.com\", \"Full Name\", or \"username\"" short:"a"`
	Web                 bool     `help:"Open the build in a web browser after it has been created." short:"w"`
	Pipeline            string   `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Env                 []string `help:"Set environment variables for the build (KEY=VALUE)" short:"e"`
	Metadata            []string `help:"Set metadata for the build (KEY=VALUE)" short:"M"`
	IgnoreBranchFilters bool     `help:"Ignore branch filters for the pipeline" short:"i"`
	EnvFile             string   `help:"Set the environment variables for the build via an environment file" short:"f"`
}

func (c *CreateCmd) Help() string {
	return `The web URL to the build will be printed to stdout.

Examples:
  # Create a new build
  $ bk build create

  # Create a new build with environment variables set
  $ bk build create -e "FOO=BAR" -e "BAR=BAZ"

  # Create a new build with metadata
  $ bk build create -M "key=value" -M "foo=bar"`
}

func (c *CreateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	// Initialize factory
	f, err := factory.New()
	if err != nil {
		return bkErrors.NewInternalError(err, "failed to initialize CLI", "This is likely a bug", "Report to Buildkite")
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	resolvers := resolver.NewAggregateResolver(
		resolver.ResolveFromFlag(c.Pipeline, f.Config),
		resolver.ResolveFromConfig(f.Config, resolver.PickOneWithFactory(f)),
		resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOneWithFactory(f), f.GitRepository != nil)),
	)

	resolvedPipeline, err := resolvers.Resolve(ctx)
	if err != nil {
		return err // Already wrapped by resolver
	}
	if resolvedPipeline == nil {
		return bkErrors.NewResourceNotFoundError(
			nil,
			"could not resolve a pipeline",
			"Specify a pipeline with --pipeline (-p)",
			"Run 'bk pipeline list' to see available pipelines",
		)
	}

	confirmed, err := bkIO.Confirm(f, fmt.Sprintf("Create new build on %s?", resolvedPipeline.Name))
	if err != nil {
		return bkErrors.NewUserAbortedError(err, "confirmation canceled")
	}

	if !confirmed {
		fmt.Println("Build creation canceled")
		return nil
	}

	// Process environment variables
	envMap := make(map[string]string)
	for _, e := range c.Env {
		key, value, _ := strings.Cut(e, "=")
		envMap[key] = value
	}

	// Process metadata variables
	metaDataMap := make(map[string]string)
	for _, m := range c.Metadata {
		key, value, _ := strings.Cut(m, "=")
		metaDataMap[key] = value
	}

	// Process environment file if specified
	if c.EnvFile != "" {
		file, err := os.Open(c.EnvFile)
		if err != nil {
			return bkErrors.NewValidationError(
				err,
				fmt.Sprintf("could not open environment file: %s", c.EnvFile),
				"Check that the file exists and is readable",
			)
		}
		defer file.Close()

		content := bufio.NewScanner(file)
		for content.Scan() {
			key, value, _ := strings.Cut(content.Text(), "=")
			envMap[key] = value
		}

		if err := content.Err(); err != nil {
			return bkErrors.NewValidationError(
				err,
				"error reading environment file",
				"Ensure the file contains valid environment variables in KEY=VALUE format",
			)
		}
	}

	return createBuild(ctx, resolvedPipeline.Org, resolvedPipeline.Name, f, c.Message, c.Commit, c.Branch, c.Web, envMap, metaDataMap, c.IgnoreBranchFilters, c.Author)
}

func parseAuthor(author string) buildkite.Author {
	if author == "" {
		return buildkite.Author{}
	}

	// Check for Git-style format: "Name <email@domain.com>"
	if strings.Contains(author, "<") && strings.Contains(author, ">") {
		parts := strings.Split(author, "<")
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			email := strings.TrimSpace(strings.Trim(parts[1], ">"))
			if name != "" && email != "" {
				return buildkite.Author{Name: name, Email: email}
			}
		}
	}

	// Check for email-only format
	if strings.Contains(author, "@") && strings.Contains(author, ".") && !strings.Contains(author, " ") {
		return buildkite.Author{Email: author}
	}

	// Check for name format (contains spaces but no email)
	if strings.Contains(author, " ") {
		return buildkite.Author{Name: author}
	}

	// Default to username
	return buildkite.Author{Username: author}
}

func createBuild(ctx context.Context, org string, pipeline string, f *factory.Factory, message string, commit string, branch string, web bool, env map[string]string, metaData map[string]string, ignoreBranchFilters bool, author string) error {
	var actionErr error
	var build buildkite.Build
	spinErr := bkIO.SpinWhile(f, fmt.Sprintf("Starting new build for %s", pipeline), func() {
		branch = strings.TrimSpace(branch)
		if len(branch) == 0 {
			p, _, err := f.RestAPIClient.Pipelines.Get(ctx, org, pipeline)
			if err != nil {
				actionErr = bkErrors.WrapAPIError(err, "fetching pipeline information")
				return
			}

			// Check if the pipeline has a default branch set
			if p.DefaultBranch == "" {
				actionErr = bkErrors.NewValidationError(
					nil,
					fmt.Sprintf("No default branch set for pipeline %s", pipeline),
					"Please specify a branch using --branch (-b)",
					"Set a default branch in your pipeline settings on Buildkite",
				)
				return
			}
			branch = p.DefaultBranch
		}

		newBuild := buildkite.CreateBuild{
			Message:                     message,
			Commit:                      commit,
			Branch:                      branch,
			Author:                      parseAuthor(author),
			Env:                         env,
			MetaData:                    metaData,
			IgnorePipelineBranchFilters: ignoreBranchFilters,
		}

		var err error
		build, _, err = f.RestAPIClient.Builds.Create(ctx, org, pipeline, newBuild)
		if err != nil {
			actionErr = bkErrors.WrapAPIError(err, "creating build")
			return
		}
	})
	if spinErr != nil {
		return bkErrors.NewInternalError(spinErr, "error in spinner UI")
	}

	if actionErr != nil {
		return actionErr
	}

	if build.WebURL == "" {
		return bkErrors.NewAPIError(
			nil,
			"build was created but no URL was returned",
			"This may be due to an API version mismatch",
		)
	}

	fmt.Printf("%s\n", renderResult(fmt.Sprintf("Build created: %s", build.WebURL)))

	if err := util.OpenInWebBrowser(web, build.WebURL); err != nil {
		return bkErrors.NewInternalError(err, "failed to open web browser")
	}

	return nil
}

func renderResult(result string) string {
	return result
}
