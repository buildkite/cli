package build

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/build/view"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

func TestViewCmdCreatorSelection(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		creator     string
		buildNumber int
		userCalls   int
	}{
		{name: "latest from any creator", buildNumber: 43},
		{name: "mine", args: []string{"--mine"}, creator: "current-user", buildNumber: 42, userCalls: 1},
		{name: "explicit user", args: []string{"--user", "other-user"}, creator: "other-user", buildNumber: 41},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var userCalls, detailNumber int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v2/user":
					userCalls++
					_ = json.NewEncoder(w).Encode(map[string]string{"id": "current-user"})
				case "/v2/organizations/acme/pipelines/widgets/builds":
					creator := r.URL.Query().Get("creator")
					if creator != tt.creator {
						t.Errorf("creator = %q, want %q", creator, tt.creator)
					}
					if branch := r.URL.Query().Get("branch[]"); branch != "feature" {
						t.Errorf("branch = %q, want feature", branch)
					}
					number := 43
					switch creator {
					case "current-user":
						number = 42
					case "other-user":
						number = 41
					}
					_ = json.NewEncoder(w).Encode([]buildkite.Build{{Number: number}})
				default:
					var number int
					if _, err := fmt.Sscanf(r.URL.Path, "/v2/organizations/acme/pipelines/widgets/builds/%d", &number); err != nil {
						t.Errorf("unexpected request: %s", r.URL.Path)
						http.NotFound(w, r)
						return
					}
					detailNumber = number
					_ = json.NewEncoder(w).Encode(buildkite.Build{Number: number, State: "passed"})
				}
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")

			var cmd ViewCmd
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""})
			args := append([]string{"--pipeline", "widgets", "--branch", "feature", "--summary", "--json"}, tt.args...)
			ctx, err := parser.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Run(ctx, cli.Globals{NoInput: true, Quiet: true, NoPager: true}); err != nil {
				t.Fatal(err)
			}
			if detailNumber != tt.buildNumber || userCalls != tt.userCalls {
				t.Fatalf("viewed build %d with %d user lookups; want build %d with %d lookups", detailNumber, userCalls, tt.buildNumber, tt.userCalls)
			}
		})
	}
}

func TestViewCmdPreservesMixedCaseSlugsInAPIPath(t *testing.T) {
	const wantPath = "/v2/organizations/ExampleOrg/pipelines/Example-Pipeline/builds/177"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("request path = %q, want %q", r.URL.Path, wantPath)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buildkite.Build{Number: 177, State: "passed"})
	}))
	defer server.Close()

	t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "ConfiguredOrg")

	var cmd ViewCmd
	parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""})
	ctx, err := parser.Parse([]string{
		"177",
		"--pipeline", "ExampleOrg/Example-Pipeline",
		"--summary",
		"--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Run(ctx, cli.Globals{NoInput: true, Quiet: true, NoPager: true}); err != nil {
		t.Fatal(err)
	}
}

func TestViewCmd_BuildGetOptions_WithJobStates(t *testing.T) {
	cmd := &ViewCmd{
		JobStates: []string{"failed", "broken"},
	}

	opts := cmd.buildGetOptions()
	if opts == nil {
		t.Fatal("Expected non-nil BuildGetOptions")
		return
	}

	if len(opts.JobStates) != 2 {
		t.Fatalf("Expected 2 job states, got %d", len(opts.JobStates))
	}

	if opts.JobStates[0] != "failed" {
		t.Errorf("Expected first state to be 'failed', got %q", opts.JobStates[0])
	}

	if opts.JobStates[1] != "broken" {
		t.Errorf("Expected second state to be 'broken', got %q", opts.JobStates[1])
	}
}

func TestViewCmd_BuildGetOptions_Empty(t *testing.T) {
	cmd := &ViewCmd{}

	opts := cmd.buildGetOptions()
	if opts != nil {
		t.Errorf("Expected nil BuildGetOptions when no job states, got %+v", opts)
	}
}

func TestViewCmd_BuildGetOptions_SingleState(t *testing.T) {
	cmd := &ViewCmd{
		JobStates: []string{"running"},
	}

	opts := cmd.buildGetOptions()
	if opts == nil {
		t.Fatal("Expected non-nil BuildGetOptions")
		return
	}

	if len(opts.JobStates) != 1 {
		t.Fatalf("Expected 1 job state, got %d", len(opts.JobStates))
	}

	if opts.JobStates[0] != "running" {
		t.Errorf("Expected state to be 'running', got %q", opts.JobStates[0])
	}
}

func TestViewCmd_BuildGetOptions_Summary(t *testing.T) {
	opts := (&ViewCmd{Summary: true}).buildGetOptions()
	if opts == nil || !opts.ExcludeJobs || !opts.ExcludePipeline {
		t.Fatalf("summary options = %+v, want jobs and pipeline excluded", opts)
	}
}

func TestViewCmd_SummaryAndWebAreIncompatible(t *testing.T) {
	var cli struct {
		View ViewCmd `cmd:""`
	}
	parser := kong.Must(&cli, kong.Vars{"output_default_format": ""})

	if _, err := parser.Parse([]string{"view", "--summary", "--web"}); err == nil {
		t.Fatal("expected --summary --web to be rejected")
	}
}

func TestFetchBuildDetails_SummarySkipsArtifactsAndAnnotations(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Query().Get("exclude_jobs") != "true" || r.URL.Query().Get("exclude_pipeline") != "true" {
			t.Errorf("summary request query = %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(buildkite.Build{Number: 42, State: "passed"})
	}))
	defer server.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	f := &factory.Factory{RestAPIClient: client}
	cmd := &ViewCmd{Summary: true}

	build, artifacts, annotations, err := cmd.fetchBuildDetails(context.Background(), f, view.ViewOptions{
		Organization: "acme",
		Pipeline:     "widgets",
		BuildNumber:  42,
	})
	if err != nil {
		t.Fatalf("fetchBuildDetails failed: %v", err)
	}
	if build.Number != 42 || len(artifacts) != 0 || len(annotations) != 0 {
		t.Fatalf("unexpected details: build=%+v artifacts=%+v annotations=%+v", build, artifacts, annotations)
	}
	if len(paths) != 1 || strings.Contains(paths[0], "artifacts") || strings.Contains(paths[0], "annotations") {
		t.Fatalf("summary requested unexpected paths: %v", paths)
	}
}

func TestBuildSummaryOutput_StructuredAndText(t *testing.T) {
	build := buildkite.Build{
		ID:      "build-id",
		Number:  42,
		State:   "passed",
		Message: "Ship it\nwith a detailed commit body",
		Branch:  "main",
		Commit:  "abcdef",
		WebURL:  "https://example.test/42",
		Jobs:    []buildkite.Job{{ID: "job-id"}},
		Pipeline: &buildkite.Pipeline{
			ID: "pipeline-id",
		},
	}
	summary := newBuildSummaryOutput(build, "acme", "widgets")

	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		var buf strings.Builder
		if err := output.Write(&buf, summary, format); err != nil {
			t.Fatalf("write %s: %v", format, err)
		}
		for _, excluded := range []string{"jobs", "artifacts", "annotations"} {
			if strings.Contains(buf.String(), excluded) {
				t.Errorf("%s summary unexpectedly contains %q: %s", format, excluded, buf.String())
			}
		}
		if !strings.Contains(buf.String(), "widgets") || strings.Contains(buf.String(), "pipeline-id") {
			t.Errorf("%s summary should contain only the pipeline slug: %s", format, buf.String())
		}
	}

	text := summary.TextOutput()
	for _, want := range []string{"acme/widgets", "#42", "passed", "Ship it", "main", "https://example.test/42"} {
		if !strings.Contains(text, want) {
			t.Errorf("summary text %q does not contain %q", text, want)
		}
	}
	if strings.Contains(text, "detailed commit body") {
		t.Errorf("summary text includes the multi-line commit body: %q", text)
	}
}
