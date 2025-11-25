package pipeline

import (
	"context"
	"fmt"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
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
			var clusterID string

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

			clusterMap := getClusters(cmd.Context(), f)
			clusterNames := make([]string, 0, len(clusterMap))

			for name := range clusterMap {
				clusterNames = append(clusterNames, name)
			}
			if len(clusterNames) > 0 {
				prompt := &survey.Select{
					Message: "Choose a cluster:",
					Options: clusterNames,
				}
				var selectedClusterName string
				err := survey.AskOne(prompt, &selectedClusterName, survey.WithValidator(survey.Required))
				if err != nil {
					return err
				}
				clusterID = clusterMap[selectedClusterName]
			} else {
				// Use user provided answer as the cluster ID
				err := survey.AskOne(&survey.Input{Message: "No clusters found. Optionally provide a Cluster ID (press Enter to skip):"}, &clusterID)
				if err != nil {
					return err
				}
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

			return createPipeline(cmd.Context(), f.RestAPIClient, f.Config.OrganizationSlug(), answers.Pipeline, answers.Description, clusterID, repoURL)
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

func getClusters(ctx context.Context, f *factory.Factory) map[string]string {
	clusters, _, err := f.RestAPIClient.Clusters.List(ctx, f.Config.OrganizationSlug(), nil)
	if err != nil {
		return map[string]string{}
	}

	if len(clusters) < 1 {
		return map[string]string{}
	}

	clusterMap := make(map[string]string, len(clusters))
	for _, cluster := range clusters {
		clusterMap[cluster.Name] = cluster.ID
	}
	return clusterMap
}

func createPipeline(ctx context.Context, client *buildkite.Client, org, pipelineName, description, clusterID, repoURL string) error {
	var err error
	var output string

	spinErr := bk_io.SpinWhile(fmt.Sprintf("Creating new pipeline %s for %s", pipelineName, org), func() {
		createPipeline := buildkite.CreatePipeline{
			Name:          pipelineName,
			Repository:    repoURL,
			Description:   description,
			ClusterID:     clusterID,
			Configuration: "steps:\n  - label: \":pipeline:\"\n    command: buildkite-agent pipeline upload",
		}

		var pipeline buildkite.Pipeline
		pipeline, _, err = client.Pipelines.Create(ctx, org, createPipeline)

		output = lipgloss.JoinVertical(lipgloss.Top, lipgloss.NewStyle().Padding(1, 1).Render(fmt.Sprintf("Pipeline created: %s", pipeline.WebURL)))
	})

	fmt.Println(output)

	if spinErr != nil {
		return spinErr
	}

	return err
}
