package build

import (
	"fmt"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/build/models"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/build/view"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

func NewCmdBuildView(f *factory.Factory) *cobra.Command {
	var opts view.ViewOptions
	var mine bool
	var branch, user string

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view [number] [flags]",
		Short:                 "View build information",
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
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := output.GetFormat(cmd.Flags())
			if err != nil {
				return err
			}

			// Get pipeline from persistent flag
			opts.Pipeline, _ = cmd.Flags().GetString("pipeline")

			// Resolve pipeline first
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(opts.Pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne, f.GitRepository != nil)),
			)

			// Resolve build options
			optionsResolver := options.AggregateResolver{
				options.ResolveBranchFromFlag(branch),
				options.ResolveBranchFromRepository(f.GitRepository),
			}.WithResolverWhen(
				user != "",
				options.ResolveUserFromFlag(user),
			).WithResolverWhen(
				mine || user == "",
				options.ResolveCurrentUser(cmd.Context(), f),
			)

			// Resolve build
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

			opts.Organization = bld.Organization
			opts.Pipeline = bld.Pipeline
			opts.BuildNumber = bld.BuildNumber

			if err := opts.Validate(); err != nil {
				return err
			}

			if opts.Web {
				buildURL := fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d",
					opts.Organization, opts.Pipeline, opts.BuildNumber)
				fmt.Printf("Opening %s in your browser\n", buildURL)
				return browser.OpenURL(buildURL)
			}

			var build buildkite.Build
			var artifacts []buildkite.Artifact
			var annotations []buildkite.Annotation
			var wg sync.WaitGroup
			var mu sync.Mutex

			spinErr := bk_io.SpinWhile("Loading build information", func() {
				wg.Add(3)
				go func() {
					defer wg.Done()
					var apiErr error
					build, _, apiErr = f.RestAPIClient.Builds.Get(
						cmd.Context(),
						opts.Organization,
						opts.Pipeline,
						fmt.Sprint(opts.BuildNumber),
						nil,
					)
					if apiErr != nil {
						mu.Lock()
						err = apiErr
						mu.Unlock()
					}
				}()

				go func() {
					defer wg.Done()
					var apiErr error
					artifacts, _, apiErr = f.RestAPIClient.Artifacts.ListByBuild(
						cmd.Context(),
						opts.Organization,
						opts.Pipeline,
						fmt.Sprint(opts.BuildNumber),
						nil,
					)
					if apiErr != nil {
						mu.Lock()
						err = apiErr
						mu.Unlock()
					}
				}()

				go func() {
					defer wg.Done()
					var apiErr error
					annotations, _, apiErr = f.RestAPIClient.Annotations.ListByBuild(
						cmd.Context(),
						opts.Organization,
						opts.Pipeline,
						fmt.Sprint(opts.BuildNumber),
						nil,
					)
					if apiErr != nil {
						mu.Lock()
						err = apiErr
						mu.Unlock()
					}
				}()

				wg.Wait()
			})
			if spinErr != nil {
				return spinErr
			}
			if err != nil {
				return err
			}

			// Create structured view for output using models package
			buildView := models.NewBuildView(&build, artifacts, annotations)

			return output.Write(cmd.OutOrStdout(), buildView, format)
		},
	}

	cmd.Flags().BoolVar(&mine, "mine", false, "Filter builds to only my user.")
	cmd.Flags().BoolVar(&opts.Web, "web", false, "Open the build in a web browser.")
	cmd.Flags().StringVar(&branch, "branch", "", "Filter builds to this branch.")
	cmd.Flags().StringVar(&user, "user", "", "Filter builds to this user. You can use name or email.")

	// can only supply --user or --mine
	cmd.MarkFlagsMutuallyExclusive("user", "mine")

	output.AddFlags(cmd.Flags())
	cmd.Flags().SortFlags = false
	return cmd
}
