package build

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/build/view"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"gopkg.in/yaml.v3"
)

func TestRecursiveFailuresCurrentTriggerTree(t *testing.T) {
	trigger := func(id, pipeline string, number int) buildkite.Job {
		return buildkite.Job{ID: id, Type: "trigger", State: "passed", TriggeredBuild: &buildkite.TriggeredBuild{
			URL: fmt.Sprintf("https://api.buildkite.com/v2/organizations/acme/pipelines/%s/builds/%d", pipeline, number),
		}}
	}
	oldTrigger := trigger("superseded-trigger", "old", 99)
	oldTrigger.Retried = true
	softTrigger := trigger("soft-trigger", "sibling", 8)
	softTrigger.State, softTrigger.SoftFailed = "failed", true
	rootJobs := []buildkite.Job{
		{ID: "root-failure", Type: "script", State: "failed"},
		{ID: "old-failure", Type: "script", State: "failed", Retried: true},
		{ID: "soft-failure", Type: "script", State: "failed", SoftFailed: true},
		{ID: "broken", Type: "script", State: "broken"},
		{ID: "canceled", Type: "script", State: "canceled"},
		{ID: "expired", Type: "script", State: "expired"},
		{ID: "dependency", Type: "script", State: "waiting_failed"},
		{ID: "promised", Type: "script", State: "running", PromisedExitStatus: new(1)},
		{ID: "skipped-trigger", Type: "trigger", State: "skipped"},
		{ID: "broken-trigger", Type: "trigger", State: "broken"},
		oldTrigger,
	}
	// Embedded jobs are not a 30-item page. A trigger after the usual list page
	// boundary must still be discovered, even when its owning build passed.
	for range 40 {
		rootJobs = append(rootJobs, buildkite.Job{Type: "script", State: "passed"})
	}
	rootJobs = append(rootJobs, trigger("async-trigger", "child", 7), softTrigger, trigger("duplicate", "child", 7))
	builds := map[string]buildkite.Build{
		"/v2/organizations/acme/pipelines/root/builds/42": {ID: "root", Number: 42, State: "failed", Jobs: rootJobs},
		"/v2/organizations/acme/pipelines/child/builds/7": {
			ID: "child", Number: 7, State: "passed", Jobs: []buildkite.Job{trigger("nested-trigger", "leaf", 3)},
		},
		"/v2/organizations/acme/pipelines/leaf/builds/3": {
			ID: "leaf", Number: 3, State: "failed", Jobs: []buildkite.Job{{
				ID: "leaf-failure", Type: "script", State: "timed_out", Name: "Tests",
				WebURL: "https://buildkite.com/acme/leaf/builds/3#leaf-failure",
				LogURL: "https://api.buildkite.com/leaf/log", RawLogsURL: "https://api.buildkite.com/leaf/log.txt",
			}},
		},
		"/v2/organizations/acme/pipelines/sibling/builds/8": {
			ID: "sibling", Number: 8, State: "failed", Jobs: []buildkite.Job{{ID: "sibling-failure", Type: "script", State: "failed"}},
		},
	}
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		b, ok := builds[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request (no logs/artifacts/old attempts expected): %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.RawQuery != "exclude_pipeline=true" {
			t.Errorf("request must include all current jobs, query = %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(b)
	}))
	defer server.Close()
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	got := fetchRecursiveFailures(context.Background(), client, view.ViewOptions{Organization: "acme", Pipeline: "root", BuildNumber: 42})
	want := recursiveFailures{
		RootBuild: "acme/root/42", Complete: true, Issues: []traversalIssue{},
		Failures: []recursiveFailure{
			{Build: "acme/root/42", ID: "root-failure", State: "failed", TriggerPath: []string{}},
			{
				Build: "acme/leaf/3", ID: "leaf-failure", State: "timed_out", Name: "Tests",
				WebURL: "https://buildkite.com/acme/leaf/builds/3#leaf-failure",
				LogURL: "https://api.buildkite.com/leaf/log", RawLogURL: "https://api.buildkite.com/leaf/log.txt",
				TriggerPath: []string{"acme/root/42#async-trigger", "acme/child/7#nested-trigger"},
			},
			{Build: "acme/sibling/8", ID: "sibling-failure", State: "failed", TriggerPath: []string{"acme/root/42#soft-trigger"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recursive failures = %+v, want %+v", got, want)
	}
	if len(calls) != 4 {
		t.Fatalf("requests = %v, want exactly four distinct builds", calls)
	}
	for path, count := range calls {
		if count != 1 {
			t.Errorf("%s requested %d times", path, count)
		}
	}
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML, output.FormatText} {
		var buf strings.Builder
		if err := output.Write(&buf, got, format); err != nil {
			t.Fatal(err)
		}
		if format == output.FormatText {
			for _, value := range []string{"complete: true", "Command failures: 3", "acme/leaf/3", "timed_out", "Tests", "acme/child/7#nested-trigger", "https://api.buildkite.com/leaf/log.txt"} {
				if !strings.Contains(buf.String(), value) {
					t.Errorf("text missing %q: %s", value, buf.String())
				}
			}
			continue
		}
		var decoded recursiveFailures
		if format == output.FormatJSON {
			err = json.Unmarshal([]byte(buf.String()), &decoded)
		} else {
			err = yaml.Unmarshal([]byte(buf.String()), &decoded)
		}
		if err != nil || !reflect.DeepEqual(decoded, want) {
			t.Fatalf("%s output = %s, decode error = %v", format, buf.String(), err)
		}
	}
}

func TestRecursiveViewPartialOutputAndError(t *testing.T) {
	for _, rootFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("root request fails=%t", rootFails), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v2/organizations/acme/pipelines/root/builds/42":
					if rootFails {
						w.WriteHeader(http.StatusForbidden)
						_, _ = fmt.Fprint(w, `{"message":"Forbidden"}`)
						return
					}
					jobs := []buildkite.Job{
						{ID: "missing", Type: "trigger", State: "failed"},
						{ID: "pending", Type: "trigger", State: "scheduled"},
						{ID: "invalid", Type: "trigger", State: "passed", TriggeredBuild: &buildkite.TriggeredBuild{URL: "https://example.test/not-a-build"}},
					}
					for _, pipeline := range []string{"denied", "deleted", "unavailable", "good"} {
						jobs = append(jobs, buildkite.Job{ID: pipeline, Type: "trigger", State: "passed", TriggeredBuild: &buildkite.TriggeredBuild{
							URL: "https://api.buildkite.com/v2/organizations/acme/pipelines/" + pipeline + "/builds/1",
						}})
					}
					_ = json.NewEncoder(w).Encode(buildkite.Build{ID: "root", Number: 42, Jobs: jobs})
				case "/v2/organizations/acme/pipelines/good/builds/1":
					_ = json.NewEncoder(w).Encode(buildkite.Build{ID: "good", Number: 1, Jobs: []buildkite.Job{{ID: "real-failure", Type: "script", State: "failed"}}})
				case "/v2/organizations/acme/pipelines/denied/builds/1":
					w.WriteHeader(http.StatusForbidden)
					_, _ = fmt.Fprint(w, `{"message":"Forbidden"}`)
				case "/v2/organizations/acme/pipelines/deleted/builds/1":
					http.NotFound(w, r)
				case "/v2/organizations/acme/pipelines/unavailable/builds/1":
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = fmt.Fprint(w, `{"message":"Unavailable"}`)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")
			var cmd ViewCmd
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""})
			ctx, err := parser.Parse([]string{"acme/root/42", "--recursive", "--json"})
			if err != nil {
				t.Fatal(err)
			}
			capture, err := os.CreateTemp(t.TempDir(), "stdout")
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			stdout := os.Stdout
			os.Stdout = capture
			defer func() { os.Stdout = stdout }()
			err = cmd.Run(ctx, cli.Globals{NoInput: true, Quiet: true, NoPager: true})
			os.Stdout = stdout
			if err == nil || !strings.Contains(err.Error(), "incomplete") {
				t.Fatalf("Run error = %v, want incomplete traversal", err)
			}
			data, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal(err)
			}
			var got recursiveFailures
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("partial output is not JSON: %s: %v", data, err)
			}
			if got.Complete || got.RootBuild != "acme/root/42" {
				t.Fatalf("unexpected partial result: %+v", got)
			}
			wantIssues, wantFailures := 6, 1
			if rootFails {
				wantIssues, wantFailures = 1, 0
			}
			if len(got.Issues) != wantIssues || len(got.Failures) != wantFailures {
				t.Fatalf("partial result = %+v, want %d issues and %d failures", got, wantIssues, wantFailures)
			}
			if !rootFails {
				if got.Failures[0].ID != "real-failure" || got.Failures[0].Build != "acme/good/1" {
					t.Fatalf("accessible sibling failure lost: %+v", got.Failures)
				}
				for i, build := range []string{"acme/root/42", "acme/root/42", "acme/root/42", "acme/denied/1", "acme/deleted/1", "acme/unavailable/1"} {
					if got.Issues[i].Build != build || got.Issues[i].Message == "" {
						t.Errorf("issue %d lost its provenance: %+v", i, got.Issues[i])
					}
				}
			}
			if text := got.TextOutput(); !strings.Contains(text, "complete: false") || !strings.Contains(text, "Incomplete: acme/root/42") {
				t.Fatalf("text hides incomplete traversal: %s", text)
			}
		})
	}
}

func TestRecursiveViewIncompatibleFlags(t *testing.T) {
	for _, flag := range []string{"--summary", "--web", "--job-states=failed"} {
		var cmd ViewCmd
		parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""})
		if _, err := parser.Parse([]string{"--recursive", flag}); err == nil {
			t.Errorf("--recursive %s should be rejected", flag)
		}
	}
}
