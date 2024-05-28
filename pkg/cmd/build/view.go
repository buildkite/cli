package build

import (
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/annotation"
	"github.com/buildkite/cli/v3/internal/artifact"
	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/io"
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

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "view [number [pipeline]] [flags]",
		Short:                 "View build information.",
		Long: heredoc.Doc(`
			View a build's information.

			It accepts a build number and a pipeline slug as an argument.
			If the build argument is be omitted, the most recent build on the current branch will be resolved.
			The pipeline can be a {pipeline_slug} or in the format {org_slug}/{pipeline_slug}.
			If the pipeline argument is omitted, it will be resolved using the current directory.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			var buildArtifacts = make([]buildkite.Artifact, 0)
			var buildAnnotations = make([]buildkite.Annotation, 0)

			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromPositionalArgument(args, 1, f.Config),
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

			l := io.NewPendingCommand(func() tea.Msg {
				var buildUrl string
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

				if web {
					buildUrl = fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d", bld.Organization, bld.Pipeline, *b.Number)
					fmt.Printf("Opening %s in your browser\n\n", buildUrl)
					time.Sleep(1 * time.Second)
					err = browser.OpenURL(buildUrl)
					if err != nil {
						fmt.Println("Error opening browser: ", err)
					}
				}

				// Obtain build summary and return
				summary := build.BuildSummary(b)
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

	return &cmd
}
