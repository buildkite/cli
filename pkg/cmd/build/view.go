package build

import (
	"fmt"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/annotation"
	"github.com/buildkite/cli/v3/internal/artifact"
	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/job"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/scopes"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func NewCmdBuildView(f *factory.Factory) *cobra.Command {
	var web, mine bool
	var branch, pipeline, user string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view [number] [flags]",
		Short:                 "View build information.",
		Args:                  cobra.MaximumNArgs(1),
		Long: heredoc.Doc(`
			View a build's information.

			You can pass an optional build number to view. If omitted, the most recent build on the current branch will be resolved.
		`),
		Example: heredoc.Doc(`
			# by default, the most recent build for the current branch is shown
			$ bk build view
			# if not inside a repository or to use a specific pipeline, pass -p
			$ bk build view -p monolith
			# to view a specific build
			$ bk build view 429
			# add -w to any command to open the build in your web browser instead
			$ bk build view -w 429
			# to view the most recent build on feature-x branch
			$ bk build view -b feature-y
			# you can filter by a user name or id
			$ bk build view -u "alice"
			# a shortcut to view your builds is --mine
			$ bk build view --mine
			# you can combine most of these flags
			# to view most recent build by greg on the deploy-pipeline
			$ bk build view -p deploy-pipeline -u "greg"
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
			var err error
			// we find the pipeline based on the following rules:
			// 1. an explicit flag is passed
			// 2. a configured pipeline for this directory
			// 3. find pipelines matching the current repository from the API
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			// we resolve a build based on the following rules:
			// 1. an optional argument
			// 2. resolve from API using some context
			//    a. filter by branch if --branch or use current repo
			//    b. filter by user if --user or --mine given
			optionsResolver := options.AggregateResolver{
				options.ResolveBranchFromFlag(branch),
				options.ResolveBranchFromRepository(f.GitRepository),
			}.WithResolverWhen(
				user != "",
				options.ResolveUserFromFlag(user),
			).WithResolverWhen(
				mine || user == "",
				options.ResolveCurrentUser(f),
			)
			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
				buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if bld == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "No build found.\n")
				return nil
			}

			if web {
				buildUrl := fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d", bld.Organization, bld.Pipeline, bld.BuildNumber)
				fmt.Printf("Opening %s in your browser\n", buildUrl)
				return browser.OpenURL(buildUrl)
			}

			var b buildkite.Build
			var buildArtifacts []buildkite.Artifact
			var buildAnnotations []buildkite.Annotation

			var wg sync.WaitGroup
			var mu sync.Mutex

			spinErr := spinner.New().
				Title("Loading build information").
				Action(func() {
					wg.Add(3)
					go func() {
						defer wg.Done()
						var apiErr error
						b, _, apiErr = f.RestAPIClient.Builds.Get(cmd.Context(), bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), nil)
						if apiErr != nil {
							mu.Lock()
							err = apiErr
							mu.Unlock()
						}
					}()

					go func() {
						defer wg.Done()
						var apiErr error
						buildArtifacts, _, apiErr = f.RestAPIClient.Artifacts.ListByBuild(cmd.Context(), bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), nil)
						if apiErr != nil {
							mu.Lock()
							err = apiErr
							mu.Unlock()
						}
					}()

					go func() {
						defer wg.Done()
						var apiErr error
						buildAnnotations, _, apiErr = f.RestAPIClient.Annotations.ListByBuild(cmd.Context(), bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), nil)
						if apiErr != nil {
							mu.Lock()
							err = apiErr
							mu.Unlock()
						}
					}()

					wg.Wait()
				}).
				Run()
			if spinErr != nil {
				return spinErr
			}
			if err != nil {
				return err
			}

			summary := build.BuildSummary(b)
			if len(b.Jobs) > 0 {
				summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("\nJobs")
				for _, j := range b.Jobs {
					if j.Type == "script" {
						summary += job.JobSummary(job.Job(j))
					}
				}
			}
			if len(buildArtifacts) > 0 {
				summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("\n\nArtifacts")
				for _, a := range buildArtifacts {
					summary += artifact.ArtifactSummary(&a)
				}
			}
			if len(buildAnnotations) > 0 {
				for _, a := range buildAnnotations {
					annotationCount := 0
					if len(annotation.AnnotationSummary(&a)) < 230 {
						continue
					}
					annotationCount += 1
					if annotationCount > 0 {
						summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("\n\nAnnotations")
						summary += annotation.AnnotationSummary(&a)
					}
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", summary)

			return err
		},
	}

	cmd.Annotations = map[string]string{
		"requiredScopes": string(scopes.ReadBuilds),
	}

	cmd.Flags().BoolVarP(&mine, "mine", "m", false, "Filter builds to only my user.")
	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser.")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Filter builds to this branch.")
	cmd.Flags().StringVarP(&user, "user", "u", "", "Filter builds to this user. You can use name or email.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to view. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)
	// can only supply --user or --mine
	cmd.MarkFlagsMutuallyExclusive("mine", "user")
	cmd.Flags().SortFlags = false

	return &cmd
}
