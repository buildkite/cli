package resolver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/afero"
)

func TestResolvePipelinesFromPath(t *testing.T) {
	t.Parallel()

	t.Run("no pipelines found", func(t *testing.T) {
		t.Parallel()
		// mock a response that doesn't match the current repository url
		s := mockHTTPServer(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/test.git"}]`)
		t.Cleanup(s.Close)

		f := testFactory(t, s.URL, "testOrg", testRepository())
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
		s := mockHTTPServer(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"}]`)
		t.Cleanup(s.Close)

		f := testFactory(t, s.URL, "testOrg", testRepository())
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
		s := mockHTTPServer(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"}, {"slug": "my-pipeline-2", "repository": "git@github.com:buildkite/cli.git"}]`)
		t.Cleanup(s.Close)

		f := testFactory(t, s.URL, "testOrg", testRepository())
		pipelines, err := resolveFromRepository(f)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 2 {
			t.Errorf("Expected 2 pipeline, got %d", len(pipelines))
		}
	})

	t.Run("no repository found", func(t *testing.T) {
		s := mockHTTPServer(`[{"slug": "", "repository": ""}]`)
		t.Cleanup(s.Close)

		f := testFactory(t, s.URL, "testOrg", nil)
		pipelines, err := resolveFromRepository(f)
		if pipelines != nil {
			t.Errorf("Expected nil, got %v", pipelines)
		}
		if err != nil {
			t.Errorf("Expected nil, got error: %s", err)
		}
	})

	t.Run("no remote repository found", func(t *testing.T) {
		s := mockHTTPServer(`[{"slug": "", "repository": ""}]`)
		t.Cleanup(s.Close)

		f := testFactory(t, s.URL, "testOrg", testRepository())
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

func testFactory(t *testing.T, serverURL string, org string, repo *git.Repository) *factory.Factory {
	t.Helper()

	bkClient, err := buildkite.NewOpts(buildkite.WithBaseURL(serverURL))
	if err != nil {
		t.Errorf("Error creating buildkite client: %s", err)
	}

	conf := config.New(afero.NewMemMapFs(), nil)
	conf.SelectOrganization(org, true)
	return &factory.Factory{
		Config:        conf,
		RestAPIClient: bkClient,
		GitRepository: repo,
	}
}

func mockHTTPServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	}))
}
