package resolver

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

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
    ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
    defer cancel()
		// mock a response that doesn't match the current repository url
		client := mockHttpClient(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/test.git"}]`)
		f := testFactory(client, "testOrg", testRepository(false, false))
		pipelines, err := resolveFromRepository(ctx, f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 0 {
			t.Errorf("Expected 0 pipeline, got %d", len(pipelines))
		}
	})

	t.Run("one pipeline", func(t *testing.T) {
		t.Parallel()
    ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
    defer cancel()
		// mock an http client response with a single pipeline matching the current repo url
		client := mockHttpClient(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"}]`)
		f := testFactory(client, "testOrg", testRepository(true, true))
		pipelines, err := resolveFromRepository(ctx, f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 1 {
			t.Errorf("Expected 1 pipeline, got %d", len(pipelines))
		}
	})

	t.Run("multiple pipelines", func(t *testing.T) {
		t.Parallel()
    ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
    defer cancel()
		// mock an http client response with 2 pipelines matching the current repo url
		client := mockHttpClient(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"},
		{"slug": "my-pipeline-2", "repository": "git@github.com:buildkite/cli.git"}]`)
		f := testFactory(client, "testOrg", testRepository(true, true))
		pipelines, err := resolveFromRepository(ctx, f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 2 {
			t.Errorf("Expected 2 pipeline, got %d", len(pipelines))
		}
	})

	t.Run("no repository found", func(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
    defer cancel()
		client := mockHttpClient(`[{"slug": "", "repository": ""}]`)
		f := testFactory(client, "testOrg", nil)
		pipelines, err := resolveFromRepository(ctx, f)
		if pipelines != nil {
			t.Errorf("Expected nil, got %v", pipelines)
		}
		if err != nil {
			t.Errorf("Expected nil, got error: %s", err)
		}
	})

	t.Run("no remote repository found", func(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
    defer cancel()
		client := mockHttpClient(`[{"slug": "", "repository": ""}]`)
		f := testFactory(client, "testOrg", testRepository(true, true))
		pipelines, err := resolveFromRepository(ctx, f)
		if pipelines != nil {
			t.Errorf("Expected nil, got %v", pipelines)
		}
		if err != nil {
			t.Errorf("Expected nil, got error: %s", err)
		}
	})
}

func TestResolvePipelinesFromCwd(t *testing.T) {
	t.Parallel()

	t.Run("multiple pipelines", func(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
    defer cancel()
		// mock an http client response with 2 pipelines matching the current repo url
		client := mockHttpClient(`[{"slug": "this-is-testDir-pipeline-2"},
		{"slug": "this-is-testDir-pipeline-2"}]`)
		f := testFactory(client, "testOrg", testRepository(false, false))
		pipelines, err := resolveFromRepository(ctx, f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 2 {
			t.Errorf("Expected 2 pipeline, got %d", len(pipelines))
		}
	})
}

// By accepting an arg for the detect and enable values, we can "skip" the looking
// for a .git file and test the directory resolver
func testRepository(detect, enable bool) *git.Repository {
	repo, _ := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: detect, EnableDotGitCommonDir: enable})
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
