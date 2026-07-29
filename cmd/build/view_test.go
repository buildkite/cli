package build

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/build/view"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

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
