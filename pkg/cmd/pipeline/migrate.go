package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	bkio "github.com/buildkite/cli/v3/internal/io"
)

// MigrationEndpoint is the API endpoint for pipeline migration
// It can be overridden at build time using ldflags:
// -X github.com/buildkite/cli/v3/pkg/cmd/pipeline.MigrationEndpoint=https://example.com/migrate
var MigrationEndpoint = "https://m4vrh5pvtd.execute-api.us-east-1.amazonaws.com/production/migrate"

type migrationRequest struct {
	Vendor string `json:"vendor"`
	Code   string `json:"code"`
	AI     bool   `json:"ai,omitempty"`
}

type migrationResponse struct {
	JobID     string `json:"jobId"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	StatusURL string `json:"statusUrl"`
}

type statusResponse struct {
	JobID       string `json:"jobId"`
	Status      string `json:"status"`
	Vendor      string `json:"vendor"`
	CreatedAt   string `json:"createdAt"`
	CompletedAt string `json:"completedAt,omitempty"`
	Result      string `json:"result,omitempty"`
	Error       string `json:"error,omitempty"`
}

func NewCmdPipelineMigrate() *cobra.Command {
	var filePath string
	var vendor string
	var useAI bool
	var outputPath string
	var timeoutSeconds int

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate a CI/CD pipeline configuration to Buildkite format",
		Long: heredoc.Doc(`
			Migrate a CI/CD pipeline configuration from various vendors to Buildkite format.

			Supported vendors:
			  - github (GitHub Actions)
			  - bitbucket (Bitbucket Pipelines)
			  - circleci (CircleCI)
			  - jenkins (Jenkins)

			The command will automatically detect the vendor based on the file name if not specified.

			By default, the migrated pipeline is saved to .buildkite/pipeline.<vendor>.yml.
			Use the --output flag to specify a custom output path.

			Note: This command does not require an API token since it uses a public migration API.
		`),
		Example: heredoc.Doc(`
			# Migrate a GitHub Actions workflow
			$ bk pipeline migrate -F .github/workflows/ci.yml

			# Migrate with explicit vendor specification
			$ bk pipeline migrate -F pipeline.yml --vendor circleci

			# Migrate Jenkins pipeline with AI support
			$ bk pipeline migrate -F Jenkinsfile --ai

			# Save output to a file
			$ bk pipeline migrate -F .github/workflows/ci.yml -o .buildkite/pipeline.yml
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("file path is required (use -F or --file)")
			}

			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("error reading file: %w", err)
			}

			// Detect vendor if not specified
			if vendor == "" {
				vendor, err = detectVendor(filePath)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Detected vendor: %s\n", vendor)
			}

			// Validate vendor
			supportedVendors := []string{"github", "bitbucket", "circleci", "jenkins"}
			if !contains(supportedVendors, vendor) {
				return fmt.Errorf("unsupported vendor: %s (supported: %s)", vendor, strings.Join(supportedVendors, ", "))
			}

			// Create migration request
			req := migrationRequest{
				Vendor: vendor,
				Code:   string(content),
				AI:     useAI,
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Submitting migration job...\n")

			// Submit migration job
			jobResp, err := submitMigrationJob(req)
			if err != nil {
				return fmt.Errorf("error submitting migration job: %w", err)
			}

			if useAI {
				fmt.Fprintf(cmd.OutOrStdout(), "Job submitted. Processing with AI (this may take several minutes)...\n")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Job submitted. Processing migration...\n")
			}

			// Poll for job completion with spinner
			var result *statusResponse
			err = bkio.SpinWhile("Processing migration...", func() {
				result, err = pollJobStatus(jobResp.JobID, timeoutSeconds)
			})
			if err != nil {
				return fmt.Errorf("error polling job status: %w", err)
			}

			if result.Status == "failed" {
				return fmt.Errorf("migration failed: %s", result.Error)
			}

			// Output result
			if outputPath != "" {
				// Save to specified file
				if err := os.WriteFile(outputPath, []byte(result.Result), 0644); err != nil {
					return fmt.Errorf("error writing output file: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n✅ Migration completed successfully!\n")
				fmt.Fprintf(cmd.OutOrStdout(), "Output saved to: %s\n", outputPath)
			} else {
				// Save to .buildkite directory with vendor-specific name
				buildkiteDir := ".buildkite"
				if err := os.MkdirAll(buildkiteDir, 0755); err != nil {
					return fmt.Errorf("error creating .buildkite directory: %w", err)
				}

				// Generate filename based on vendor
				outputFilename := fmt.Sprintf("pipeline.%s.yml", vendor)
				defaultOutputPath := filepath.Join(buildkiteDir, outputFilename)

				if err := os.WriteFile(defaultOutputPath, []byte(result.Result), 0644); err != nil {
					return fmt.Errorf("error writing output file: %w", err)
				}

				fmt.Fprintf(cmd.OutOrStdout(), "\n✅ Migration completed successfully!\n")
				fmt.Fprintf(cmd.OutOrStdout(), "Output saved to: %s\n", defaultOutputPath)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "F", "", "Path to the pipeline file to migrate (required)")
	cmd.Flags().StringVarP(&vendor, "vendor", "v", "", "CI/CD vendor (auto-detected if not specified)")
	cmd.Flags().BoolVar(&useAI, "ai", false, "Use AI-powered migration (recommended for Jenkins)")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Custom path to save the migrated pipeline (default: .buildkite/pipeline.<vendor>.yml)")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 300, "Timeout in seconds (use 600+ for AI migrations)")

	return cmd
}

// detectVendor attempts to detect the CI/CD vendor based on the file path
func detectVendor(filePath string) (string, error) {
	fileName := filepath.Base(filePath)

	if strings.Contains(filePath, ".github/workflows") || strings.Contains(filePath, ".github\\workflows") {
		return "github", nil
	}

	if fileName == "bitbucket-pipelines.yml" || fileName == "bitbucket-pipelines.yaml" {
		return "bitbucket", nil
	}

	if strings.Contains(filePath, ".circleci") {
		return "circleci", nil
	}

	if fileName == "Jenkinsfile" || strings.HasPrefix(fileName, "Jenkinsfile.") {
		return "jenkins", nil
	}

	return "", fmt.Errorf("could not detect vendor from file path. Please specify vendor explicitly with --vendor")
}

func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// submitMigrationJob submits a migration job to the API
func submitMigrationJob(req migrationRequest) (*migrationResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", MigrationEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var jobResp migrationResponse
	if err := json.Unmarshal(body, &jobResp); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return &jobResp, nil
}

// pollJobStatus polls the job status until completion
func pollJobStatus(jobID string, timeoutSeconds int) (*statusResponse, error) {
	statusURL := fmt.Sprintf("%s/%s/status", MigrationEndpoint, jobID)
	client := &http.Client{Timeout: 30 * time.Second}

	// Calculate max attempts based on timeout (poll every 5 seconds)
	maxAttempts := timeoutSeconds / 5
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for i := 0; i < maxAttempts; i++ {
		time.Sleep(5 * time.Second)

		req, err := http.NewRequestWithContext(context.Background(), "GET", statusURL, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating status request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error checking status: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("error reading status response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("status check failed (status %d): %s", resp.StatusCode, string(body))
		}

		var status statusResponse
		if err := json.Unmarshal(body, &status); err != nil {
			return nil, fmt.Errorf("error parsing status response: %w", err)
		}

		if status.Status == "completed" {
			return &status, nil
		}

		if status.Status == "failed" {
			return &status, nil
		}

		// Continue polling for "queued" or "processing" status
	}

	return nil, fmt.Errorf("migration timed out after %d seconds", timeoutSeconds)
}
