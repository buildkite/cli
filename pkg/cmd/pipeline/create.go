package pipeline

import (
	"context"
	"fmt"
	"sort"

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

			clusterMap, err := getClusters(cmd.Context(), f)
			if err != nil {
				return err
			}

			clusterNames := make([]string, 0, len(clusterMap))
			for name := range clusterMap {
				clusterNames = append(clusterNames, name)
			}
			sort.Strings(clusterNames)
			clusterNames = append([]string{"Skip (No Cluster)"}, clusterNames...)

			if len(clusterMap) > 0 {

				prompt := &survey.Select{
					Message: "Choose a cluster:",
					Options: clusterNames,
				}

				var selectedClusterName string
				err := survey.AskOne(prompt, &selectedClusterName, survey.WithValidator(survey.Required))
				if err != nil {
					return err
				}

				if selectedClusterName != "Skip (No Cluster)" {
					clusterID = clusterMap[selectedClusterName] // will be "" if "Skip (no cluster)" was selected
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

			return createPipeline(cmd.Context(), f, answers.Pipeline, answers.Description, clusterID, repoURL)
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

func getClusters(ctx context.Context, f *factory.Factory) (map[string]string, error) {

	clusterMap := make(map[string]string) // map of cluster name to cluster ID
	page := 1
	per_page := 30

	for more_clusters := true; more_clusters; {
		opts := buildkite.ClustersListOptions{
			ListOptions: buildkite.ListOptions{
				Page:    page,
				PerPage: per_page,
			},
		}
		clusters, resp, err := f.RestAPIClient.Clusters.List(ctx, f.Config.OrganizationSlug(), &opts)
		if err != nil {
			return map[string]string{}, err
		}

		if len(clusters) < 1 {
			return map[string]string{}, nil
		}

		for _, c := range clusters {
			clusterMap[c.Name] = c.ID
		}

		if resp.NextPage == 0 {
			more_clusters = false
		} else {
			page = resp.NextPage
		}
	}
	return clusterMap, nil
}

func createPipeline(ctx context.Context, f *factory.Factory, pipelineName, description, clusterID, repoURL string) error {
	var err error
	var output string

	spinErr := bk_io.SpinWhile(f, fmt.Sprintf("Creating new pipeline %s for %s", pipelineName, f.Config.OrganizationSlug()), func() {
		createPipeline := buildkite.CreatePipeline{
			Name:          pipelineName,
			Repository:    repoURL,
			Description:   description,
			ClusterID:     clusterID,
			Configuration: "steps:\n  - label: \":pipeline:\"\n    command: buildkite-agent pipeline upload",
		}

		var pipeline buildkite.Pipeline
		pipeline, _, err = f.RestAPIClient.Pipelines.Create(ctx, f.Config.OrganizationSlug(), createPipeline)

		output = lipgloss.JoinVertical(lipgloss.Top, lipgloss.NewStyle().Padding(1, 1).Render(fmt.Sprintf("Pipeline created: %s", pipeline.WebURL)))
	})

	fmt.Println(output)

	if spinErr != nil {
		return spinErr
	}

	return err
}
