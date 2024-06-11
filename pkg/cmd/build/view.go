package build

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/annotation"
	"github.com/buildkite/cli/v3/internal/artifact"
	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/job"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
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
			buildArtifacts := make([]buildkite.Artifact, 0)
			buildAnnotations := make([]buildkite.Annotation, 0)

			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
				buildResolver.ResolveBuildFromCurrentBranch(f.GitRepository, pipelineRes.Resolve, f),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if bld == nil {
				return fmt.Errorf("could not resolve a build")
			}

			if web {
				buildUrl := fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d", bld.Organization, bld.Pipeline, bld.BuildNumber)
				fmt.Printf("Opening %s in your browser\n\n", buildUrl)
				return browser.OpenURL(buildUrl)
			}

			l := io.NewPendingCommand(func() tea.Msg {

				b, _, err := f.RestAPIClient.Builds.Get(bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), &buildkite.BuildsListOptions{})
				if err != nil {
					return err
				}

				buildArtifacts, _, err = f.RestAPIClient.Artifacts.ListByBuild(bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), &buildkite.ArtifactListOptions{})
				if err != nil {
					return err
				}

				buildAnnotations, _, err = f.RestAPIClient.Annotations.ListByBuild(bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), &buildkite.AnnotationListOptions{})
				if err != nil {
					return err
				}

				// Obtain build summary and return
				summary := build.BuildSummary(b)
				if len(b.Jobs) > 0 {
					summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Render("\nJobs")
					for _, j := range b.Jobs {
						summary += job.JobSummary(j)
					}
				}
				if len(buildArtifacts) > 0 {
					summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Render("\nArtifacts")
					for _, a := range buildArtifacts {
						summary += artifact.ArtifactSummary(&a)
					}
				}
				if len(buildAnnotations) > 0 {
					summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Render("\nAnnotations")
					for _, a := range buildAnnotations {
						summary += annotation.AnnotationSummary(&a)
					}
				}
				return io.PendingOutput(summary)
			}, "Loading build information")

			p := tea.NewProgram(l)
			_, err = p.Run()

			return err
		},
	}

	cmd.Flags().BoolVarP(&web, "web", "w", false, "Open the build in a web browser.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to view. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)

	return &cmd
}
