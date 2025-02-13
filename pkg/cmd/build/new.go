package build

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/internal/validation/scopes"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/huh/spinner"
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
	envMap := make(map[string]string)
	var envFile string

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

		`),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Get the command's required and optional scopes
			cmdScopes := scopes.GetCommandScopes(cmd)

			// Get the token scopes from the factory
			tokenScopes := f.Config.GetTokenScopes()
			if len(tokenScopes) == 0 {
				return fmt.Errorf("no scopes found in token. Please ensure you're using a token with appropriate scopes")
			}

			// Validate the scopes
			if err := scopes.ValidateScopes(cmdScopes, tokenScopes); err != nil {
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvers := resolver.NewAggregateResolver(
				resolver.ResolveFromFlag(pipeline, f.Config),
				resolver.ResolveFromConfig(f.Config, resolver.PickOne),
				resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOne)),
			)

			pipeline, err := resolvers.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if pipeline == nil {
				return fmt.Errorf("could not resolve a pipeline")
			}

			err = io.Confirm(&confirmed, fmt.Sprintf("Create new build on %s?", pipeline.Name))
			if err != nil {
				return err
			}

			if confirmed {
				for _, e := range env {
					key, value, _ := strings.Cut(e, "=")
					envMap[key] = value
				}
				if envFile != "" {
					file, err := os.Open(envFile)
					if err != nil {
						return err
					}
					defer file.Close()
					content := bufio.NewScanner(file)
					for content.Scan() {
						key, value, _ := strings.Cut(content.Text(), "=")
						envMap[key] = value
					}
				}
				return newBuild(cmd.Context(), pipeline.Org, pipeline.Name, f, message, commit, branch, web, envMap, ignoreBranchFilters)
			} else {
				return nil
			}
		},
	}

	cmd.Annotations = map[string]string{
		"requiredScopes": string(scopes.WriteBuilds),
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Description of the build. If left blank, the commit message will be used once the build starts.")
	cmd.Flags().StringVarP(&commit, "commit", "c", "HEAD", "The commit to build.")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "The branch to build. Defaults to the default branch of the pipeline.")
	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser after it has been created.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to build. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)
	cmd.Flags().StringArrayVarP(&env, "env", "e", []string{}, "Set environment variables for the build")
	cmd.Flags().BoolVarP(&ignoreBranchFilters, "ignore-branch-filters", "i", false, "Ignore branch filters for the pipeline")
	cmd.Flags().BoolVarP(&confirmed, "yes", "y", false, "Skip the confirmation prompt. Useful if being used in automation/CI")
	cmd.Flags().StringVarP(&envFile, "env-file", "f", "", "Set the environment variables for the build via an environment file")
	cmd.Flags().StringVarP(&envFile, "envFile", "", "", "Set the environment variables for the build via an environment file")
	_ = cmd.Flags().MarkDeprecated("envFile", "use --env-file instead")
	cmd.Flags().SetNormalizeFunc(normaliseFlags)
	cmd.Flags().SortFlags = false
	return &cmd
}

func newBuild(ctx context.Context, org string, pipeline string, f *factory.Factory, message string, commit string, branch string, web bool, env map[string]string, ignoreBranchFilters bool) error {
	var actionErr error
	var build buildkite.Build
	spinErr := spinner.New().
		Title(fmt.Sprintf("Starting new build for %s", pipeline)).
		Action(func() {
			branch = strings.TrimSpace(branch)
			if len(branch) == 0 {
				p, _, err := f.RestAPIClient.Pipelines.Get(ctx, org, pipeline)
				if err != nil {
					actionErr = fmt.Errorf("Error getting pipeline: %w", err)
					return
				}

				// Check if the pipeline has a default branch set
				if p.DefaultBranch == "" {
					actionErr = fmt.Errorf("No default branch set for pipeline %s. Please specify a branch using --branch (-b)", pipeline)
					return
				}
				branch = p.DefaultBranch
			}

			newBuild := buildkite.CreateBuild{
				Message:                     message,
				Commit:                      commit,
				Branch:                      branch,
				Env:                         env,
				IgnorePipelineBranchFilters: ignoreBranchFilters,
			}

			var err error
			build, _, err = f.RestAPIClient.Builds.Create(ctx, org, pipeline, newBuild)
			if err != nil {
				actionErr = fmt.Errorf("Error creating build: %w", err)
				return
			}
		}).
		Run()
	if spinErr != nil {
		return spinErr
	}

	if actionErr != nil {
		return actionErr
	}

	if build.WebURL == "" {
		return fmt.Errorf("no build was created")
	}

	fmt.Printf("%s\n", renderResult(fmt.Sprintf("Build created: %s", build.WebURL)))

	return util.OpenInWebBrowser(web, build.WebURL)
}

func normaliseFlags(pf *pflag.FlagSet, name string) pflag.NormalizedName {
	switch name {
	case "envFile":
		name = "env-file"
	}
	return pflag.NormalizedName(name)
}
