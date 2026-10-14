package pipeline

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
)

func TestIsGitHubURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "GitHub SSH URL",
			url:      "git@github.com:org/repo.git",
			expected: true,
		},
		{
			name:     "GitHub HTTPS URL",
			url:      "https://github.com/org/repo.git",
			expected: true,
		},
		{
			name:     "GitHub HTTPS URL without .git",
			url:      "https://github.com/org/repo",
			expected: true,
		},
		{
			name:     "GitHub Enterprise SSH URL",
			url:      "git@github.mycompany.com:org/repo.git",
			expected: true,
		},
		{
			name:     "GitHub Enterprise HTTPS URL",
			url:      "https://github.mycompany.com/org/repo.git",
			expected: true,
		},
		{
			name:     "GitLab SSH URL",
			url:      "git@gitlab.com:org/repo.git",
			expected: false,
		},
		{
			name:     "GitLab HTTPS URL",
			url:      "https://gitlab.com/org/repo.git",
			expected: false,
		},
		{
			name:     "Bitbucket SSH URL",
			url:      "git@bitbucket.org:org/repo.git",
			expected: false,
		},
		{
			name:     "Bitbucket HTTPS URL",
			url:      "https://bitbucket.org/org/repo.git",
			expected: false,
		},
		{
			name:     "empty string",
			url:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isGitHubURL(tt.url)
			if got != tt.expected {
				t.Errorf("isGitHubURL(%q) = %v, want %v", tt.url, got, tt.expected)
			}
		})
	}
}

func TestGetRepositoryURL(t *testing.T) {
	t.Parallel()

	t.Run("returns flag value when provided", func(t *testing.T) {
		t.Parallel()
		f := &factory.Factory{}
		repoURL := getRepositoryURL(f, "git@github.com:org/repo.git")
		if repoURL != "git@github.com:org/repo.git" {
			t.Errorf("expected flag value, got %q", repoURL)
		}
	})

	t.Run("returns empty when no flag and nil git repo", func(t *testing.T) {
		t.Parallel()
		f := &factory.Factory{}
		repoURL := getRepositoryURL(f, "")
		if repoURL != "" {
			t.Errorf("expected empty string, got %q", repoURL)
		}
	})

	t.Run("returns empty when factory is nil and no flag", func(t *testing.T) {
		t.Parallel()
		repoURL := getRepositoryURL(nil, "")
		if repoURL != "" {
			t.Errorf("expected empty string, got %q", repoURL)
		}
	})

	t.Run("returns flag value even when factory is nil", func(t *testing.T) {
		t.Parallel()
		repoURL := getRepositoryURL(nil, "git@github.com:org/repo.git")
		if repoURL != "git@github.com:org/repo.git" {
			t.Errorf("expected flag value, got %q", repoURL)
		}
	})
}

func TestExtractRepoPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "GitHub SSH", input: "git@github.com:org/repo.git", expected: "org/repo"},
		{name: "GitHub HTTPS", input: "https://github.com/org/repo.git", expected: "org/repo"},
		{name: "GitHub HTTPS no .git", input: "https://github.com/org/repo", expected: "org/repo"},
		{name: "non-GitHub URL", input: "git@gitlab.com:org/repo.git", expected: "git@gitlab.com:org/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractRepoPath(tt.input)
			if got != tt.expected {
				t.Errorf("extractRepoPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCreateResolvedInputsAndDryRunSafety(t *testing.T) {
	for _, tc := range []struct {
		name, origin, explicitName, explicitRepo, wantName, wantRepo string
	}{
		{"SSH origin", "git@github.com:acme/widgets.git", "", "", "widgets", "git@github.com:acme/widgets.git"},
		{"HTTPS origin", "https://github.com/acme/widgets.git/", "", "", "widgets", "https://github.com/acme/widgets.git/"},
		{"SSH URL origin", "ssh://git@example.com:2222/acme/widgets.git", "", "", "widgets", "ssh://git@example.com:2222/acme/widgets.git"},
		{"overrides", "git@github.com:acme/widgets.git", "Custom Pipeline", "https://example.com/another.git", "Custom Pipeline", "https://example.com/another.git"},
		{"no origin", "", "", "https://example.com/explicit.git", "checkout", "https://example.com/explicit.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "checkout")
			repo, err := git.PlainInit(root, false)
			if err != nil {
				t.Fatal(err)
			}
			if tc.origin != "" {
				if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{tc.origin}}); err != nil {
					t.Fatal(err)
				}
			}
			sub := filepath.Join(root, "src")
			os.Mkdir(sub, 0o755)
			t.Chdir(sub)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
			t.Setenv("BUILDKITE_CREDENTIAL_STORE", "shm")
			t.Setenv("BUILDKITE_CREDENTIAL_STORE_PATH", filepath.Join(t.TempDir(), "credentials"))
			mutations := 0
			var created buildkite.CreatePipeline
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					if strings.Contains(r.URL.Path, "/pipelines/") {
						w.WriteHeader(http.StatusNotFound)
						fmtBody := `{"message":"not found"}`
						w.Write([]byte(fmtBody))
						return
					}
					w.Write([]byte(`{"name":"Test User"}`))
					return
				}
				mutations++
				if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
					t.Error(err)
				}
				json.NewEncoder(w).Encode(buildkite.Pipeline{Name: created.Name, Repository: created.Repository, Configuration: created.Configuration})
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			for _, dry := range []bool{true, false} {
				var cmd struct {
					Create CreateCmd `cmd:""`
				}
				var out, stderr bytes.Buffer
				parser, err := kong.New(&cmd, kong.Writers(&out, &stderr), kong.Vars{"output_default_format": ""})
				if err != nil {
					t.Fatal(err)
				}
				args := []string{"create", "--json"}
				if tc.explicitName != "" {
					args = append(args, tc.explicitName)
				}
				ctx, err := parser.Parse(args)
				if err != nil {
					t.Fatal(err)
				}
				cmd.Create.Repository = tc.explicitRepo
				cmd.Create.DryRun = dry
				if dry {
					cmd.Create.Template = "go"
					cmd.Create.CreateWebhook = true
				}
				if err := cmd.Create.Run(ctx, cli.Globals{NoInput: true, Quiet: true}); err != nil {
					t.Fatal(err)
				}
				var result buildkite.Pipeline
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Name != tc.wantName || result.Repository != tc.wantRepo {
					t.Fatalf("dry=%t: got %q / %q, want %q / %q", dry, result.Name, result.Repository, tc.wantName, tc.wantRepo)
				}
				if dry && mutations != 0 {
					t.Fatal("dry-run mutated remote state")
				}
				if _, err := os.Stat(filepath.Join(root, ".buildkite")); !os.IsNotExist(err) {
					t.Fatalf("dry-run/noninteractive create wrote local files: %v", err)
				}
			}
			if mutations != 1 || !strings.Contains(created.Configuration, "buildkite-agent pipeline upload") {
				t.Fatalf("expected one bootstrap pipeline creation, got %d / %q", mutations, created.Configuration)
			}
		})
	}
}
