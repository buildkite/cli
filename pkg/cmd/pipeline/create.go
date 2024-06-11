package pipeline

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	tea "github.com/charmbracelet/bubbletea"
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

			fmt.Printf("Creating pipeline %s with description %s, repo %s\n", answers.Pipeline, answers.Description, repoURL)

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
	l := io.NewPendingCommand(func() tea.Msg {

		createPipeline := buildkite.CreatePipeline{
			Name:          *buildkite.String(pipelineName),
			Repository:    *buildkite.String(repoURL),
			Description:   *buildkite.String(description),
			Configuration: *buildkite.String("steps:\n  - label: \":pipeline:\"\n    command: buildkite-agent pipeline upload"),
		}

		pipeline, resp, err := client.Pipelines.Create(org, &createPipeline)

		if err != nil {
			//return renderOutput(fmt.Sprintf("Unable to create pipeline.: %s", err.Error()))
			return io.PendingOutput(lipgloss.JoinVertical(lipgloss.Top,
				lipgloss.NewStyle().Padding(1, 1).Render(fmt.Sprintf("Unable to create pipeline.: %s", err.Error()))))
		}

		if resp == nil {
			return io.PendingOutput(lipgloss.JoinVertical(lipgloss.Top,
				lipgloss.NewStyle().Padding(1, 1).Render("Unable to create pipeline.")))
		}

		if resp.StatusCode != 201 {
			return io.PendingOutput(lipgloss.JoinVertical(lipgloss.Top,
				lipgloss.NewStyle().Padding(1, 1).Render(fmt.Sprintf("Unable to create pipeline. %d: %s", resp.StatusCode, resp.Status))))
		}

		return io.PendingOutput(lipgloss.JoinVertical(lipgloss.Top,
			lipgloss.NewStyle().Padding(1, 1).Render(fmt.Sprintf("Pipeline created: %s", *pipeline.WebURL))))

	}, fmt.Sprintf("Creating new pipeline %s for %s", pipelineName, org))
	p := tea.NewProgram(l)
	_, err := p.Run()
	return err
}
