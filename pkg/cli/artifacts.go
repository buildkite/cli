package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// Artifacts commands
type ArtifactsCmd struct {
	List     ArtifactsListCmd     `cmd:"" help:"List build artifacts"`
	Download ArtifactsDownloadCmd `cmd:"" help:"Download artifacts"`
}

type ArtifactsListCmd struct {
	OutputFlag `embed:""`
	Build      string `arg:"" help:"Build number (pipeline auto-detected from git repo) or build URL"`
	Job        string `help:"Job UUID to filter artifacts"`
	State      string `help:"Artifact state to filter"`
}

func (a *ArtifactsListCmd) Help() string {
	return `EXAMPLES:
  # List all artifacts from build #42
  bk artifacts list 42

  # List artifacts from a specific job
  bk artifacts list 42 --job 01234567-89ab-cdef-0123-456789abcdef

  # List artifacts with a specific state
  bk artifacts list 42 --state finished`
}

type ArtifactsDownloadCmd struct {
	OutputFlag  `embed:""`
	Build       string `arg:"" help:"Build number (pipeline auto-detected from git repo) or build URL"`
	Destination string `help:"Download destination directory"`
	Job         string `help:"Job UUID to filter artifacts"`
	Pattern     string `help:"Artifact name pattern"`
}

func (a *ArtifactsDownloadCmd) Help() string {
	return `EXAMPLES:
  # Download all artifacts from build #42
  bk artifacts download 42

  # Download artifacts to a specific directory
  bk artifacts download 42 --destination ./artifacts

  # Download artifacts from a specific job
  bk artifacts download 42 --job 01234567-89ab-cdef-0123-456789abcdef

  # Download artifacts matching a pattern
  bk artifacts download 42 --pattern "*.xml"`
}

// Artifacts command implementations
func (a *ArtifactsListCmd) Run(ctx context.Context, f *factory.Factory) error {
	a.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Parse build URL or UUID to extract org, pipeline, and build number
	org, pipeline, buildNumber, err := parseBuildIdentifier(a.Build, f.Config.OrganizationSlug())
	if err != nil {
		return fmt.Errorf("error parsing build identifier: %w", err)
	}

	// Convert build number to int
	buildNum, parseErr := strconv.Atoi(buildNumber)
	if parseErr != nil {
		return fmt.Errorf("invalid build number '%s': %w", buildNumber, parseErr)
	}

	// List artifacts for the build
	var artifacts []buildkite.Artifact
	spinErr := bk_io.SpinWhile("Loading build artifacts", func() {
		artifacts, _, err = f.RestAPIClient.Artifacts.ListByBuild(ctx, org, pipeline, fmt.Sprint(buildNum), nil)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error listing artifacts: %w", err)
	}

	if len(artifacts) == 0 {
		fmt.Println("No artifacts found for this build")
		return nil
	}

	// Filter artifacts if job or state specified
	filteredArtifacts := artifacts
	if a.Job != "" || a.State != "" {
		filteredArtifacts = []buildkite.Artifact{}
		for _, artifact := range artifacts {
			include := true
			if a.Job != "" && artifact.JobID != a.Job {
				include = false
			}
			if a.State != "" && artifact.State != a.State {
				include = false
			}
			if include {
				filteredArtifacts = append(filteredArtifacts, artifact)
			}
		}
	}

	if len(filteredArtifacts) == 0 {
		if ShouldUseStructuredOutput(f) {
			return Print([]any{}, f)
		}
		fmt.Println("No artifacts match the specified filters")
		return nil
	}

	// Output artifacts in requested format
	if ShouldUseStructuredOutput(f) {
		return Print(filteredArtifacts, f)
	}

	// Display artifacts in tabular format
	fmt.Printf("%-40s %-10s %-15s %s\n", "Filename", "Size", "State", "Job ID")
	fmt.Println(strings.Repeat("-", 80))
	for _, artifact := range filteredArtifacts {
		filename := artifact.Filename
		if len(filename) > 40 {
			filename = filename[:37] + "..."
		}

		size := "Unknown"
		if artifact.FileSize > 0 {
			size = fmt.Sprintf("%.1fKB", float64(artifact.FileSize)/1024)
		}

		jobID := artifact.JobID
		if len(jobID) > 15 {
			jobID = jobID[:12] + "..."
		}

		fmt.Printf("%-40s %-10s %-15s %s\n",
			filename,
			size,
			artifact.State,
			jobID)
	}

	fmt.Printf("\nTotal: %d artifacts\n", len(filteredArtifacts))
	return nil
}

func (a *ArtifactsDownloadCmd) Run(ctx context.Context, f *factory.Factory) error {
	a.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Parse build URL or UUID to extract org, pipeline, and build number
	org, pipeline, buildNumber, err := parseBuildIdentifier(a.Build, f.Config.OrganizationSlug())
	if err != nil {
		return fmt.Errorf("error parsing build identifier: %w", err)
	}

	// Convert build number to int
	buildNum, parseErr := strconv.Atoi(buildNumber)
	if parseErr != nil {
		return fmt.Errorf("invalid build number '%s': %w", buildNumber, parseErr)
	}

	// Set default destination
	destination := a.Destination
	if destination == "" {
		destination = "artifacts"
	}

	// List artifacts for the build
	var artifacts []buildkite.Artifact
	spinErr := bk_io.SpinWhile("Loading build artifacts", func() {
		artifacts, _, err = f.RestAPIClient.Artifacts.ListByBuild(ctx, org, pipeline, fmt.Sprint(buildNum), nil)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error listing artifacts: %w", err)
	}

	if len(artifacts) == 0 {
		fmt.Println("No artifacts found for this build")
		return nil
	}

	// Filter artifacts if job or pattern specified
	filteredArtifacts := artifacts
	if a.Job != "" || a.Pattern != "" {
		filteredArtifacts = []buildkite.Artifact{}
		for _, artifact := range artifacts {
			include := true
			if a.Job != "" && artifact.JobID != a.Job {
				include = false
			}
			if a.Pattern != "" && !strings.Contains(artifact.Filename, a.Pattern) {
				include = false
			}
			if include {
				filteredArtifacts = append(filteredArtifacts, artifact)
			}
		}
	}

	if len(filteredArtifacts) == 0 {
		fmt.Println("No artifacts match the specified filters")
		return nil
	}

	// Create destination directory
	if err := os.MkdirAll(destination, 0755); err != nil {
		return fmt.Errorf("error creating destination directory: %w", err)
	}

	fmt.Printf("Downloading %d artifacts to %s/\n", len(filteredArtifacts), destination)

	// Download each artifact
	downloaded := 0
	for i, artifact := range filteredArtifacts {
		filename := filepath.Join(destination, artifact.Filename)

		// Create directory structure if needed
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			fmt.Printf("Warning: failed to create directory for %s: %v\n", filename, err)
			continue
		}

		fmt.Printf("[%d/%d] %s\n", i+1, len(filteredArtifacts), artifact.Filename)

		// Download the artifact
		err = nil
		spinErr := bk_io.SpinWhile("", func() {
			file, createErr := os.Create(filename)
			if createErr != nil {
				err = createErr
				return
			}
			defer file.Close()
			_, err = f.RestAPIClient.Artifacts.DownloadArtifactByURL(ctx, artifact.DownloadURL, file)
		})
		if spinErr != nil {
			fmt.Printf("Warning: failed to download %s: %v\n", artifact.Filename, spinErr)
			continue
		}
		if err != nil {
			fmt.Printf("Warning: failed to download %s: %v\n", artifact.Filename, err)
			continue
		}
		downloaded++
	}

	fmt.Printf("✓ Successfully downloaded %d/%d artifacts to %s/\n", downloaded, len(filteredArtifacts), destination)
	return nil
}
