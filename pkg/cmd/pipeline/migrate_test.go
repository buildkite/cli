package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

func TestMigrationAPIEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a simple GitHub Actions workflow for testing
	testWorkflow := `name: Test
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - run: echo "Hello World"
`

	// Submit a migration job
	req := migrationRequest{
		Vendor: "github",
		Code:   testWorkflow,
		AI:     false,
	}

	jobResp, err := submitMigrationJob(req)
	if err != nil {
		t.Fatalf("Migration API endpoint is not accessible or broken. This will break the CLI for users. Error: %v", err)
	}

	if jobResp.JobID == "" {
		t.Error("Expected job ID to be returned")
	}

	if jobResp.Status != "processing" && jobResp.Status != "queued" {
		t.Errorf("Expected status to be 'processing' or 'queued', got: %s", jobResp.Status)
	}

	// Poll for completion with a reasonable timeout
	result, err := pollJobStatus(jobResp.JobID, 60) // 60 seconds timeout
	if err != nil {
		t.Fatalf("Failed to poll job status: %v", err)
	}

	if result.Status == "failed" {
		t.Errorf("Migration failed: %s", result.Error)
	}

	if result.Status != "completed" {
		t.Errorf("Expected status to be 'completed', got: %s", result.Status)
	}

	if result.Result == "" {
		t.Error("Expected result to contain migrated pipeline YAML")
	}

	// Verify the result is valid YAML
	if !strings.Contains(result.Result, "steps:") {
		t.Errorf("Expected result to contain 'steps:', got: %s", result.Result)
	}
}

func TestDetectVendor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filePath   string
		wantVendor string
		wantErr    bool
	}{
		{
			name:       "GitHub Actions workflow",
			filePath:   ".github/workflows/ci.yml",
			wantVendor: "github",
			wantErr:    false,
		},
		{
			name:       "GitHub Actions workflow (Windows path)",
			filePath:   ".github\\workflows\\ci.yml",
			wantVendor: "github",
			wantErr:    false,
		},
		{
			name:       "Bitbucket Pipelines",
			filePath:   "bitbucket-pipelines.yml",
			wantVendor: "bitbucket",
			wantErr:    false,
		},
		{
			name:       "Bitbucket Pipelines (yaml extension)",
			filePath:   "bitbucket-pipelines.yaml",
			wantVendor: "bitbucket",
			wantErr:    false,
		},
		{
			name:       "CircleCI config",
			filePath:   ".circleci/config.yml",
			wantVendor: "circleci",
			wantErr:    false,
		},
		{
			name:       "Jenkins file",
			filePath:   "Jenkinsfile",
			wantVendor: "jenkins",
			wantErr:    false,
		},
		{
			name:       "Jenkins file with extension",
			filePath:   "Jenkinsfile.production",
			wantVendor: "jenkins",
			wantErr:    false,
		},
		{
			name:     "Unknown file",
			filePath: "some-random-file.yml",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vendor, err := detectVendor(tt.filePath)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if vendor != tt.wantVendor {
				t.Errorf("Expected vendor %q, got %q", tt.wantVendor, vendor)
			}
		})
	}
}

func TestContains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		slice []string
		str   string
		want  bool
	}{
		{
			name:  "string present",
			slice: []string{"github", "bitbucket", "circleci"},
			str:   "github",
			want:  true,
		},
		{
			name:  "string not present",
			slice: []string{"github", "bitbucket", "circleci"},
			str:   "jenkins",
			want:  false,
		},
		{
			name:  "empty slice",
			slice: []string{},
			str:   "github",
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := contains(tt.slice, tt.str)
			if got != tt.want {
				t.Errorf("Expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestSubmitMigrationJob(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}

		var req migrationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req.Vendor == "" || req.Code == "" {
			t.Error("Expected vendor and code fields")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		resp := migrationResponse{
			JobID:     "test-job-123",
			Status:    "processing",
			Message:   "Migration job queued for processing",
			StatusURL: "https://example.com/migrate/test-job-123/status",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	originalEndpoint := MigrationEndpoint
	MigrationEndpoint = server.URL
	defer func() { MigrationEndpoint = originalEndpoint }()

	req := migrationRequest{
		Vendor: "github",
		Code:   "name: Test\non: [push]",
		AI:     false,
	}

	resp, err := submitMigrationJob(req)
	if err != nil {
		t.Fatalf("Failed to submit migration job: %v", err)
	}

	if resp.JobID != "test-job-123" {
		t.Errorf("Expected job ID 'test-job-123', got %q", resp.JobID)
	}

	if resp.Status != "processing" {
		t.Errorf("Expected status 'processing', got %q", resp.Status)
	}
}

func TestPollJobStatus(t *testing.T) {
	t.Parallel()

	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++

		var status string
		var result string

		// First attempt returns "processing", second returns "completed"
		if attempt == 1 {
			status = "processing"
		} else {
			status = "completed"
			result = "steps:\n  - command: echo 'test'\n"
		}

		resp := statusResponse{
			JobID:       "test-job-123",
			Status:      status,
			Vendor:      "github",
			CreatedAt:   time.Now().Format(time.RFC3339),
			CompletedAt: time.Now().Format(time.RFC3339),
			Result:      result,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	originalEndpoint := MigrationEndpoint
	MigrationEndpoint = server.URL
	defer func() { MigrationEndpoint = originalEndpoint }()

	result, err := pollJobStatus("test-job-123", 30)
	if err != nil {
		t.Fatalf("Failed to poll job status: %v", err)
	}

	if result.Status != "completed" {
		t.Errorf("Expected status 'completed', got %q", result.Status)
	}

	if result.Result == "" {
		t.Error("Expected result to be populated")
	}
}

func TestPollJobStatusTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := statusResponse{
			JobID:     "test-job-123",
			Status:    "processing",
			Vendor:    "github",
			CreatedAt: time.Now().Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	originalEndpoint := MigrationEndpoint
	MigrationEndpoint = server.URL
	defer func() { MigrationEndpoint = originalEndpoint }()

	_, err := pollJobStatus("test-job-123", 5) // 5 seconds = 1 attempt
	if err == nil {
		t.Error("Expected timeout error but got none")
	}

	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestMigrateCommandIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, ".github", "workflows", "test.yml")

	if err := os.MkdirAll(filepath.Dir(testFile), 0o755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	testWorkflow := `name: Test
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - run: echo "Test"
`
	if err := os.WriteFile(testFile, []byte(testWorkflow), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	cmd := NewCmdPipelineMigrate(&factory.Factory{})
	if cmd == nil {
		t.Fatal("Failed to create migrate command")
	}

	fileFlag := cmd.Flag("file")
	if fileFlag == nil {
		t.Error("Expected 'file' flag to exist")
	}

	vendorFlag := cmd.Flag("vendor")
	if vendorFlag == nil {
		t.Error("Expected 'vendor' flag to exist")
	}

	aiFlag := cmd.Flag("ai")
	if aiFlag == nil {
		t.Error("Expected 'ai' flag to exist")
	}

	outputFlag := cmd.Flag("output")
	if outputFlag == nil {
		t.Error("Expected 'output' flag to exist")
	}

	timeoutFlag := cmd.Flag("timeout")
	if timeoutFlag == nil {
		t.Error("Expected 'timeout' flag to exist")
	} else if timeoutFlag.DefValue != "300" {
		t.Errorf("Expected default timeout of 300, got %s", timeoutFlag.DefValue)
	}
}
