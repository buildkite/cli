package artifacts

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

func TestDisplayArtifactsRendersJobIDAsBuildkiteURL(t *testing.T) {
	// Wide enough that the URL column doesn't get truncated.
	t.Setenv("BUILDKITE_TABLE_MAX_WIDTH", "300")

	artifacts := []buildkite.Artifact{
		{ID: "art-1", Path: "logs/rspec.json", FileSize: 1024, JobID: "job-1"},
	}
	const baseURL = "https://buildkite.com/organizations/acme/pipelines/monolith/builds/429"

	var buf bytes.Buffer
	if err := displayArtifacts(artifacts, &buf, baseURL); err != nil {
		t.Fatalf("displayArtifacts() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"ID", "PATH", "SIZE", "URL",
		"art-1", "logs/rspec.json", "1.0KB",
		baseURL + "/jobs/job-1/artifacts/art-1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
}

func TestDisplayArtifactsFallsBackToArtifactURL(t *testing.T) {
	t.Parallel()

	artifacts := []buildkite.Artifact{
		{ID: "art-2", Path: "output.zip", URL: "https://api.example.com/artifacts/art-2"},
	}

	var buf bytes.Buffer
	if err := displayArtifacts(artifacts, &buf, "https://buildkite.com/x"); err != nil {
		t.Fatalf("displayArtifacts() error = %v", err)
	}
	if !strings.Contains(buf.String(), "https://api.example.com/artifacts/art-2") {
		t.Errorf("expected artifact URL to be used when JobID is empty:\n%s", buf.String())
	}
}

func TestDisplayArtifactsRendersDashWhenNoURL(t *testing.T) {
	t.Parallel()

	artifacts := []buildkite.Artifact{{ID: "art-3", Path: "orphan.txt"}}

	var buf bytes.Buffer
	if err := displayArtifacts(artifacts, &buf, "https://buildkite.com/x"); err != nil {
		t.Fatalf("displayArtifacts() error = %v", err)
	}
	// Table columns render "-" for artifacts with no JobID and no URL.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var dataLine string
	for _, l := range lines {
		if strings.Contains(l, "orphan.txt") {
			dataLine = l
			break
		}
	}
	if dataLine == "" {
		t.Fatalf("no data line for orphan.txt in:\n%s", buf.String())
	}
	if !strings.Contains(dataLine, "-") {
		t.Errorf("expected '-' placeholder for empty URL in row: %q", dataLine)
	}
}

func TestDisplayArtifactsEmpty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := displayArtifacts(nil, &buf, "https://buildkite.com/x"); err != nil {
		t.Fatalf("displayArtifacts() error = %v", err)
	}
	// Headers should still render even when there are no rows.
	for _, want := range []string{"ID", "PATH", "SIZE", "URL"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output missing header %q\n%s", want, buf.String())
		}
	}
}

func TestListCmdFlagParsing(t *testing.T) {
	t.Parallel()

	var cmd ListCmd
	parser, err := kong.New(&cmd, kong.Vars{"output_default_format": ""})
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	if _, err := parser.Parse([]string{
		"429",
		"-p", "monolith",
		"--job-uuid", "job-uuid-1",
		"--path", "log/rspec*.json",
		"--state", "Finished",
	}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cmd.BuildNumber != "429" {
		t.Errorf("BuildNumber = %q, want 429", cmd.BuildNumber)
	}
	if cmd.Pipeline != "monolith" {
		t.Errorf("Pipeline = %q, want monolith", cmd.Pipeline)
	}
	if cmd.JobUUID != "job-uuid-1" {
		t.Errorf("JobUUID = %q, want job-uuid-1", cmd.JobUUID)
	}
	if cmd.Path != "log/rspec*.json" {
		t.Errorf("Path = %q, want log/rspec*.json", cmd.Path)
	}
	if cmd.State != "Finished" {
		t.Errorf("State = %q, want Finished (parser preserves casing)", cmd.State)
	}
}

func TestListCmdBuildNumberOptional(t *testing.T) {
	t.Parallel()

	var cmd ListCmd
	parser, err := kong.New(&cmd, kong.Vars{"output_default_format": ""})
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	if _, err := parser.Parse([]string{"--state", "finished"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cmd.BuildNumber != "" {
		t.Errorf("BuildNumber = %q, want empty", cmd.BuildNumber)
	}
	if cmd.State != "finished" {
		t.Errorf("State = %q, want finished", cmd.State)
	}
}

func TestListCmdHelpMentionsFilters(t *testing.T) {
	t.Parallel()

	var cmd ListCmd
	help := cmd.Help()
	for _, want := range []string{"--path", "--state", "log/rspec*.json", "bk artifacts list"} {
		if !strings.Contains(help, want) {
			t.Errorf("Help() missing %q", want)
		}
	}
}
