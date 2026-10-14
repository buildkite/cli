package secret

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

type githubRepository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Owner         struct {
		ID int64 `json:"id"`
	} `json:"owner"`
}

type migrationGrantRequest struct {
	Policy            string   `json:"policy"`
	SecretNames       []string `json:"secret_names"`
	RepositoryID      int64    `json:"repository_id"`
	RepositoryOwnerID int64    `json:"repository_owner_id"`
	WorkflowPath      string   `json:"workflow_path"`
	DefaultBranchRef  string   `json:"default_branch_ref"`
	WorkflowSHA       string   `json:"workflow_sha"`
}

type migrationGrant struct {
	ID           string `json:"id"`
	MigrationURL string `json:"migration_url"`
	Audience     string `json:"audience"`
}

type githubCommandRunner interface {
	Run(context.Context, []string, []byte) ([]byte, error)
}

type execGitHubRunner struct {
	stderr io.Writer
}

func (r execGitHubRunner) Run(ctx context.Context, args []string, stdin []byte) ([]byte, error) {
	command := exec.CommandContext(ctx, "gh", args...)
	command.Stdin = bytes.NewReader(stdin)
	command.Stderr = r.stderr
	output, err := command.Output()
	if err != nil {
		return output, err
	}
	return output, nil
}

type migrationBuildkiteClient interface {
	GetPipeline(context.Context, string, string) (buildkite.Pipeline, error)
	ListPipelines(context.Context, string, string, int) ([]buildkite.Pipeline, error)
	ListSecrets(context.Context, string, string, int) ([]buildkite.ClusterSecret, error)
	CreateGrant(context.Context, string, string, migrationGrantRequest) (migrationGrant, error)
}

type migrationBuildkiteAPI struct {
	client *buildkite.Client
}

func (api migrationBuildkiteAPI) GetPipeline(ctx context.Context, organization, pipeline string) (buildkite.Pipeline, error) {
	result, _, err := api.client.Pipelines.Get(ctx, organization, pipeline)
	return result, err
}

func (api migrationBuildkiteAPI) ListPipelines(ctx context.Context, organization, repository string, page int) ([]buildkite.Pipeline, error) {
	result, _, err := api.client.Pipelines.List(ctx, organization, &buildkite.PipelineListOptions{
		Repository:  repository,
		ListOptions: buildkite.ListOptions{Page: page, PerPage: 100},
	})
	return result, err
}

func (api migrationBuildkiteAPI) ListSecrets(ctx context.Context, organization, cluster string, page int) ([]buildkite.ClusterSecret, error) {
	result, _, err := api.client.ClusterSecrets.List(ctx, organization, cluster, &buildkite.ClusterSecretsListOptions{
		ListOptions: buildkite.ListOptions{Page: page, PerPage: 100},
	})
	return result, err
}

func (api migrationBuildkiteAPI) CreateGrant(ctx context.Context, organization, cluster string, input migrationGrantRequest) (migrationGrant, error) {
	endpoint := fmt.Sprintf("v2/organizations/%s/clusters/%s/github-actions-secret-migrations", url.PathEscape(organization), url.PathEscape(cluster))
	request, err := api.client.NewRequest(ctx, http.MethodPost, endpoint, input)
	if err != nil {
		return migrationGrant{}, err
	}
	request.Header.Set("Accept", "application/json")
	var grant migrationGrant
	_, err = api.client.Do(request, &grant)
	return grant, err
}
