package resolver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/spf13/afero"
)

func TestResolvePipelinesFromPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const testOrg = "testOrg"

	t.Run("no pipelines found", func(t *testing.T) {
		t.Parallel()
		// mock a response that doesn't match the current repository url
		s := mockHTTPServer(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/test.git"}]`)
		t.Cleanup(s.Close)

		f := testFactory(t, s.URL, testOrg, testRepository(t, "https://github.com/buildkite/cli.git"))
		pipelines, err := resolveFromRepository(ctx, f, testOrg)
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

		f := testFactory(t, s.URL, testOrg, testRepository(t, "https://github.com/buildkite/cli.git"))
		pipelines, err := resolveFromRepository(ctx, f, testOrg)
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

		f := testFactory(t, s.URL, testOrg, testRepository(t, "https://github.com/buildkite/cli.git"))
		pipelines, err := resolveFromRepository(ctx, f, testOrg)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 2 {
			t.Errorf("Expected 2 pipeline, got %d", len(pipelines))
		}
	})

	t.Run("normalizes repository queries for ssh and https equivalents", func(t *testing.T) {
		var queries []string
		s := mockHTTPServerWithHandler(t, func(r *http.Request) any {
			query := r.URL.Query().Get("repository")
			queries = append(queries, query)
			if query != "buildkite/cli" {
				return []map[string]string{}
			}

			return []map[string]string{{
				"slug":       "my-pipeline",
				"repository": "git@github.com:buildkite/cli.git",
			}}
		})
		t.Cleanup(s.Close)

		f := testFactory(t, s.URL, testOrg, testRepository(t, "https://github.com/buildkite/cli.git"))
		pipelines, err := resolveFromRepository(ctx, f, testOrg)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 1 {
			t.Errorf("Expected 1 pipeline, got %d", len(pipelines))
		}
		if !slices.Contains(queries, "buildkite/cli") {
			t.Errorf("Expected normalized repository query, got %v", queries)
		}
	})

	t.Run("does not match sibling repositories when the remote omits .git", func(t *testing.T) {
		t.Parallel()
		s := mockHTTPServer(`[{"slug": "agent", "repository": "git@github.com:buildkite/agent"}, {"slug": "agent-private", "repository": "git@github.com:buildkite/agent-private"}]`)
		t.Cleanup(s.Close)

		f := testFactory(t, s.URL, testOrg, testRepository(t, "https://github.com/buildkite/agent"))
		pipelines, err := resolveFromRepository(ctx, f, testOrg)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 1 {
			t.Errorf("Expected 1 pipeline, got %d", len(pipelines))
		}
		if len(pipelines) == 1 && pipelines[0].Name != "agent" {
			t.Errorf("Expected agent pipeline, got %s", pipelines[0].Name)
		}
	})

	t.Run("no repository found", func(t *testing.T) {
		s := mockHTTPServer(`[{"slug": "", "repository": ""}]`)
		t.Cleanup(s.Close)

		f := testFactory(t, s.URL, testOrg, nil)
		pipelines, err := resolveFromRepository(ctx, f, testOrg)
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

		f := testFactory(t, s.URL, testOrg, testRepository(t))
		pipelines, err := resolveFromRepository(ctx, f, testOrg)
		if pipelines != nil {
			t.Errorf("Expected nil, got %v", pipelines)
		}
		if err != nil {
			t.Errorf("Expected nil, got error: %s", err)
		}
	})
}

func TestResolvePipelinesFromGitFallback(t *testing.T) {
	ctx := context.Background()
	const testOrg = "testOrg"

	s := mockHTTPServer(`[{"slug": "cli-resolver-smoke", "repository": "git@github.com:buildkite/cli.git"}]`)
	t.Cleanup(s.Close)

	repo := testRepository(t, "https://github.com/buildkite/cli.git")
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree returned error: %v", err)
	}
	t.Chdir(wt.Filesystem.Root())

	f := testFactory(t, s.URL, testOrg, nil)
	pipelines, err := resolveFromRepository(ctx, f, testOrg)
	if err != nil {
		t.Errorf("Error: %s", err)
	}
	if len(pipelines) != 1 {
		t.Errorf("Expected 1 pipeline, got %d", len(pipelines))
	}
	if len(pipelines) == 1 && pipelines[0].Name != "cli-resolver-smoke" {
		t.Errorf("Expected cli-resolver-smoke pipeline, got %s", pipelines[0].Name)
	}
}

func testRepository(t *testing.T, remoteURLs ...string) *git.Repository {
	t.Helper()

	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit returned error: %v", err)
	}
	if len(remoteURLs) == 0 {
		return repo
	}

	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: remoteURLs})
	if err != nil {
		t.Fatalf("CreateRemote returned error: %v", err)
	}

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

func mockHTTPServerWithHandler(t *testing.T, handler func(*http.Request) any) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(handler(r)); err != nil {
			t.Errorf("Encode returned error: %v", err)
		}
	}))
}
