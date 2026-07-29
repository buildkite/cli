package build

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/goccy/go-yaml"
)

type buildListOptions struct {
	duration string
	message  string
}

func applyClientSideFilters(builds []buildkite.Build, opts buildListOptions) ([]buildkite.Build, error) {
	cmd := &ListCmd{
		Duration: opts.duration,
		Message:  opts.message,
	}
	return cmd.applyClientSideFilters(builds)
}

func TestBuildListOptions_MetaData(t *testing.T) {
	cmd := &ListCmd{
		MetaData: map[string]string{
			"env":    "production",
			"deploy": "true",
		},
	}

	opts, err := cmd.buildListOptions()
	if err != nil {
		t.Fatalf("buildListOptions failed: %v", err)
	}

	if len(opts.MetaData.MetaData) != 2 {
		t.Errorf("Expected 2 meta-data filters, got %d", len(opts.MetaData.MetaData))
	}

	if opts.MetaData.MetaData["env"] != "production" {
		t.Errorf("Expected env=production, got env=%s", opts.MetaData.MetaData["env"])
	}

	if opts.MetaData.MetaData["deploy"] != "true" {
		t.Errorf("Expected deploy=true, got deploy=%s", opts.MetaData.MetaData["deploy"])
	}
}

func TestBuildListOptions_EmptyMetaData(t *testing.T) {
	cmd := &ListCmd{}

	opts, err := cmd.buildListOptions()
	if err != nil {
		t.Fatalf("buildListOptions failed: %v", err)
	}

	if len(opts.MetaData.MetaData) != 0 {
		t.Errorf("Expected empty meta-data, got %d entries", len(opts.MetaData.MetaData))
	}
}

func TestBuildListOptions_Summary(t *testing.T) {
	cmd := &ListCmd{Summary: true}

	opts, err := cmd.buildListOptions()
	if err != nil {
		t.Fatalf("buildListOptions failed: %v", err)
	}

	if !opts.ExcludeJobs || !opts.ExcludePipeline {
		t.Fatalf("summary options = %+v, want jobs and pipeline excluded", opts)
	}
}

func TestDisplayBuildSummaries_StructuredShape(t *testing.T) {
	builds := []buildkite.Build{{
		ID:      "build-id",
		Number:  42,
		State:   "passed",
		Message: "Ship it",
		Branch:  "main",
		Commit:  "abcdef",
		WebURL:  "https://buildkite.com/acme/widgets/builds/42",
		Jobs:    []buildkite.Job{{ID: "job-id"}},
		Pipeline: &buildkite.Pipeline{
			ID: "pipeline-id",
		},
	}}

	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var buf bytes.Buffer
			if err := displayBuildSummaries(builds, "acme", "widgets", format, &buf); err != nil {
				t.Fatalf("displayBuildSummaries failed: %v", err)
			}

			jsonData := buf.Bytes()
			if format == output.FormatYAML {
				var err error
				jsonData, err = yaml.YAMLToJSON(jsonData)
				if err != nil {
					t.Fatalf("convert YAML to JSON: %v", err)
				}
			}

			var got []map[string]any
			if err := json.Unmarshal(jsonData, &got); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if len(got) != 1 || got[0]["number"] != float64(42) || got[0]["state"] != "passed" {
				t.Fatalf("unexpected summary: %#v", got)
			}
			if got[0]["organization"] != "acme" || got[0]["pipeline"] != "widgets" {
				t.Errorf("summary target = %v/%v, want acme/widgets", got[0]["organization"], got[0]["pipeline"])
			}
			for _, excluded := range []string{"jobs", "artifacts", "annotations"} {
				if _, ok := got[0][excluded]; ok {
					t.Errorf("summary unexpectedly contains %q: %#v", excluded, got[0])
				}
			}
		})
	}
}

func TestDisplayBuildSummaries_Text(t *testing.T) {
	var buf bytes.Buffer
	builds := []buildkite.Build{{Number: 42, State: "failed", Message: "Deploy\nwith a detailed commit body", Branch: "main", WebURL: "https://example.test/42"}}

	if err := displayBuildSummaries(builds, "", "", output.FormatText, &buf); err != nil {
		t.Fatalf("displayBuildSummaries failed: %v", err)
	}

	for _, want := range []string{"42", "failed", "Deploy", "main", "https://example.test/42"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("summary text %q does not contain %q", buf.String(), want)
		}
	}
	if strings.Contains(buf.String(), "detailed commit body") {
		t.Errorf("summary text includes the multi-line commit body: %q", buf.String())
	}
}

func TestDisplayBuilds_EmptyJSON(t *testing.T) {
	var buf bytes.Buffer
	err := displayBuilds([]buildkite.Build{}, output.FormatJSON, &buf)
	if err != nil {
		t.Fatalf("displayBuilds failed: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "[]" {
		t.Errorf("Expected empty JSON array '[]', got %q", got)
	}
}

func TestDisplayBuilds_EmptyYAML(t *testing.T) {
	var buf bytes.Buffer
	err := displayBuilds([]buildkite.Build{}, output.FormatYAML, &buf)
	if err != nil {
		t.Fatalf("displayBuilds failed: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "[]" {
		t.Errorf("Expected empty YAML array '[]', got %q", got)
	}
}

func TestFilterBuilds(t *testing.T) {
	now := time.Now()
	builds := []buildkite.Build{
		{
			Number:     1,
			Message:    "Fast build",
			StartedAt:  &buildkite.Timestamp{Time: now.Add(-5 * time.Minute)},
			FinishedAt: &buildkite.Timestamp{Time: now.Add(-4 * time.Minute)}, // 1 minute
		},
		{
			Number:     2,
			Message:    "Long build",
			StartedAt:  &buildkite.Timestamp{Time: now.Add(-30 * time.Minute)},
			FinishedAt: &buildkite.Timestamp{Time: now.Add(-10 * time.Minute)}, // 20 minutes
		},
	}

	opts := buildListOptions{duration: "10m"}
	filtered, err := applyClientSideFilters(builds, opts)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 build >= 10m, got %d", len(filtered))
	}

	opts = buildListOptions{message: "Fast"}
	filtered, err = applyClientSideFilters(builds, opts)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 build with 'Fast', got %d", len(filtered))
	}
}
