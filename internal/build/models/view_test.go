package models

import (
	"strings"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestNewBuildView(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2025, 05, 31, 10, 30, 0, 0, time.UTC)
	startedAt := time.Date(2025, 05, 31, 10, 31, 0, 0, time.UTC)
	finishedAt := time.Date(2025, 05, 31, 10, 35, 0, 0, time.UTC)

	// Create buildkite types
	build := &buildkite.Build{
		ID:         "build-123",
		GraphQLID:  "QnVpbGQtMTIz",
		URL:        "https://api.buildkite.com/v2/organizations/test-org/pipelines/test-pipeline/builds/42",
		WebURL:     "https://buildkite.com/test-org/test-pipeline/builds/42",
		Number:     42,
		State:      "passed",
		Blocked:    false,
		Message:    "Test build message",
		Commit:     "abc123def456",
		Branch:     "main",
		Source:     "api",
		CreatedAt:  &buildkite.Timestamp{Time: createdAt},
		StartedAt:  &buildkite.Timestamp{Time: startedAt},
		FinishedAt: &buildkite.Timestamp{Time: finishedAt},
		Creator: buildkite.Creator{
			ID:        "user-123",
			Name:      "Test User",
			Email:     "test@example.com",
			AvatarURL: "https://avatar.example.com/user.png",
			CreatedAt: &buildkite.Timestamp{Time: createdAt},
		},
		Author: buildkite.Author{
			Username: "testuser",
			Name:     "Test Author",
			Email:    "author@example.com",
		},
		Jobs: []buildkite.Job{
			{
				ID:         "job-123",
				GraphQLID:  "Sm9iLTEyMw==",
				Type:       "script",
				Name:       "Test Job",
				Label:      "Test",
				Command:    "echo 'hello world'",
				State:      "passed",
				WebURL:     "https://buildkite.com/test-org/test-pipeline/builds/42#job-123",
				CreatedAt:  &buildkite.Timestamp{Time: createdAt},
				StartedAt:  &buildkite.Timestamp{Time: startedAt},
				FinishedAt: &buildkite.Timestamp{Time: finishedAt},
				ExitStatus: func() *int { i := 0; return &i }(),
			},
		},
		Env: map[string]interface{}{
			"TEST_VAR": "test_value",
		},
		MetaData: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	artifacts := []buildkite.Artifact{
		{
			ID:           "artifact-123",
			JobID:        "job-123",
			URL:          "https://api.buildkite.com/v2/artifacts/artifact-123",
			DownloadURL:  "https://buildkite.com/artifacts/artifact-123/download",
			State:        "finished",
			Path:         "test-results.xml",
			Dirname:      ".",
			Filename:     "test-results.xml",
			MimeType:     "application/xml",
			FileSize:     1024,
			GlobPath:     "*.xml",
			OriginalPath: "test-results.xml",
			SHA1:         "abc123def456",
		},
	}

	annotations := []buildkite.Annotation{
		{
			ID:       "annotation-123",
			Context:  "default",
			Style:    "info",
			BodyHTML: "<p>Test annotation</p>",
		},
	}

	// Test conversion
	view := NewBuildView(build, artifacts, annotations)

	// Verify basic fields
	if view.ID != build.ID {
		t.Errorf("Expected ID %s, got %s", build.ID, view.ID)
	}
	if view.Number != build.Number {
		t.Errorf("Expected Number %d, got %d", build.Number, view.Number)
	}
	if view.State != build.State {
		t.Errorf("Expected State %s, got %s", build.State, view.State)
	}
	if view.Message != build.Message {
		t.Errorf("Expected Message %s, got %s", build.Message, view.Message)
	}

	// Verify timestamps
	if !view.CreatedAt.Equal(createdAt) {
		t.Errorf("Expected CreatedAt %v, got %v", createdAt, view.CreatedAt)
	}
	if view.StartedAt == nil || !view.StartedAt.Equal(startedAt) {
		t.Errorf("Expected StartedAt %v, got %v", &startedAt, view.StartedAt)
	}
	if view.FinishedAt == nil || !view.FinishedAt.Equal(finishedAt) {
		t.Errorf("Expected FinishedAt %v, got %v", &finishedAt, view.FinishedAt)
	}

	// Verify creator
	if view.Creator == nil {
		t.Error("Expected Creator to be non-nil")
	} else {
		if view.Creator.ID != build.Creator.ID {
			t.Errorf("Expected Creator ID %s, got %s", build.Creator.ID, view.Creator.ID)
		}
		if view.Creator.Name != build.Creator.Name {
			t.Errorf("Expected Creator Name %s, got %s", build.Creator.Name, view.Creator.Name)
		}
	}

	// Verify author
	if view.Author == nil {
		t.Error("Expected Author to be non-nil")
	} else {
		if view.Author.Username != build.Author.Username {
			t.Errorf("Expected Author Username %s, got %s", build.Author.Username, view.Author.Username)
		}
	}

	// Verify jobs
	if len(view.Jobs) != len(build.Jobs) {
		t.Errorf("Expected %d jobs, got %d", len(build.Jobs), len(view.Jobs))
	}
	if len(view.Jobs) > 0 {
		job := view.Jobs[0]
		originalJob := build.Jobs[0]
		if job.ID != originalJob.ID {
			t.Errorf("Expected Job ID %s, got %s", originalJob.ID, job.ID)
		}
		if job.ExitStatus == nil || *job.ExitStatus != *originalJob.ExitStatus {
			t.Errorf("Expected Job ExitStatus %v, got %v", originalJob.ExitStatus, job.ExitStatus)
		}
	}

	// Verify artifacts
	if len(view.Artifacts) != len(artifacts) {
		t.Errorf("Expected %d artifacts, got %d", len(artifacts), len(view.Artifacts))
	}
	if len(view.Artifacts) > 0 {
		artifact := view.Artifacts[0]
		originalArtifact := artifacts[0]
		if artifact.ID != originalArtifact.ID {
			t.Errorf("Expected Artifact ID %s, got %s", originalArtifact.ID, artifact.ID)
		}
		if artifact.FileSize != originalArtifact.FileSize {
			t.Errorf("Expected Artifact FileSize %d, got %d", originalArtifact.FileSize, artifact.FileSize)
		}
	}

	// Verify annotations
	if len(view.Annotations) != len(annotations) {
		t.Errorf("Expected %d annotations, got %d", len(annotations), len(view.Annotations))
	}
	if len(view.Annotations) > 0 {
		annotation := view.Annotations[0]
		originalAnnotation := annotations[0]
		if annotation.ID != originalAnnotation.ID {
			t.Errorf("Expected Annotation ID %s, got %s", originalAnnotation.ID, annotation.ID)
		}
	}

	// Verify environment variables
	if view.Env == nil {
		t.Error("Expected Env to be non-nil")
	} else {
		if view.Env["TEST_VAR"] != "test_value" {
			t.Errorf("Expected Env TEST_VAR to be 'test_value', got %v", view.Env["TEST_VAR"])
		}
	}

	// Verify metadata conversion
	if view.MetaData == nil {
		t.Error("Expected MetaData to be non-nil")
	} else {
		if view.MetaData["key1"] != "value1" {
			t.Errorf("Expected MetaData key1 to be 'value1', got %v", view.MetaData["key1"])
		}
	}
}

func TestBuildViewTextOutput(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2025, 05, 31, 10, 30, 0, 0, time.UTC)

	view := BuildView{
		ID:        "build-123",
		Number:    42,
		State:     "passed",
		Message:   "Test build message",
		Branch:    "main",
		Commit:    "abc123def456",
		Source:    "api",
		CreatedAt: createdAt,
		Creator: &CreatorView{
			ID:        "user-123",
			Name:      "Test User",
			Email:     "test@example.com",
			AvatarURL: "https://avatar.example.com/user.png",
			CreatedAt: createdAt,
		},
	}

	output := view.TextOutput()

	// Verify output contains expected content
	if !strings.Contains(output, "Build #42") {
		t.Errorf("Expected output to contain build number, got: %s", output)
	}
	if !strings.Contains(output, "Test build message") {
		t.Errorf("Expected output to contain build message, got: %s", output)
	}
	if !strings.Contains(output, "✓") {
		t.Errorf("Expected output to contain build status symbol, got: %s", output)
	}
	if !strings.Contains(output, "main") {
		t.Errorf("Expected output to contain branch name, got: %s", output)
	}
}

func TestNewBuildViewWithNilValues(t *testing.T) {
	t.Parallel()

	// Test with minimal build data
	build := &buildkite.Build{
		ID:        "build-123",
		Number:    1,
		State:     "running",
		Message:   "Minimal build",
		CreatedAt: &buildkite.Timestamp{Time: time.Now()},
	}

	view := NewBuildView(build, nil, nil)

	if view.ID != build.ID {
		t.Errorf("Expected ID %s, got %s", build.ID, view.ID)
	}
	if view.Number != build.Number {
		t.Errorf("Expected Number %d, got %d", build.Number, view.Number)
	}

	// Verify nil fields are handled properly
	if view.Creator != nil {
		t.Error("Expected Creator to be nil")
	}
	if view.Author != nil {
		t.Error("Expected Author to be nil")
	}
	if len(view.Jobs) != 0 {
		t.Errorf("Expected 0 jobs, got %d", len(view.Jobs))
	}
	if len(view.Artifacts) != 0 {
		t.Errorf("Expected 0 artifacts, got %d", len(view.Artifacts))
	}
	if len(view.Annotations) != 0 {
		t.Errorf("Expected 0 annotations, got %d", len(view.Annotations))
	}
}

func TestNewBuildViewWithAgent(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2025, 05, 31, 10, 30, 0, 0, time.UTC)

	build := &buildkite.Build{
		ID:        "build-123",
		Number:    42,
		State:     "passed",
		CreatedAt: &buildkite.Timestamp{Time: createdAt},
		Jobs: []buildkite.Job{
			{
				ID:        "job-123",
				Type:      "script",
				State:     "passed",
				CreatedAt: &buildkite.Timestamp{Time: createdAt},
				Agent: buildkite.Agent{
					ID:             "agent-123",
					Name:           "test-agent",
					ConnectedState: "connected",
					Hostname:       "test-host",
					IPAddress:      "192.168.1.1",
					UserAgent:      "buildkite-agent/3.0.0",
					CreatedAt:      &buildkite.Timestamp{Time: createdAt},
				},
			},
		},
	}

	view := NewBuildView(build, nil, nil)

	if len(view.Jobs) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(view.Jobs))
	}

	job := view.Jobs[0]
	if job.Agent == nil {
		t.Error("Expected Agent to be non-nil")
	} else {
		if job.Agent.ID != "agent-123" {
			t.Errorf("Expected Agent ID 'agent-123', got %s", job.Agent.ID)
		}
		if job.Agent.ConnectionState != "connected" {
			t.Errorf("Expected Agent ConnectionState 'connected', got %s", job.Agent.ConnectionState)
		}
	}
}
