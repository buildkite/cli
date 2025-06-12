package build

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/build/models"
	"gopkg.in/yaml.v3"
)

func TestBuildViewStructured_TextOutput(t *testing.T) {
	t.Parallel()

	// Create test data
	createdAt := time.Date(2025, 0o5, 31, 10, 30, 0, 0, time.UTC)
	startedAt := time.Date(2025, 0o5, 31, 10, 31, 0, 0, time.UTC)
	finishedAt := time.Date(2025, 0o5, 31, 10, 35, 0, 0, time.UTC)

	view := models.BuildView{
		ID:         "build-123",
		GraphQLID:  "QnVpbGQtMTIz",
		Number:     42,
		State:      "passed",
		Blocked:    false,
		Message:    "Test build message",
		Commit:     "abc123def456",
		Branch:     "main",
		Source:     "api",
		WebURL:     "https://buildkite.com/test-org/test-pipeline/builds/42",
		URL:        "https://api.buildkite.com/v2/organizations/test-org/pipelines/test-pipeline/builds/42",
		CreatedAt:  createdAt,
		StartedAt:  &startedAt,
		FinishedAt: &finishedAt,
		Creator: &models.CreatorView{
			ID:        "user-123",
			Name:      "Test User",
			Email:     "test@example.com",
			AvatarURL: "https://avatar.example.com/user.png",
			CreatedAt: createdAt,
		},
		Author: &models.AuthorView{
			Username: "testuser",
			Name:     "Test Author",
			Email:    "author@example.com",
		},
		Jobs: []models.JobView{
			{
				ID:         "job-123",
				GraphQLID:  "Sm9iLTEyMw==",
				Type:       "script",
				Name:       "Test Job",
				Label:      "Test",
				Command:    "echo 'hello world'",
				State:      "passed",
				WebURL:     "https://buildkite.com/test-org/test-pipeline/builds/42#job-123",
				CreatedAt:  createdAt,
				StartedAt:  &startedAt,
				FinishedAt: &finishedAt,
				ExitStatus: func() *int { i := 0; return &i }(),
			},
		},
		Artifacts: []models.ArtifactView{
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
		},
		Annotations: []models.AnnotationView{
			{
				ID:       "annotation-123",
				Context:  "default",
				Style:    "info",
				BodyHTML: "<p>Test annotation</p>",
			},
		},
	}

	// Test TextOutput method
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

func TestBuildViewStructured_JSONSerialization(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2025, 0o5, 31, 10, 30, 0, 0, time.UTC)

	view := models.BuildView{
		ID:        "build-123",
		GraphQLID: "QnVpbGQtMTIz",
		Number:    42,
		State:     "passed",
		Blocked:   false,
		Message:   "Test build message",
		Commit:    "abc123def456",
		Branch:    "main",
		Source:    "api",
		WebURL:    "https://buildkite.com/test-org/test-pipeline/builds/42",
		URL:       "https://api.buildkite.com/v2/organizations/test-org/pipelines/test-pipeline/builds/42",
		CreatedAt: createdAt,
		Creator: &models.CreatorView{
			ID:        "user-123",
			Name:      "Test User",
			Email:     "test@example.com",
			AvatarURL: "https://avatar.example.com/user.png",
			CreatedAt: createdAt,
		},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("Failed to marshal to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled models.BuildView
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal from JSON: %v", err)
	}

	// Verify fields
	if unmarshaled.ID != view.ID {
		t.Errorf("Expected ID %s, got %s", view.ID, unmarshaled.ID)
	}
	if unmarshaled.Number != view.Number {
		t.Errorf("Expected Number %d, got %d", view.Number, unmarshaled.Number)
	}
	if unmarshaled.State != view.State {
		t.Errorf("Expected State %s, got %s", view.State, unmarshaled.State)
	}
	if !unmarshaled.CreatedAt.Equal(view.CreatedAt) {
		t.Errorf("Expected CreatedAt %v, got %v", view.CreatedAt, unmarshaled.CreatedAt)
	}
	if unmarshaled.Creator == nil {
		t.Error("Expected Creator to be non-nil")
	} else {
		if unmarshaled.Creator.ID != view.Creator.ID {
			t.Errorf("Expected Creator ID %s, got %s", view.Creator.ID, unmarshaled.Creator.ID)
		}
		if unmarshaled.Creator.Name != view.Creator.Name {
			t.Errorf("Expected Creator Name %s, got %s", view.Creator.Name, unmarshaled.Creator.Name)
		}
	}
}

func TestBuildViewStructured_YAMLSerialization(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2025, 0o5, 31, 10, 30, 0, 0, time.UTC)

	view := models.BuildView{
		ID:        "build-123",
		GraphQLID: "QnVpbGQtMTIz",
		Number:    42,
		State:     "passed",
		Blocked:   false,
		Message:   "Test build message",
		Commit:    "abc123def456",
		Branch:    "main",
		Source:    "api",
		WebURL:    "https://buildkite.com/test-org/test-pipeline/builds/42",
		URL:       "https://api.buildkite.com/v2/organizations/test-org/pipelines/test-pipeline/builds/42",
		CreatedAt: createdAt,
		Jobs: []models.JobView{
			{
				ID:        "job-123",
				GraphQLID: "Sm9iLTEyMw==",
				Type:      "script",
				Name:      "Test Job",
				State:     "passed",
				CreatedAt: createdAt,
			},
		},
	}

	// Test YAML marshaling
	yamlData, err := yaml.Marshal(view)
	if err != nil {
		t.Fatalf("Failed to marshal to YAML: %v", err)
	}

	// Test YAML unmarshaling
	var unmarshaled models.BuildView
	err = yaml.Unmarshal(yamlData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal from YAML: %v", err)
	}

	// Verify fields
	if unmarshaled.ID != view.ID {
		t.Errorf("Expected ID %s, got %s", view.ID, unmarshaled.ID)
	}
	if unmarshaled.Number != view.Number {
		t.Errorf("Expected Number %d, got %d", view.Number, unmarshaled.Number)
	}
	if len(unmarshaled.Jobs) != len(view.Jobs) {
		t.Errorf("Expected %d jobs, got %d", len(view.Jobs), len(unmarshaled.Jobs))
	}
	if len(unmarshaled.Jobs) > 0 {
		if unmarshaled.Jobs[0].ID != view.Jobs[0].ID {
			t.Errorf("Expected Job ID %s, got %s", view.Jobs[0].ID, unmarshaled.Jobs[0].ID)
		}
	}
}

func TestBuildViewStructured_EmptyValues(t *testing.T) {
	t.Parallel()

	// Test with minimal data
	view := models.BuildView{
		ID:        "build-123",
		Number:    1,
		State:     "running",
		Message:   "Minimal build",
		Branch:    "main",
		CreatedAt: time.Now(),
	}

	// Test TextOutput with minimal data
	output := view.TextOutput()
	if !strings.Contains(output, "Build #1") {
		t.Errorf("Expected output to contain build number, got: %s", output)
	}

	// Test JSON serialization with minimal data
	jsonData, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("Failed to marshal minimal data to JSON: %v", err)
	}

	var unmarshaled models.BuildView
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal minimal data from JSON: %v", err)
	}

	if unmarshaled.ID != view.ID {
		t.Errorf("Expected ID %s, got %s", view.ID, unmarshaled.ID)
	}
}

func TestBuildViewStructured_NilPointers(t *testing.T) {
	t.Parallel()

	// Test with nil pointers
	view := models.BuildView{
		ID:          "build-123",
		Number:      1,
		State:       "running",
		Message:     "Build with nil pointers",
		Branch:      "main",
		CreatedAt:   time.Now(),
		ScheduledAt: nil,
		StartedAt:   nil,
		FinishedAt:  nil,
		Creator:     nil,
		Author:      nil,
		Jobs:        nil,
		Artifacts:   nil,
		Annotations: nil,
	}

	// Test TextOutput handles nil pointers gracefully
	output := view.TextOutput()
	if output == "" {
		t.Error("Expected non-empty output even with nil pointers")
	}

	// Test JSON serialization with nil pointers
	jsonData, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("Failed to marshal data with nil pointers to JSON: %v", err)
	}

	// Verify that nil fields are omitted or null in JSON
	jsonString := string(jsonData)
	if !strings.Contains(jsonString, `"id":"build-123"`) {
		t.Error("Expected ID field in JSON output")
	}
}

func TestJobView_ExitStatus(t *testing.T) {
	t.Parallel()

	// Test with exit status
	exitStatus := 0
	job := models.JobView{
		ID:         "job-123",
		Type:       "script",
		State:      "passed",
		CreatedAt:  time.Now(),
		ExitStatus: &exitStatus,
	}

	jsonData, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("Failed to marshal job to JSON: %v", err)
	}

	var unmarshaled models.JobView
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal job from JSON: %v", err)
	}

	if unmarshaled.ExitStatus == nil {
		t.Error("Expected ExitStatus to be non-nil")
	} else if *unmarshaled.ExitStatus != exitStatus {
		t.Errorf("Expected ExitStatus %d, got %d", exitStatus, *unmarshaled.ExitStatus)
	}
}

func TestArtifactView_FileSize(t *testing.T) {
	t.Parallel()

	artifact := models.ArtifactView{
		ID:       "artifact-123",
		Path:     "test.txt",
		FileSize: 1024,
		SHA1:     "abc123",
	}

	jsonData, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("Failed to marshal artifact to JSON: %v", err)
	}

	var unmarshaled models.ArtifactView
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal artifact from JSON: %v", err)
	}

	if unmarshaled.FileSize != artifact.FileSize {
		t.Errorf("Expected FileSize %d, got %d", artifact.FileSize, unmarshaled.FileSize)
	}
}

func TestBuildViewStructured_OutputFormats(t *testing.T) {
	t.Parallel()

	// Import the output package for testing
	// This test demonstrates how the JSON/YAML output would be used
	createdAt := time.Date(2025, 0o5, 31, 10, 30, 0, 0, time.UTC)

	view := models.BuildView{
		ID:        "build-123",
		GraphQLID: "QnVpbGQtMTIz",
		Number:    42,
		State:     "passed",
		Blocked:   false,
		Message:   "Integration test build",
		Commit:    "abc123def456",
		Branch:    "main",
		Source:    "api",
		WebURL:    "https://buildkite.com/test-org/test-pipeline/builds/42",
		URL:       "https://api.buildkite.com/v2/organizations/test-org/pipelines/test-pipeline/builds/42",
		CreatedAt: createdAt,
		Creator: &models.CreatorView{
			ID:        "user-123",
			Name:      "Test User",
			Email:     "test@example.com",
			AvatarURL: "https://avatar.example.com/user.png",
			CreatedAt: createdAt,
		},
		Jobs: []models.JobView{
			{
				ID:        "job-123",
				Type:      "script",
				Name:      "Test Job",
				State:     "passed",
				CreatedAt: createdAt,
			},
		},
	}

	// Test that the view implements the output.Formatter interface correctly
	textOutput := view.TextOutput()
	if textOutput == "" {
		t.Error("TextOutput should not be empty")
	}
	if !strings.Contains(textOutput, "Build #42") {
		t.Error("TextOutput should contain build number")
	}
	if !strings.Contains(textOutput, "Integration test build") {
		t.Error("TextOutput should contain build message")
	}

	// Test JSON output structure
	jsonData, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal to JSON: %v", err)
	}

	// Verify JSON contains expected fields
	jsonString := string(jsonData)
	expectedJSONFields := []string{
		`"id": "build-123"`,
		`"number": 42`,
		`"state": "passed"`,
		`"message": "Integration test build"`,
		`"branch": "main"`,
		`"creator"`,
		`"jobs"`,
	}

	for _, field := range expectedJSONFields {
		if !strings.Contains(jsonString, field) {
			t.Errorf("JSON output should contain %s, got: %s", field, jsonString)
		}
	}

	// Test YAML output structure
	yamlData, err := yaml.Marshal(view)
	if err != nil {
		t.Fatalf("Failed to marshal to YAML: %v", err)
	}

	yamlString := string(yamlData)
	expectedYAMLFields := []string{
		"id: build-123",
		"number: 42",
		"state: passed",
		"message: Integration test build",
		"branch: main",
		"creator:",
		"jobs:",
	}

	for _, field := range expectedYAMLFields {
		if !strings.Contains(yamlString, field) {
			t.Errorf("YAML output should contain %s, got: %s", field, yamlString)
		}
	}
}
