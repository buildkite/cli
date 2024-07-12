package pipeline

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func NewCmdPipelineCreate(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "create",
		Short:                 "Creates a new pipeline",
		Args:                  cobra.NoArgs,
		Long: heredoc.Doc(`
			Creates a new pipeline in the current org and outputs the URL to the pipeline.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			var repoURL string

			qs := []*survey.Question{
				{
					Name:     "pipeline",
					Prompt:   &survey.Input{Message: "Name:"},
					Validate: survey.Required,
				},
				{
					Name:     "description",
					Prompt:   &survey.Input{Message: "Description:"},
					Validate: survey.Required,
				},
			}
			answers := struct{ Pipeline, Description string }{}
			err := survey.Ask(qs, &answers)
			if err != nil {
				return err
			}

			repoURLS := getRepoURLS(f)
			if len(repoURLS) > 0 {
				prompt := &survey.Select{
					Message: "Choose a repository:",
					Options: repoURLS,
				}
				err := survey.AskOne(prompt, &repoURL, survey.WithValidator(survey.Required))
				if err != nil {
					return err
				}
			} else {
				err := survey.AskOne(&survey.Input{Message: "Repository URL:"}, &repoURL, survey.WithValidator(survey.Required))
				if err != nil {
					return err
				}
			}

			return createPipeline(f.RestAPIClient, f.Config.OrganizationSlug(), answers.Pipeline, answers.Description, repoURL)
		},
	}

	return &cmd
}

func getRepoURLS(f *factory.Factory) []string {
	if f.GitRepository == nil {
		return []string{}
	}

	c, err := f.GitRepository.Config()
	if err != nil {
		return []string{}
	}

	if _, ok := c.Remotes["origin"]; !ok {
		return []string{}
	}
	return c.Remotes["origin"].URLs
}

func createPipeline(client *buildkite.Client, org, pipelineName, description, repoURL string) error {
	var err error
	var output string

	spinErr := spinner.New().
		Title(fmt.Sprintf("Creating new pipeline %s for %s", pipelineName, org)).
		Action(func() {
			createPipeline := buildkite.CreatePipeline{
				Name:          *buildkite.String(pipelineName),
				Repository:    *buildkite.String(repoURL),
				Description:   *buildkite.String(description),
				Configuration: *buildkite.String("steps:\n  - label: \":pipeline:\"\n    command: buildkite-agent pipeline upload"),
			}

			var pipeline *buildkite.Pipeline
			pipeline, _, err = client.Pipelines.Create(org, &createPipeline)

			output = lipgloss.JoinVertical(lipgloss.Top, lipgloss.NewStyle().Padding(1, 1).Render(fmt.Sprintf("Pipeline created: %s", *pipeline.WebURL)))
		}).
		Run()

	fmt.Println(output)

	if spinErr != nil {
		return spinErr
	}

	return err
}
