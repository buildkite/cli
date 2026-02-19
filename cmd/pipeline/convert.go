package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

var convertEndpoint = "https://m4vrh5pvtd.execute-api.us-east-1.amazonaws.com/production/migrate"

type conversionRequest struct {
	Vendor string `json:"vendor"`
	Code   string `json:"code"`
}

type conversionResponse struct {
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

type ConvertCmd struct {
	File    string `help:"Path to the pipeline file to convert (required)" short:"F" required:""`
	Vendor  string `help:"CI/CD vendor (auto-detected if not specified)" short:"v"`
	Output  string `help:"Custom path to save the converted pipeline (default: .buildkite/pipeline.<vendor>.yml)" short:"o"`
	Timeout int    `help:"Timeout in seconds for conversion" default:"300"`
}

func (c *ConvertCmd) Help() string {
	return `
Supported vendors:
  - github (GitHub Actions)
  - bitbucket (Bitbucket Pipelines)
  - circleci (CircleCI)
  - jenkins (Jenkins)
  - gitlab (GitLab CI) (beta)
  - harness (Harness CI) (beta)
  - bitrise (Bitrise) (beta)

The command will automatically detect the vendor based on the file name if not specified.

By default, the converted pipeline is saved to .buildkite/pipeline.<vendor>.yml.
Use the --output flag to specify a custom output path.

Note: This command does not require an API token since it uses a public conversion API.

Examples:
  # Convert a GitHub Actions workflow
  $ bk pipeline convert -F .github/workflows/ci.yml

  # Convert with explicit vendor specification
  $ bk pipeline convert -F pipeline.yml --vendor circleci

  # Save output to a file
  $ bk pipeline convert -F .github/workflows/ci.yml -o .buildkite/pipeline.yml
`
}

func (c *ConvertCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	content, err := os.ReadFile(c.File)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	if c.Vendor == "" {
		c.Vendor, err = detectVendor(c.File)
		if err != nil {
			return err
		}
		fmt.Printf("Detected vendor: %s\n", c.Vendor)
	}

	supportedVendors := []string{"github", "bitbucket", "circleci", "jenkins", "gitlab", "harness", "bitrise"}
	if !slices.Contains(supportedVendors, c.Vendor) {
		return fmt.Errorf("unsupported vendor: %s (supported: %s)", c.Vendor, strings.Join(supportedVendors, ", "))
	}

	if c.Timeout < 1 {
		return errors.New("a timeout cannot be less than 1 second")
	}

	req := conversionRequest{
		Vendor: c.Vendor,
		Code:   string(content),
	}

	fmt.Println("Submitting conversion job...")

	jobResp, err := submitConversionJob(req)
	if err != nil {
		return fmt.Errorf("error submitting conversion job: %w", err)
	}

	fmt.Println("Job submitted. Processing with AI (this may take several minutes)...")

	var result *statusResponse
	var pollErr error
	err = bkIO.SpinWhile(f, "Processing conversion...", func() {
		result, pollErr = pollJobStatus(jobResp.JobID, c.Timeout)
	})
	if err != nil {
		return fmt.Errorf("error polling job status: %w", err)
	}
	if pollErr != nil {
		return fmt.Errorf("error polling job status: %w", pollErr)
	}

	if result.Status == "failed" {
		return fmt.Errorf("conversion failed: %s", result.Error)
	}

	if c.Output != "" {
		if err := os.WriteFile(c.Output, []byte(result.Result), 0o644); err != nil {
			return fmt.Errorf("error writing output file: %w", err)
		}
		fmt.Printf("\n✅ conversion completed successfully!\n")
		fmt.Printf("Output saved to: %s\n", c.Output)
	} else {
		buildkiteDir := ".buildkite"
		if err := os.MkdirAll(buildkiteDir, 0o755); err != nil {
			return fmt.Errorf("error creating .buildkite directory: %w", err)
		}

		outputFilename := fmt.Sprintf("pipeline.%s.yml", c.Vendor)
		defaultOutputPath := filepath.Join(buildkiteDir, outputFilename)

		if err := os.WriteFile(defaultOutputPath, []byte(result.Result), 0o644); err != nil {
			return fmt.Errorf("error writing output file: %w", err)
		}

		fmt.Printf("\n✅ conversion completed successfully!\n")
		fmt.Printf("Output saved to: %s\n", defaultOutputPath)
	}

	return nil
}

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

func submitConversionJob(req conversionRequest) (*conversionResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", convertEndpoint, bytes.NewReader(reqBody))
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

	var jobResp conversionResponse
	if err := json.Unmarshal(body, &jobResp); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return &jobResp, nil
}

func pollJobStatus(jobID string, timeoutSeconds int) (*statusResponse, error) {
	statusURL := fmt.Sprintf("%s/%s/status", convertEndpoint, jobID)
	client := &http.Client{Timeout: 30 * time.Second}

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
	}

	return nil, fmt.Errorf("conversion timed out after %d seconds", timeoutSeconds)
}
