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
		// mock a response that doesn't match the current repository url
		transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/test.git"}]`)),
			}, nil
		})
		client := &http.Client{Transport: transport}

		bkClient := buildkite.NewClient(client)
		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testOrg")
		f := factory.Factory{
			Config:        conf,
			RestAPIClient: bkClient,
			HttpClient:    client,
			GitRepository: testRepository(),
		}
		pipelines, err := resolveFromRepository(&f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 0 {
			t.Errorf("Expected 0 pipeline, got %d", len(pipelines))
		}

	})

	t.Run("one pipeline", func(t *testing.T) {
		// mock a response with a single pipeline matching the current repo url
		transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"}]`)),
			}, nil
		})
		client := &http.Client{Transport: transport}

		bkClient := buildkite.NewClient(client)
		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testOrg")
		f := factory.Factory{
			Config:        conf,
			RestAPIClient: bkClient,
			HttpClient:    client,
			GitRepository: testRepository(),
		}
		pipelines, err := resolveFromRepository(&f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 1 {
			t.Errorf("Expected 1 pipeline, got %d", len(pipelines))
		}
	})

	t.Run("multiple pipelines", func(t *testing.T) {
		// mock a response with 2 pipelines matching the current repo url
		transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"},
						{"slug": "my-pipeline-2", "repository": "git@github.com:buildkite/cli.git"}]`)),
			}, nil
		})
		client := &http.Client{Transport: transport}
		bkClient := buildkite.NewClient(client)
		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testOrg")
		f := factory.Factory{
			Config:        conf,
			RestAPIClient: bkClient,
			HttpClient:    client,
			GitRepository: testRepository(),
		}
		pipelines, err := resolveFromRepository(&f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 2 {
			t.Errorf("Expected 2 pipeline, got %d", len(pipelines))
		}
	})
}

func testRepository() *git.Repository {
	repo, _ := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	return repo
}
