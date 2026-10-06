package repositoryconnection

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"gopkg.in/yaml.v3"
)

const (
	testOrg          = "acme"
	testConnectionID = "01234567-89ab-cdef-0123-456789abcdef"
)

const githubConnectionJSON = `{
  "id": "01234567-89ab-cdef-0123-456789abcdef",
  "type": "github_code_access_app",
  "display_name": "GitHub (acme)",
  "url": "https://api.buildkite.com/v2/organizations/acme/repository_connections/01234567-89ab-cdef-0123-456789abcdef",
  "service_account": {"login": "acme"},
  "host": {"type": "github", "url": "https://github.com"},
  "rate_limit": {"limit": 5000, "used": 42, "remaining": 4958, "reset_at": "2999-07-16T06:00:00Z"}
}`

const gitlabConnectionJSON = `{
  "id": "01234567-89ab-cdef-0123-456789abcdef",
  "type": "gitlab_self_managed",
  "display_name": "GitLab Self-Managed",
  "url": "https://api.buildkite.com/v2/organizations/acme/repository_connections/01234567-89ab-cdef-0123-456789abcdef",
  "service_account": null,
  "host": null,
  "rate_limit": null
}`

func testFactory(t *testing.T, path, body string, status int) *factory.Factory {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != path {
			t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.Path, path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	return &factory.Factory{RestAPIClient: client, Quiet: true, NoInput: true}
}

func listPath() string {
	return "/v2/organizations/" + testOrg + "/repository_connections"
}

func viewPath() string {
	return listPath() + "/" + testConnectionID
}

func TestListCmd(t *testing.T) {
	body := `[
  {"id": "github-uuid", "type": "github_code_access_app", "display_name": "GitHub (acme)", "url": "u1"},
  {"id": "gitlab-uuid", "type": "gitlab_self_managed", "display_name": "GitLab Self-Managed", "url": "u2"}
]`
	f := testFactory(t, listPath(), body, http.StatusOK)

	var stdout bytes.Buffer
	if err := (&ListCmd{}).run(context.Background(), f, testOrg, &stdout, output.FormatJSON); err != nil {
		t.Fatal(err)
	}
	var connections []buildkite.RepositoryConnection
	if err := json.Unmarshal(stdout.Bytes(), &connections); err != nil {
		t.Fatal(err)
	}
	if len(connections) != 2 || connections[1].ID != "gitlab-uuid" || connections[1].Type != "gitlab_self_managed" {
		t.Fatalf("connections = %#v", connections)
	}

	stdout.Reset()
	if err := (&ListCmd{}).run(context.Background(), f, testOrg, &stdout, output.FormatText); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Repository Connections (2) in acme", "GitHub (acme)", "gitlab_self_managed", "gitlab-uuid", "bk repository-connection view"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("text output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestListCmdEmpty(t *testing.T) {
	f := testFactory(t, listPath(), `[]`, http.StatusOK)

	var stdout bytes.Buffer
	if err := (&ListCmd{}).run(context.Background(), f, testOrg, &stdout, output.FormatJSON); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "[]" {
		t.Fatalf("JSON output = %q, want []", got)
	}

	stdout.Reset()
	if err := (&ListCmd{}).run(context.Background(), f, testOrg, &stdout, output.FormatText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "No repository connections found") {
		t.Fatalf("text output = %q", stdout.String())
	}
}

func TestListCmdAPIError(t *testing.T) {
	f := testFactory(t, listPath(), `{"message":"Forbidden"}`, http.StatusForbidden)

	err := (&ListCmd{}).run(context.Background(), f, testOrg, &bytes.Buffer{}, output.FormatText)
	if err == nil || !strings.Contains(err.Error(), "error fetching repository connections") || !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("error = %v", err)
	}
}

func TestViewCmdShowsGitHubRateLimit(t *testing.T) {
	f := testFactory(t, viewPath(), githubConnectionJSON, http.StatusOK)
	cmd := ViewCmd{ConnectionID: testConnectionID}

	var stdout bytes.Buffer
	if err := cmd.run(context.Background(), f, testOrg, &stdout, output.FormatJSON); err != nil {
		t.Fatal(err)
	}
	var connection buildkite.RepositoryConnection
	if err := json.Unmarshal(stdout.Bytes(), &connection); err != nil {
		t.Fatal(err)
	}
	want := buildkite.RepositoryConnectionRateLimit{
		Limit: 5000, Used: 42, Remaining: 4958,
		ResetAt: time.Date(2999, 7, 16, 6, 0, 0, 0, time.UTC),
	}
	if connection.RateLimit == nil || *connection.RateLimit != want {
		t.Fatalf("rate_limit = %#v, want %#v", connection.RateLimit, want)
	}
	if connection.ServiceAccount == nil || connection.ServiceAccount.Login != "acme" {
		t.Fatalf("service_account = %#v", connection.ServiceAccount)
	}

	stdout.Reset()
	if err := cmd.run(context.Background(), f, testOrg, &stdout, output.FormatText); err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	for _, want := range []string{
		"Repository Connection: GitHub (acme)",
		"not the Buildkite API rate limit",
		"5000", "42", "4958", "2999-07-16T06:00:00Z",
		"https://github.com",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("text output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "stale") || strings.Contains(text, "Not available") {
		t.Fatalf("current rate limit was reported as stale or unavailable:\n%s", text)
	}
}

func TestViewCmdMarksPassedResetAsStale(t *testing.T) {
	text := renderConnectionText(buildkite.RepositoryConnection{
		DisplayName: "GitHub (acme)",
		RateLimit: &buildkite.RepositoryConnectionRateLimit{
			Limit: 5000, Remaining: 0, ResetAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	})
	if !strings.Contains(text, "2020-01-01T00:00:00Z (passed; used and remaining are stale)") {
		t.Fatalf("passed reset time is not marked stale:\n%s", text)
	}
}

func TestViewCmdReportsUnavailableRateLimit(t *testing.T) {
	f := testFactory(t, viewPath(), gitlabConnectionJSON, http.StatusOK)
	cmd := ViewCmd{ConnectionID: testConnectionID}

	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var stdout bytes.Buffer
			if err := cmd.run(context.Background(), f, testOrg, &stdout, format); err != nil {
				t.Fatal(err)
			}
			fields := map[string]any{}
			var err error
			if format == output.FormatJSON {
				err = json.Unmarshal(stdout.Bytes(), &fields)
			} else {
				err = yaml.Unmarshal(stdout.Bytes(), &fields)
			}
			if err != nil {
				t.Fatal(err)
			}
			if fields["type"] != "gitlab_self_managed" {
				t.Fatalf("output = %#v", fields)
			}
			for _, key := range []string{"rate_limit", "service_account", "host"} {
				value, ok := fields[key]
				if !ok || value != nil {
					t.Fatalf("%s = %#v (present %v), want explicit null:\n%s", key, value, ok, stdout.String())
				}
			}
		})
	}

	var stdout bytes.Buffer
	if err := cmd.run(context.Background(), f, testOrg, &stdout, output.FormatText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Not available") {
		t.Fatalf("text output does not say the rate limit is unavailable:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Remaining") {
		t.Fatalf("text output shows rate limit values for an unavailable quota:\n%s", stdout.String())
	}
}

func TestViewCmdAPIError(t *testing.T) {
	f := testFactory(t, viewPath(), `{"message":"Not Found"}`, http.StatusNotFound)

	err := (&ViewCmd{ConnectionID: testConnectionID}).run(context.Background(), f, testOrg, &bytes.Buffer{}, output.FormatText)
	if err == nil || !strings.Contains(err.Error(), "error fetching repository connection") || !strings.Contains(err.Error(), "Not Found") {
		t.Fatalf("error = %v", err)
	}
}
