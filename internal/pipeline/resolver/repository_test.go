package resolver

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/afero"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestResolvePipelinesFromPath(t *testing.T) {
	t.Parallel()

	t.Run("no pipelines found", func(t *testing.T) {
		t.Parallel()
		// mock a response that doesn't match the current repository url
		client := mockHttpClient(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/test.git"}]`)
		f := testFactory(client, "testOrg", testRepository())
		pipelines, err := resolveFromRepository(f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 0 {
			t.Errorf("Expected 0 pipeline, got %d", len(pipelines))
		}

	})

	t.Run("one pipeline", func(t *testing.T) {
		t.Parallel()
		// mock an http client response with a single pipeline matching the current repo url
		client := mockHttpClient(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"}]`)
		f := testFactory(client, "testOrg", testRepository())
		pipelines, err := resolveFromRepository(f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 1 {
			t.Errorf("Expected 1 pipeline, got %d", len(pipelines))
		}
	})

	t.Run("multiple pipelines", func(t *testing.T) {
		t.Parallel()
		// mock an http client response with 2 pipelines matching the current repo url
		client := mockHttpClient(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"},
		{"slug": "my-pipeline-2", "repository": "git@github.com:buildkite/cli.git"}]`)
		f := testFactory(client, "testOrg", testRepository())
		pipelines, err := resolveFromRepository(f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 2 {
			t.Errorf("Expected 2 pipeline, got %d", len(pipelines))
		}
	})

	t.Run("no repository found", func(t *testing.T) {
		client := mockHttpClient(`[{"slug": "", "repository": ""}]`)
		f := testFactory(client, "testOrg", nil)
		pipelines, err := resolveFromRepository(f)
		if pipelines != nil {
			t.Errorf("Expected nil, got %v", pipelines)
		}
		if err != nil {
			t.Errorf("Expected nil, got error: %s", err)
		}
	})

	t.Run("no remote repository found", func(t *testing.T) {
		client := mockHttpClient(`[{"slug": "", "repository": ""}]`)
		f := testFactory(client, "testOrg", testRepository())
		pipelines, err := resolveFromRepository(f)
		if pipelines != nil {
			t.Errorf("Expected nil, got %v", pipelines)
		}
		if err != nil {
			t.Errorf("Expected nil, got error: %s", err)
		}
	})
}

func testRepository() *git.Repository {
	repo, _ := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	return repo
}

func testFactory(client *http.Client, org string, repo *git.Repository) *factory.Factory {
	bkClient := buildkite.NewClient(client)
	conf := config.New(afero.NewMemMapFs(), nil)
	conf.SelectOrganization(org)
	return &factory.Factory{
		Config:        conf,
		RestAPIClient: bkClient,
		HttpClient:    client,
		GitRepository: repo,
	}
}

func mockHttpClient(response string) *http.Client {
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(response)),
		}, nil
	})
	return &http.Client{Transport: transport}
}
