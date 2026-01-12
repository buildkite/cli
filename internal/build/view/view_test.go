package view

import (
	"strings"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestBuildSummary_NilBuild(t *testing.T) {
	result := BuildSummary(nil, "my-org", "my-pipeline")

	if !strings.Contains(result, "my-org") {
		t.Errorf("Expected result to contain organization, got: %s", result)
	}
	if !strings.Contains(result, "my-pipeline") {
		t.Errorf("Expected result to contain pipeline, got: %s", result)
	}
	if !strings.Contains(result, "no data available") {
		t.Errorf("Expected result to indicate no data available, got: %s", result)
	}
}

func TestBuildSummary_ValidBuild(t *testing.T) {
	build := &buildkite.Build{
		Number:  123,
		State:   "passed",
		Message: "Test build",
		Branch:  "main",
	}

	result := BuildSummary(build, "my-org", "my-pipeline")

	if !strings.Contains(result, "#123") {
		t.Errorf("Expected result to contain build number, got: %s", result)
	}
	if !strings.Contains(result, "passed") {
		t.Errorf("Expected result to contain state, got: %s", result)
	}
}

func TestBuildSummaryWithJobs_NilBuild(t *testing.T) {
	result := BuildSummaryWithJobs(nil, "my-org", "my-pipeline")

	if !strings.Contains(result, "my-org") {
		t.Errorf("Expected result to contain organization, got: %s", result)
	}
	if !strings.Contains(result, "no data available") {
		t.Errorf("Expected result to indicate no data available, got: %s", result)
	}
}

func TestBuildSummaryWithJobs_ValidBuild(t *testing.T) {
	build := &buildkite.Build{
		Number:  456,
		State:   "running",
		Message: "Test build with jobs",
		Jobs: []buildkite.Job{
			{Type: "script", Name: "Test Job", State: "passed"},
		},
	}

	result := BuildSummaryWithJobs(build, "my-org", "my-pipeline")

	if !strings.Contains(result, "#456") {
		t.Errorf("Expected result to contain build number, got: %s", result)
	}
	if !strings.Contains(result, "Jobs") {
		t.Errorf("Expected result to contain jobs section, got: %s", result)
	}
}

func TestBuildView_Render_NilBuild(t *testing.T) {
	view := NewBuildView(nil, nil, nil, "my-org", "my-pipeline")
	result := view.Render()

	if !strings.Contains(result, "my-org") {
		t.Errorf("Expected result to contain organization, got: %s", result)
	}
	if !strings.Contains(result, "no data available") {
		t.Errorf("Expected result to indicate no data available, got: %s", result)
	}
}

func TestBuildView_Render_ValidBuild(t *testing.T) {
	build := &buildkite.Build{
		Number:  789,
		State:   "passed",
		Message: "Test build",
		Jobs: []buildkite.Job{
			{Type: "script", Name: "Build", State: "passed"},
		},
	}
	artifacts := []buildkite.Artifact{
		{ID: "art-1", Path: "dist/app.js", FileSize: 1024},
	}
	annotations := []buildkite.Annotation{
		{Style: "info", Context: "test-context"},
	}

	view := NewBuildView(build, artifacts, annotations, "my-org", "my-pipeline")
	result := view.Render()

	if !strings.Contains(result, "#789") {
		t.Errorf("Expected result to contain build number, got: %s", result)
	}
	if !strings.Contains(result, "Jobs") {
		t.Errorf("Expected result to contain jobs section, got: %s", result)
	}
	if !strings.Contains(result, "Artifacts") {
		t.Errorf("Expected result to contain artifacts section, got: %s", result)
	}
	if !strings.Contains(result, "Annotations") {
		t.Errorf("Expected result to contain annotations section, got: %s", result)
	}
}

func TestCreatorName_NilBuild(t *testing.T) {
	result := creatorName(nil)

	if result != "Unknown" {
		t.Errorf("Expected 'Unknown' for nil build, got: %s", result)
	}
}

func TestCreatorName_WithCreator(t *testing.T) {
	build := &buildkite.Build{
		Creator: buildkite.Creator{
			ID:   "user-123",
			Name: "John Doe",
		},
	}

	result := creatorName(build)

	if result != "John Doe" {
		t.Errorf("Expected 'John Doe', got: %s", result)
	}
}

func TestCreatorName_WithAuthor(t *testing.T) {
	build := &buildkite.Build{
		Author: buildkite.Author{
			Username: "janedoe",
			Name:     "Jane Doe",
		},
	}

	result := creatorName(build)

	if result != "Jane Doe" {
		t.Errorf("Expected 'Jane Doe', got: %s", result)
	}
}

func TestCreatorName_NoCreatorOrAuthor(t *testing.T) {
	build := &buildkite.Build{}

	result := creatorName(build)

	if result != "Unknown" {
		t.Errorf("Expected 'Unknown', got: %s", result)
	}
}
