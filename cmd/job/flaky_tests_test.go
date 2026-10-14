package job

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"gopkg.in/yaml.v3"
)

func TestFlakyTestsCmd(t *testing.T) {
	const jobID = "0190046e-e199-453b-a302-a21a4d649d31"
	const buildID = "0190046e-e199-453b-a302-a21a4d649d32"
	const buildPath = "/v2/organizations/buildkite/pipelines/cli/builds/42"
	const testsPath = "/v2/analytics/organizations/buildkite/builds/" + buildID + "/tests"
	for _, tt := range []struct {
		name      string
		flags     []string
		format    string
		empty     bool
		failAt    string
		state     string
		buildURL  string
		missingID bool
		jobID     string
		wantError string
	}{
		{name: "JSON all pages", flags: []string{"--json"}, format: "json"},
		{name: "YAML all pages", flags: []string{"--yaml"}, format: "yaml"},
		{name: "empty is not a negative verdict", format: "json", empty: true},
		{name: "job unavailable", failAt: "job", wantError: "failed to get job"},
		{name: "build unavailable", failAt: "build", wantError: "failed to get job's build"},
		{name: "Test Engine inaccessible", failAt: "tests", wantError: "requires read_suites"},
		{name: "later page fails", failAt: "page3", wantError: "failed to list Test Engine tests"},
		{name: "not a failed job", state: "passed", wantError: "not failed"},
		{name: "missing build URL", buildURL: "missing", wantError: "cannot resolve build"},
		{name: "wrong organization", buildURL: "https://api.buildkite.com/v2/organizations/other/pipelines/cli/builds/42", wantError: "cannot resolve build"},
		{name: "missing build UUID", missingID: true, wantError: "build has no UUID"},
		{name: "tag injection rejected", jobID: jobID + ",result:~passed", wantError: "invalid job UUID"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var pages []string
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				if r.Method != http.MethodGet {
					t.Errorf("method = %s, want GET", r.Method)
				}
				if (tt.failAt == "job" && r.URL.Path == "/v2/organizations/buildkite/jobs/"+jobID) ||
					(tt.failAt == "build" && r.URL.Path == buildPath) ||
					(tt.failAt == "tests" && r.URL.Path == testsPath) ||
					(tt.failAt == "page3" && r.URL.Query().Get("page") == "3") {
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"message":"Access denied"}`))
					return
				}
				switch r.URL.Path {
				case "/v2/organizations/buildkite/jobs/" + jobID:
					state := tt.state
					if state == "" {
						state = "failed"
					}
					buildURL := tt.buildURL
					switch buildURL {
					case "":
						// The host is intentionally not the configured endpoint. Only
						// the build path should be used; do not follow this host.
						buildURL = "https://api.buildkite.com" + buildPath
					case "missing":
						buildURL = ""
					}
					_ = json.NewEncoder(w).Encode(buildkite.Job{ID: jobID, State: state, BuildURL: buildURL})
				case buildPath:
					if r.URL.Query().Get("exclude_jobs") != "true" || r.URL.Query().Get("exclude_pipeline") != "true" {
						t.Errorf("build lookup must exclude jobs and pipeline: %s", r.URL)
					}
					id := buildID
					if tt.missingID {
						id = ""
					}
					_ = json.NewEncoder(w).Encode(buildkite.Build{ID: id, Number: 42})
				case testsPath:
					query := r.URL.Query()
					if query.Get("labels") != "flaky" || query.Get("tags") != "build.job_id:"+jobID+",result:~failed" || query.Get("per_page") != "100" {
						t.Errorf("incorrect filters: %s", r.URL)
					}
					page := query.Get("page")
					pages = append(pages, page)
					if tt.empty {
						_, _ = w.Write([]byte(`[]`))
						return
					}
					if page == "1" {
						// A short page still has a next link, which is authoritative.
						w.Header().Set("Link", fmt.Sprintf(`<https://api.buildkite.com%s?page=3>; rel="next"`, testsPath))
					} else if page != "3" {
						t.Errorf("unexpected page: %s", page)
					}
					_, _ = fmt.Fprintf(w, `[{"id":"test-%s","name":"fails sometimes %s","scope":"checkout","labels":["flaky"],"web_url":"https://buildkite.com/test-%s","executions_count_by_result":{"passed":2,"failed":1}}]`, page, page, page)
				default:
					t.Errorf("unexpected URL: %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "buildkite")
			t.Setenv("BUILDKITE_OUTPUT_FORMAT", "json")

			var cmd FlakyTestsCmd
			var stdout, stderr bytes.Buffer
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(&stdout, &stderr))
			inputID := tt.jobID
			if inputID == "" {
				inputID = jobID
			}
			ctx, err := parser.Parse(append([]string{inputID}, tt.flags...))
			if err != nil {
				t.Fatal(err)
			}
			err = cmd.Run(ctx, cli.Globals{NoInput: true, Quiet: true})
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) || stdout.Len() != 0 {
					t.Fatalf("error = %v, stdout = %q; want %q and no output", err, stdout.String(), tt.wantError)
				}
				if tt.jobID != "" && requests != 0 {
					t.Fatalf("invalid job UUID made %d requests", requests)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var result flakyTestsOutput
			if tt.format == "yaml" {
				err = yaml.Unmarshal(stdout.Bytes(), &result)
			} else {
				err = json.Unmarshal(stdout.Bytes(), &result)
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.JobID != jobID || result.BuildID != buildID || result.Organization != "buildkite" {
				t.Fatalf("wrong job/build association: %+v", result)
			}
			if !strings.Contains(result.Note, "No matches does not mean not flaky") {
				t.Fatalf("missing uncertainty note: %q", result.Note)
			}
			if tt.empty {
				if result.Tests == nil || len(result.Tests) != 0 || !strings.Contains(result.TextOutput(), "No matching tests found") {
					t.Fatalf("empty output = %+v", result)
				}
			} else {
				if strings.Join(pages, ",") != "1,3" || len(result.Tests) != 2 || result.Tests[0].ID != "test-1" || result.Tests[1].ID != "test-3" || result.Tests[1].ExecutionsCountByResult["failed"] != 1 {
					t.Fatalf("lost pages or test metrics: pages = %v, tests = %+v", pages, result.Tests)
				}
				text := result.TextOutput()
				for _, want := range []string{jobID, "checkout", "fails sometimes 3", "https://buildkite.com/test-3", "do not establish the cause"} {
					if !strings.Contains(text, want) {
						t.Errorf("text output missing %q: %s", want, text)
					}
				}
			}
		})
	}
}
