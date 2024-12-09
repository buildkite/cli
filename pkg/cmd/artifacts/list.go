package artifacts

import (
	"fmt"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/artifact"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func NewCmdArtifactsList(f *factory.Factory) *cobra.Command {
	var pipeline, job string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "list [number] [flags]",
		Short:                 "List artifacts for a build or a job in a build.",
		Args:                  cobra.MaximumNArgs(1),
		Long: heredoc.Doc(`
			List artifacts for a build or a job in a build.

			You can pass an optional build number. If omitted, the most recent build on the current branch will be resolved.
	`),
		Example: heredoc.Doc(`
			# by default, artifacts of the most recent build for the current branch is shown
			$ bk artifacts list 
			# to list artifacts of a specific build
			$ bk artifacts list 429 
			# to list artifacts of a specific job in a build
			$ bk artifacts list 429 --job 0193903e-ecd9-4c51-9156-0738da987e87  
			# if not inside a repository or to use a specific pipeline, pass -p
			$ bk artifacts list -p monolith 
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error

			buildRes := resolveFrom(pipeline, f, args)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if bld == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "No build found.\n")
				return nil
			}

			var buildArtifacts []buildkite.Artifact

			var wg sync.WaitGroup
			var mu sync.Mutex

			spinErr := spinner.New().
				Title("Loading artifacts information").
				Action(func() {
					wg.Add(1)

					go func() {
						defer wg.Done()
						var apiErr error

						if job != "" { // list artifacts for a specific job
							buildArtifacts, _, apiErr = f.RestAPIClient.Artifacts.ListByJob(cmd.Context(), bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), job, nil)
						} else {
							buildArtifacts, _, apiErr = f.RestAPIClient.Artifacts.ListByBuild(cmd.Context(), bld.Organization, bld.Pipeline, fmt.Sprint(bld.BuildNumber), nil)
						}
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

			var summary string
			if len(buildArtifacts) > 0 {
				summary += lipgloss.NewStyle().Bold(true).Padding(0, 1).Underline(true).Render("Artifacts")
				for _, a := range buildArtifacts {
					summary += artifact.ArtifactSummary(&a)
				}
			} else {
				summary += lipgloss.NewStyle().Padding(0, 1).Render("No artifacts found.")
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", summary)

			return err
		},
	}

	cmd.Flags().StringVarP(&job, "job", "j", "", "List artifacts for a specific job on the given build.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to view. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)

	cmd.Flags().SortFlags = false

	return &cmd
}
