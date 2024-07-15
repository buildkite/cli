package build

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/annotation"
	"github.com/buildkite/cli/v3/internal/artifact"
	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/job"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func NewCmdBuildView(f *factory.Factory) *cobra.Command {
	var web bool
	var pipeline string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view [number] [flags]",
		Short:                 "View build information.",
		Args:                  cobra.MaximumNArgs(1),
		Long: heredoc.Doc(`
			View a build's information.

			You can pass an optional build number to view. If omitted, the most recent build on the current branch will be resolved.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
			).WithResolverWhen(
				f.GitRepository != nil,
				buildResolver.ResolveBuildFromCurrentBranch(f.GitRepository, pipelineRes.Resolve, f),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if bld == nil || err != nil {
				fmt.Printf("No build found.\n")
				return nil
			}

			var b *buildkite.Build
			var buildArtifacts []buildkite.Artifact
			var buildAnnotations []buildkite.Annotation

			group, _ := errgroup.WithContext(cmd.Context())
			spinErr := spinner.New().
				Title("Requesting build information").
				Action(func() {
					group.Go(func() error {
						var err error
						b, _, err = f.RestAPIClient.Builds.Get(bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), &buildkite.BuildsListOptions{})
						return err
					})

					group.Go(func() error {
						var err error
						buildArtifacts, _, err = f.RestAPIClient.Artifacts.ListByBuild(bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), &buildkite.ArtifactListOptions{})
						return err
					})

					group.Go(func() error {
						var err error
						buildAnnotations, _, err = f.RestAPIClient.Annotations.ListByBuild(bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), &buildkite.AnnotationListOptions{})
						return err
					})

					err = group.Wait()
				}).
				Run()
			if spinErr != nil {
				return spinErr
			}

			if b == nil {
				fmt.Printf("Could not find build #%d for pipeline %s\n", bld.BuildNumber, bld.Pipeline)
				return nil
			}

			if err != nil {
				return err
			}

			if web {
				buildUrl := fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d", bld.Organization, bld.Pipeline, bld.BuildNumber)
				fmt.Printf("Opening %s in your browser\n", buildUrl)
				return browser.OpenURL(buildUrl)
			}

			summary := build.BuildSummary(b)
			if len(b.Jobs) > 0 {
				summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("\nJobs")
				for _, j := range b.Jobs {
					bkJob := *j
					if *bkJob.Type == "script" {
						summary += job.JobSummary(job.Job(bkJob))
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

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to view. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)

	return &cmd
}
