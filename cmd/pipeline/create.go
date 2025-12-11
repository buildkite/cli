package pipeline

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type CreateCmd struct {
	Name        string `arg:"" help:"Name of the pipeline" required:""`
	Description string `help:"Description of the pipeline" short:"d"`
	Repository  string `help:"Repository URL" short:"r"`
	ClusterID   string `help:"Cluster name or ID to assign the pipeline to" short:"c"`
	DryRun      bool   `help:"Simulate pipeline creation without actually creating it"`
	Output      string `help:"Outputs the created pipeline. One of: json, yaml, text" short:"o" default:"text"`
}

func (c *CreateCmd) Help() string {
	return `Creates a new pipeline in the current org and outputs the URL to the pipeline.

You can specify a --dry-run flag to see the pipeline that would be created without
actually creating it. This outputs a JSON representation of the pipeline to be created by default.

The --cluster-id flag accepts either a cluster name or cluster ID. If a name is provided,
it will be automatically resolved to the corresponding cluster ID.

Examples:
  # Create a new pipeline
  $ bk pipeline create "My Pipeline" --description "My pipeline description" --repository "git@github.com:org/repo.git"

  # Create a new pipeline and view the created pipeline in JSON format
  $ bk pipeline create "My Pipeline" --description "My pipeline description" --repository "git@github.com:org/repo.git" --output json

  # Create a pipeline with a cluster (by name)
  $ bk pipeline create "My Pipeline" -d "Description" -r "git@github.com:org/repo.git" -c "my-cluster"

  # Create a pipeline with a cluster (by ID)
  $ bk pipeline create "My Pipeline" -d "Description" -r "git@github.com:org/repo.git" -c "cluster-id-123"

  # Simulate creating a pipeline and view the output in yaml format
  $ bk pipeline create "My Pipeline" -d "Description" -r "git@github.com:org/repo.git" --dry-run --output yaml
`
}

func (c *CreateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	if c.DryRun {
		return c.runPipelineCreateDryRun(kongCtx, f)
	}
	return c.runPipelineCreate(kongCtx, f)
}

func (c *CreateCmd) runPipelineCreateDryRun(kongCtx *kong.Context, f *factory.Factory) error {
	ctx := context.Background()
	format := output.Format(c.Output)

	pipeline, err := c.createPipelineDryRun(ctx, f)
	if err != nil {
		return err
	}
	// for dry-run, if text format is requested, always default to json
	if format == output.FormatText {
		format = output.FormatJSON
	}
	return output.Write(kongCtx.Stdout, pipeline, format)
}

func (c *CreateCmd) runPipelineCreate(kongCtx *kong.Context, f *factory.Factory) error {

	ctx := context.Background()
	format := output.Format(c.Output)

	pipeline, err := c.createPipeline(ctx, f)
	if err != nil {
		return err
	}
	if format != output.FormatText {
		return output.Write(kongCtx.Stdout, pipeline, format)
	}
	fmt.Printf("%s\n", pipeline.WebURL)
	return nil
}

func (c *CreateCmd) createPipeline(ctx context.Context, f *factory.Factory) (*buildkite.Pipeline, error) {
	// Resolve cluster name to ID if provided
	clusterID, err := resolveClusterID(ctx, f, c.ClusterID)
	if err != nil {
		return nil, err
	}

	var pipeline buildkite.Pipeline
	var resp *buildkite.Response

	spinErr := bkIO.SpinWhile(f, fmt.Sprintf("Creating pipeline %s", c.Name), func() {
		createPipeline := buildkite.CreatePipeline{
			Name:          c.Name,
			Repository:    c.Repository,
			Description:   c.Description,
			ClusterID:     clusterID,
			Configuration: "steps:\n  - label: \":pipeline:\"\n    command: buildkite-agent pipeline upload",
		}

		pipeline, resp, err = f.RestAPIClient.Pipelines.Create(ctx, f.Config.OrganizationSlug(), createPipeline)
	})

	if spinErr != nil {
		return nil, spinErr
	}

	if err != nil {
		// Check if this is a 422 error (validation failed)
		if resp != nil && resp.StatusCode == http.StatusUnprocessableEntity {
			// Try to find an existing pipeline with the same name
			if existingPipeline := c.findPipelineByName(ctx, f); existingPipeline != nil {
				return nil, fmt.Errorf("a pipeline with the name '%s' already exists: %s", c.Name, existingPipeline.WebURL)
			}
		}
		return nil, err
	}

	return &pipeline, nil
}

func (c *CreateCmd) findPipelineByName(ctx context.Context, f *factory.Factory) *buildkite.Pipeline {
	opts := buildkite.PipelineListOptions{
		ListOptions: buildkite.ListOptions{
			PerPage: 100,
		},
	}

	pipelines, _, err := f.RestAPIClient.Pipelines.List(ctx, f.Config.OrganizationSlug(), &opts)
	if err != nil {
		return nil
	}

	for _, p := range pipelines {
		if p.Name == c.Name {
			return &p
		}
	}

	return nil
}

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

func (c *CreateCmd) createPipelineDryRun(ctx context.Context, f *factory.Factory) (*PipelineDryRun, error) {
	pipelineSlug := generateSlug(c.Name)

	pipelineSlug, err := getAvailablePipelineSlug(ctx, f, pipelineSlug, c.Name)
	if err != nil {
		return nil, err
	}

	orgSlug := f.Config.OrganizationSlug()
	pipeline := initialisePipelineDryRun()

	pipeline.ID = "00000000-0000-0000-0000-000000000000"
	pipeline.GraphQLID = "UGlwZWxpbmUtLS0wMDAwMDAwMC0wMDAwLTAwMDAtMDAwMC0wMDAwMDAwMDAwMDA="
	pipeline.URL = fmt.Sprintf("https://api.buildkite.com/v2/organizations/%s/pipelines/%s", orgSlug, pipelineSlug)
	pipeline.WebURL = fmt.Sprintf("https://buildkite.com/%s/%s", orgSlug, pipelineSlug)
	pipeline.Name = c.Name
	pipeline.Description = c.Description
	pipeline.Slug = pipelineSlug
	pipeline.Repository = c.Repository
	pipeline.ClusterID = c.ClusterID
	pipeline.ClusterURL = getClusterUrl(orgSlug, c.ClusterID)
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
			Repository:          extractRepoPath(c.Repository),
		},
	}

	pipeline.CreatedBy = getCreatedByDetails(ctx, f)

	return &pipeline, nil
}

func generateSlug(name string) string {
	name = strings.TrimSpace(name)

	var slug strings.Builder
	lastWasSeparator := false

	for _, c := range strings.ToLower(name) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			slug.WriteRune(c)
			lastWasSeparator = false
		} else if c == ' ' || c == '-' || c == '_' {
			if !lastWasSeparator && slug.Len() > 0 {
				slug.WriteRune('-')
				lastWasSeparator = true
			}
		}
	}

	result := slug.String()
	return strings.TrimRight(result, "-")
}

func extractRepoPath(repoURL string) string {
	if strings.HasPrefix(repoURL, "git@github.com:") {
		path := strings.TrimPrefix(repoURL, "git@github.com:")
		return strings.TrimSuffix(path, ".git")
	}

	if strings.HasPrefix(repoURL, "https://github.com/") {
		path := strings.TrimPrefix(repoURL, "https://github.com/")
		return strings.TrimSuffix(path, ".git")
	}

	return repoURL
}

func getAvailablePipelineSlug(ctx context.Context, f *factory.Factory, pipelineSlug, pipelineName string) (string, error) {
	pipeline, resp, err := f.RestAPIClient.Pipelines.Get(ctx, f.Config.OrganizationSlug(), pipelineSlug)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return pipelineSlug, nil
		}
		return "", fmt.Errorf("failed to validate pipeline name")
	}

	if pipeline.Name == pipelineName {
		return "", fmt.Errorf("a pipeline with the name '%s' already exists", pipelineName)
	}

	counter := 1
	for {
		newSlug := fmt.Sprintf("%s-%d", pipelineSlug, counter)
		pipeline, resp, err := f.RestAPIClient.Pipelines.Get(ctx, f.Config.OrganizationSlug(), newSlug)
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				return newSlug, nil
			}
			return "", fmt.Errorf("failed to validate pipeline name")
		}

		if pipeline.Name == pipelineName {
			return "", fmt.Errorf("a pipeline with the name '%s' already exists", pipelineName)
		}

		counter++
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

func getClusters(ctx context.Context, f *factory.Factory) (map[string]string, error) {
	clusterMap := make(map[string]string)
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

func listClusterNames(ctx context.Context, f *factory.Factory) ([]string, error) {
	clusterMap, err := getClusters(ctx, f)
	if err != nil {
		return nil, err
	}

	clusterNames := make([]string, 0, len(clusterMap))
	for name := range clusterMap {
		clusterNames = append(clusterNames, name)
	}
	sort.Strings(clusterNames)

	return clusterNames, nil
}

func resolveClusterID(ctx context.Context, f *factory.Factory, clusterNameOrID string) (string, error) {
	if clusterNameOrID == "" {
		return "", nil
	}

	// First, try to get clusters map
	clusterMap, err := getClusters(ctx, f)
	if err != nil {
		return "", fmt.Errorf("failed to fetch clusters: %w", err)
	}

	// Check if it's a cluster name
	if clusterID, exists := clusterMap[clusterNameOrID]; exists {
		return clusterID, nil
	}

	// Check if it's already a valid cluster ID
	for _, id := range clusterMap {
		if id == clusterNameOrID {
			return clusterNameOrID, nil
		}
	}

	// Not found - provide helpful error with available clusters
	clusterNames, _ := listClusterNames(ctx, f)
	if len(clusterNames) > 0 {
		return "", fmt.Errorf("cluster '%s' not found. Available clusters: %s", clusterNameOrID, strings.Join(clusterNames, ", "))
	}

	return "", fmt.Errorf("cluster '%s' not found", clusterNameOrID)
}

func getCreatedByDetails(ctx context.Context, f *factory.Factory) *buildkite.User {
	user, _, err := f.RestAPIClient.User.CurrentUser(ctx)
	if err != nil {
		return nil
	}
	return &user
}
