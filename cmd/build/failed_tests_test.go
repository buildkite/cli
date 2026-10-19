package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/keyring"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"gopkg.in/yaml.v3"
)

func TestFailedTestsCmd(t *testing.T) {
	const buildID = "0190046e-e199-453b-a302-a21a4d649d32"
	const buildPath = "/v2/organizations/acme/pipelines/widgets/builds/42"
	const testsPath = "/v2/analytics/organizations/acme/builds/" + buildID + "/tests"
	for _, tt := range []struct {
		name      string
		args      []string
		empty     bool
		failAt    string
		missingID bool
		stored    bool
		pipelines []string
		wantError string
	}{
		{name: "qualified build JSON", args: []string{"acme/widgets/42", "--json"}},
		{name: "number and pipeline YAML", args: []string{"42", "-p", "acme/widgets", "--yaml"}},
		{name: "URL and target org credentials", args: []string{"https://buildkite.com/acme/widgets/builds/42"}, stored: true},
		{name: "unique configured pipeline", args: []string{"42"}, pipelines: []string{"widgets"}},
		{name: "ambiguous pipeline JSON", args: []string{"42", "--json"}, pipelines: []string{"widgets", "other"}, wantError: "use --pipeline"},
		{name: "ambiguous pipeline YAML", args: []string{"42", "--yaml"}, pipelines: []string{"widgets", "other"}, wantError: "use --pipeline"},
		{name: "empty array", empty: true},
		{name: "build denied", failAt: "build", wantError: "requires read_builds"},
		{name: "tests denied", failAt: "1", wantError: "requires read_suites"},
		{name: "later page denied", failAt: "3", wantError: "failed to list Test Engine tests"},
		{name: "missing UUID", missingID: true, wantError: "build has no UUID"},
		{name: "invalid build", args: []string{"not-a-build"}, wantError: "unable to parse"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var pages []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("unexpected method or credentials for %s", r.URL)
				}
				page := r.URL.Query().Get("page")
				if (tt.failAt == "build" && r.URL.Path == buildPath) || (tt.failAt == page && r.URL.Path == testsPath) {
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"message":"Access denied"}`))
					return
				}
				switch r.URL.Path {
				case buildPath:
					if r.URL.Query().Get("exclude_jobs") != "true" || r.URL.Query().Get("exclude_pipeline") != "true" {
						t.Errorf("heavyweight build lookup: %s", r.URL)
					}
					id := buildID
					if tt.missingID {
						id = ""
					}
					// A passing build can still have tests that failed before retry.
					_ = json.NewEncoder(w).Encode(buildkite.Build{ID: id, Number: 42, State: "passed"})
				case testsPath:
					query := r.URL.Query()
					if query.Get("tags") != "result:~failed" || query.Has("labels") || query.Has("state") || query.Get("per_page") != "100" {
						t.Errorf("incorrect failed-test filter: %s", r.URL)
					}
					pages = append(pages, page)
					if tt.empty {
						_, _ = w.Write([]byte(`[]`))
						return
					}
					switch page {
					case "1":
						// Follow next even on a short page, without assuming page + 1.
						w.Header().Set("Link", fmt.Sprintf(`<https://api.buildkite.com%s?page=3>; rel="next"`, testsPath))
						_, _ = w.Write([]byte(`[{"id":"unlabelled","scope":"checkout","name":"rejects expired cards","location":"checkout_test.go:42","labels":[],"executions_count_by_result":{"passed":2,"failed":1}}]`))
					case "3":
						_, _ = w.Write([]byte(`[{"id":"labelled","scope":"accounts","name":"creates user","labels":["flaky"],"web_url":"https://buildkite.com/test-2","executions_count_by_result":{"passed":0,"failed":3}}]`))
					default:
						t.Errorf("unexpected page: %s", page)
					}
				default:
					t.Errorf("unexpected URL: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Chdir(t.TempDir())
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			t.Setenv(keyring.CredentialStorePathEnv, filepath.Join(t.TempDir(), "credentials.json"))
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "other-org")
			t.Setenv("BUILDKITE_OUTPUT_FORMAT", "json")
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			keyring.MockForTesting()
			t.Cleanup(keyring.ResetForTesting)
			if tt.stored {
				t.Setenv("BUILDKITE_API_TOKEN", "")
				if err := keyring.New().Set("acme", "test-token"); err != nil {
					t.Fatal(err)
				}
			}
			if len(tt.pipelines) > 0 {
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")
				data, err := yaml.Marshal(map[string]any{"selected_org": "acme", "pipelines": tt.pipelines})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(".bk.yaml", data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			args := tt.args
			if len(args) == 0 {
				args = []string{"acme/widgets/42"}
			}
			var cmd FailedTestsCmd
			var stdout, stderr bytes.Buffer
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(&stdout, &stderr))
			ctx, err := parser.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			err = cmd.Run(ctx, cli.Globals{Quiet: true, NoPager: true})
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) || stdout.Len() != 0 {
					t.Fatalf("error = %v, output = %q; want %q and no partial output", err, stdout.String(), tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var result failedTestsOutput
			if slices.Contains(args, "--yaml") {
				err = yaml.Unmarshal(stdout.Bytes(), &result)
			} else {
				err = json.Unmarshal(stdout.Bytes(), &result)
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Organization != "acme" || result.Pipeline != "widgets" || result.BuildNumber != 42 || result.BuildID != buildID {
				t.Fatalf("wrong build association: %+v", result)
			}
			if !strings.Contains(result.Note, "No matches does not prove") || !strings.Contains(result.Note, "not a per-failure") {
				t.Fatalf("missing data/classification caveats: %q", result.Note)
			}
			if tt.empty {
				if result.Tests == nil || len(result.Tests) != 0 || !strings.Contains(result.TextOutput(), "No matching tests found") {
					t.Fatalf("empty result = %+v", result)
				}
				return
			}
			if strings.Join(pages, ",") != "1,3" || len(result.Tests) != 2 {
				t.Fatalf("lost pages: pages = %v, tests = %+v", pages, result.Tests)
			}
			first, second := result.Tests[0], result.Tests[1]
			if first.ID != "unlabelled" || len(first.Labels) != 0 || first.ExecutionsCountByResult["passed"] != 2 || first.ExecutionsCountByResult["failed"] != 1 || second.ID != "labelled" || !slices.Equal(second.Labels, []string{"flaky"}) || second.ExecutionsCountByResult["failed"] != 3 {
				t.Fatalf("lost tests, labels, or retry metrics: %+v", result.Tests)
			}
			for _, want := range []string{"acme/widgets/42", "checkout", "rejects expired cards", "checkout_test.go:42", "https://buildkite.com/test-2"} {
				if !strings.Contains(result.TextOutput(), want) {
					t.Errorf("text output missing %q: %s", want, result.TextOutput())
				}
			}
		})
	}
}
