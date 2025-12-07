package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type pipelineCreateOptions struct {
	DryRun bool
}

func NewCmdPipelineCreate(f *factory.Factory) *cobra.Command {
	var options pipelineCreateOptions

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "create [flags]",
		Short:                 "Creates a new pipeline",
		Args:                  cobra.NoArgs,
		Long: heredoc.Doc(`
			Creates a new pipeline in the current org and outputs the URL to the pipeline. 

			You can specify a --dry-run flag to see the pipeline that would be created without actually creating it. This outputs a JSON representation of the pipeline to be created.
		`),
		Example: heredoc.Doc(`
			# Create the default pipeline file
			$ bk pipeline create

			# View the pipeline that would be created without actually creating it
			$ bk pipeline create --dry-run
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

			if options.DryRun {
				return createPipelineDryRun(cmd.Context(), f, answers.Pipeline, answers.Description, clusterID, repoURL)
			}

			return createPipeline(cmd.Context(), f, answers.Pipeline, answers.Description, clusterID, repoURL)
		},
	}

	cmd.Flags().BoolVar(&options.DryRun, "dry-run", false, "Outputs the pipeline that would be created without actually creating it")
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

// PipelineDryRun is a custom struct for dry-run output that includes all fields
// without omitempty tags, ensuring empty strings and zero values are included in JSON output
type PipelineDryRun struct {
	ID                              string               `json:"id"`
	GraphQLID                       string               `json:"graphql_id"`
	URL                             string               `json:"url"`
	WebURL                          string               `json:"web_url"`
	Name                            string               `json:"name"`
	Description                     string               `json:"description"`
	Slug                            string               `json:"slug"`
	Repository                      string               `json:"repository"`
	ClusterID                       string               `json:"cluster_id"`
	ClusterURL                      string               `json:"cluster_url"`
	BranchConfiguration             string               `json:"branch_configuration"`
	DefaultBranch                   string               `json:"default_branch"`
	SkipQueuedBranchBuilds          bool                 `json:"skip_queued_branch_builds"`
	SkipQueuedBranchBuildsFilter    string               `json:"skip_queued_branch_builds_filter"`
	CancelRunningBranchBuilds       bool                 `json:"cancel_running_branch_builds"`
	CancelRunningBranchBuildsFilter string               `json:"cancel_running_branch_builds_filter"`
	BuildsURL                       string               `json:"builds_url"`
	BadgeURL                        string               `json:"badge_url"`
	CreatedAt                       *buildkite.Timestamp `json:"created_at"`
	Env                             map[string]any       `json:"env"`
	ScheduledBuildsCount            int                  `json:"scheduled_builds_count"`
	RunningBuildsCount              int                  `json:"running_builds_count"`
	ScheduledJobsCount              int                  `json:"scheduled_jobs_count"`
	RunningJobsCount                int                  `json:"running_jobs_count"`
	WaitingJobsCount                int                  `json:"waiting_jobs_count"`
	Visibility                      string               `json:"visibility"`
	Tags                            []string             `json:"tags"`
	Configuration                   string               `json:"configuration"`
	Steps                           []buildkite.Step     `json:"steps"`
	Provider                        buildkite.Provider   `json:"provider"`
	PipelineTemplateUUID            string               `json:"pipeline_template_uuid"`
	AllowRebuilds                   bool                 `json:"allow_rebuilds"`
	Emoji                           *string              `json:"emoji"`
	Color                           *string              `json:"color"`
	CreatedBy                       *buildkite.User      `json:"created_by"`
}

func initialisePipelineDryRun() PipelineDryRun {
	return PipelineDryRun{
		Env:   nil,
		Tags:  nil,
		Steps: []buildkite.Step{},
		Provider: buildkite.Provider{
			Settings: &buildkite.GitHubSettings{},
		},
		AllowRebuilds: true,
	}
}

func createPipelineDryRun(ctx context.Context, f *factory.Factory, pipelineName, description, clusterID, repoURL string) error {

	pipelineSlug := generateSlug(pipelineName)

	pipelineSlug, err := getAvailablePipelineSlug(ctx, f, pipelineSlug, pipelineName)
	if err != nil {
		return err
	}

	orgSlug := f.Config.OrganizationSlug()
	pipeline := initialisePipelineDryRun()

	// Set specific fields with actual values
	pipeline.ID = "00000000-0000-0000-0000-000000000000"
	pipeline.GraphQLID = "UGlwZWxpbmUtLS0wMDAwMDAwMC0wMDAwLTAwMDAtMDAwMC0wMDAwMDAwMDAwMDA="
	pipeline.URL = fmt.Sprintf("https://api.buildkite.com/v2/organizations/%s/pipelines/%s", orgSlug, pipelineSlug)
	pipeline.WebURL = fmt.Sprintf("https://buildkite.com/%s/%s", orgSlug, pipelineSlug)
	pipeline.Name = pipelineName
	pipeline.Description = description
	pipeline.Slug = pipelineSlug
	pipeline.Repository = repoURL
	pipeline.ClusterID = clusterID
	pipeline.ClusterURL = getClusterUrl(orgSlug, clusterID)
	pipeline.DefaultBranch = "main"
	pipeline.BuildsURL = fmt.Sprintf("https://api.buildkite.com/v2/organizations/%s/pipelines/%s/builds", orgSlug, pipelineSlug)
	pipeline.BadgeURL = fmt.Sprintf("https://badge.buildkite.com/%s.svg", "00000000000000000000000000000000000000000000000000")
	pipeline.CreatedAt = buildkite.NewTimestamp(time.Now())
	pipeline.Visibility = "private"
	pipeline.Configuration = "steps:\n  - label: \":pipeline:\"\n    command: buildkite-agent pipeline upload"
	pipeline.Steps = []buildkite.Step{
		{
			Type:    ":pipeline:",
			Name:    ":pipeline:",
			Command: "buildkite-agent pipeline upload",
		},
	}
	pipeline.Provider = buildkite.Provider{
		ID:         "github",
		WebhookURL: "https://webhook.buildkite.com/deliver/00000000000000000000000000000000000000000000000000",
		Settings: &buildkite.GitHubSettings{
			TriggerMode:         "code",
			BuildPullRequests:   true,
			BuildBranches:       true,
			PublishCommitStatus: true,
			Repository:          extractRepoPath(repoURL),
		},
	}

	pipeline.CreatedBy = getCreatedByDetails(ctx, f)

	jsonOutput, err := json.MarshalIndent(pipeline, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dry run response: %w", err)
	}

	fmt.Println(string(jsonOutput))
	return nil
}

// generateSlug creates a URL-friendly slug from the pipeline name
func generateSlug(name string) string {
	// Trim leading and trailing spaces
	name = strings.TrimSpace(name)

	var slug strings.Builder
	lastWasSeparator := false

	for _, c := range strings.ToLower(name) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			slug.WriteRune(c)
			lastWasSeparator = false
		} else if c == ' ' || c == '-' || c == '_' {
			// Only add a hyphen if the last character wasn't already a separator
			if !lastWasSeparator && slug.Len() > 0 {
				slug.WriteRune('-')
				lastWasSeparator = true
			}
		}
	}

	// Trim trailing hyphens
	result := slug.String()
	return strings.TrimRight(result, "-")
}

// extractRepoPath extracts the repository path from a git URL
func extractRepoPath(repoURL string) string {
	// Handle git@github.com:org/repo.git format
	if strings.HasPrefix(repoURL, "git@github.com:") {
		path := strings.TrimPrefix(repoURL, "git@github.com:")
		return strings.TrimSuffix(path, ".git")
	}

	// Handle https://github.com/org/repo.git format
	if strings.HasPrefix(repoURL, "https://github.com/") {
		path := strings.TrimPrefix(repoURL, "https://github.com/")
		return strings.TrimSuffix(path, ".git")
	}

	return repoURL
}

func getAvailablePipelineSlug(ctx context.Context, f *factory.Factory, pipelineSlug, pipelineName string) (string, error) {
	// Check if the original slug is available
	pipeline, resp, err := f.RestAPIClient.Pipelines.Get(ctx, f.Config.OrganizationSlug(), pipelineSlug)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return pipelineSlug, nil // Original slug is available
		}
		return "", fmt.Errorf("failed to validate pipeline name")
	}

	// If a pipeline slug exists but with the same name, return a 422 error
	if pipeline.Name == pipelineName {
		return "", fmt.Errorf("a pipeline with the name '%s' already exists", pipelineName)
	}

	// Slug is taken, find the next available one by appending a counter
	counter := 1
	for {
		newSlug := fmt.Sprintf("%s-%d", pipelineSlug, counter)
		pipeline, resp, err := f.RestAPIClient.Pipelines.Get(ctx, f.Config.OrganizationSlug(), newSlug)
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				return newSlug, nil // Found an available slug
			}
			return "", fmt.Errorf("failed to validate pipeline name")
		}

		// If a pipeline slug exists but with the same name, return a 422 error
		if pipeline.Name == pipelineName {
			return "", fmt.Errorf("a pipeline with the name '%s' already exists", pipelineName)
		}

		counter++
		// Safety check to prevent infinite loops
		if counter > 1000 {
			return "", fmt.Errorf("unable to find available slug after 1000 attempts")
		}
	}
}

func getClusterUrl(orgSlug, clusterID string) string {
	if clusterID == "" {
		return ""
	}
	return fmt.Sprintf("https://api.buildkite.com/v2/organizations/%s/clusters/%s", orgSlug, clusterID)
}

func getCreatedByDetails(ctx context.Context, f *factory.Factory) *buildkite.User {
	user, _, err := f.RestAPIClient.User.CurrentUser(ctx)
	if err != nil {
		return nil
	}
	return &user
}
