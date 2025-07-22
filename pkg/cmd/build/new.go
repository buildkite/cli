package build

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/internal/validation/scopes"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCmdBuildNew(f *factory.Factory) *cobra.Command {
	var branch string
	var commit string
	var message string
	var pipeline string
	var confirmed bool
	var web bool
	var ignoreBranchFilters bool
	var env []string
	var metaData []string
	envMap := make(map[string]string)
	metaDataMap := make(map[string]string)
	var envFile string
	var author string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "new [flags]",
		Short:                 "Create a new build",
		Args:                  cobra.NoArgs,
		Long: heredoc.Doc(`
			Create a new build on a pipeline.
			The web URL to the build will be printed to stdout.

			## To create a new build
			$ bk build new

			## To create a new build with environment variables set
			$ bk build new -e "FOO=BAR" -e "BAR=BAZ"

			## To create a new build with metadata
			$ bk build new -M "key=value" -M "foo=bar"
		`),
		PreRunE: bkErrors.WrapRunE(func(cmd *cobra.Command, args []string) error {
			// Get the command's required and optional scopes
			cmdScopes := scopes.GetCommandScopes(cmd)

			// Get the token scopes from the factory
			tokenScopes := f.Config.GetTokenScopes()
			if len(tokenScopes) == 0 {
				return bkErrors.NewAuthenticationError(
					nil,
					"no scopes found in token",
					"Please ensure you're using a token with appropriate scopes",
					"Run 'bk configure' to update your API token",
				)
			}

			// Validate the scopes
			if err := scopes.ValidateScopes(cmdScopes, tokenScopes); err != nil {
				return bkErrors.NewPermissionDeniedError(
					err,
					"insufficient token permissions",
					"Your API token needs the 'write_builds' scope to create builds",
					"Create a new token with the required permissions in your Buildkite account settings",
				)
			}

			return nil
		}),
		RunE: bkErrors.WrapRunE(func(cmd *cobra.Command, args []string) error {
			resolvers := resolver.NewAggregateResolver(
				resolver.ResolveFromFlag(pipeline, f.Config),
				resolver.ResolveFromConfig(f.Config, resolver.PickOne),
				resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOne, f.GitRepository != nil)),
			)

			resolvedPipeline, err := resolvers.Resolve(cmd.Context())
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

			err = bk_io.Confirm(&confirmed, fmt.Sprintf("Create new build on %s?", resolvedPipeline.Name))
			if err != nil {
				return bkErrors.NewUserAbortedError(err, "confirmation canceled")
			}

			if confirmed {
				// Process environment variables
				for _, e := range env {
					key, value, _ := strings.Cut(e, "=")
					envMap[key] = value
				}

				// Process metadata variables
				for _, m := range metaData {
					key, value, _ := strings.Cut(m, "=")
					metaDataMap[key] = value
				}

				// Process environment file if specified
				if envFile != "" {
					file, err := os.Open(envFile)
					if err != nil {
						return bkErrors.NewValidationError(
							err,
							fmt.Sprintf("could not open environment file: %s", envFile),
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

				return newBuild(cmd.Context(), resolvedPipeline.Org, resolvedPipeline.Name, f, message, commit, branch, web, envMap, metaDataMap, ignoreBranchFilters, author)
			} else {
				// User chose not to proceed - provide feedback
				fmt.Fprintf(cmd.OutOrStdout(), "Build creation canceled\n")
				return nil
			}
		}),
	}

	cmd.Annotations = map[string]string{
		"requiredScopes": string(scopes.WriteBuilds),
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Description of the build. If left blank, the commit message will be used once the build starts.")
	cmd.Flags().StringVarP(&commit, "commit", "c", "HEAD", "The commit to build.")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "The branch to build. Defaults to the default branch of the pipeline.")
	cmd.Flags().StringVarP(&author, "author", "a", "", "Author of the build. Supports: \"Name <email>\", \"email@domain.com\", \"Full Name\", or \"username\"")
	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been created.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to build. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)
	cmd.Flags().StringArrayVarP(&env, "env", "e", []string{}, "Set environment variables for the build")
	cmd.Flags().StringArrayVarP(&metaData, "metadata", "M", []string{}, "Set metadata for the build (KEY=VALUE)")
	cmd.Flags().BoolVarP(&ignoreBranchFilters, "ignore-branch-filters", "i", false, "Ignore branch filters for the pipeline")
	cmd.Flags().BoolVarP(&confirmed, "yes", "y", false, "Skip the confirmation prompt. Useful if being used in automation/CI")
	cmd.Flags().StringVarP(&envFile, "env-file", "f", "", "Set the environment variables for the build via an environment file")
	cmd.Flags().StringVarP(&envFile, "envFile", "", "", "Set the environment variables for the build via an environment file")
	_ = cmd.Flags().MarkDeprecated("envFile", "use --env-file instead")
	cmd.Flags().SetNormalizeFunc(normaliseFlags)
	cmd.Flags().SortFlags = false
	return &cmd
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

func newBuild(ctx context.Context, org string, pipeline string, f *factory.Factory, message string, commit string, branch string, web bool, env map[string]string, metaData map[string]string, ignoreBranchFilters bool, author string) error {
	var actionErr error
	var build buildkite.Build
	spinErr := bk_io.SpinWhile(fmt.Sprintf("Starting new build for %s", pipeline), func() {
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

func normaliseFlags(pf *pflag.FlagSet, name string) pflag.NormalizedName {
	switch name {
	case "envFile":
		name = "env-file"
	}
	return pflag.NormalizedName(name)
}
