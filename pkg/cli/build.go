package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/pkg/browser"
	"gopkg.in/yaml.v3"
)

// Build commands
type BuildCmd struct {
	Cancel   BuildCancelCmd   `cmd:"" help:"Cancel a build"`
	Download BuildDownloadCmd `cmd:"" help:"Download build artifacts or job logs"`
	New      BuildNewCmd      `cmd:"" help:"Create a build"`
	Rebuild  BuildRebuildCmd  `cmd:"" help:"Rebuild a build"`
	View     BuildViewCmd     `cmd:"" help:"View a build"`
	Watch    BuildWatchCmd    `cmd:"" help:"Watch build status until completion"`
}

type BuildCancelCmd struct {
	Build string `arg:"" help:"Build number (pipeline auto-detected from git repo) or build URL"`
}

type BuildDownloadCmd struct {
	Build       string `arg:"" help:"Build number (pipeline auto-detected from git repo) or build URL"`
	Destination string `help:"Download destination directory"`
	Job         string `help:"Job ID to filter artifacts"`
	Pattern     string `help:"Artifact name pattern"`
	Logs        bool   `help:"Download job logs instead of artifacts"`
	FailedOnly  bool   `help:"Only download logs from failed jobs (requires --logs)" name:"failed-only"`
}

func (b *BuildDownloadCmd) Help() string {
	return `EXAMPLES:
  # Download all artifacts from build #42
  bk build download 42

  # Download artifacts with pattern matching
  bk build download 42 --pattern "*.xml" --destination ./reports

  # Download logs from failed jobs only
  bk build download 42 --logs --failed-only --destination ./debug-logs

  # Download all job logs
  bk build download 42 --logs --destination ./all-logs`
}

type BuildNewCmd struct {
	OutputFlag          `embed:""`
	Pipeline            string            `arg:"" optional:"" help:"Pipeline slug (e.g., 'my-pipeline' or 'org/my-pipeline')"`
	Branch              string            `help:"Branch to build (if omitted, uses pipeline's default branch)" placeholder:"main"`
	Commit              string            `help:"Commit SHA to build (full SHA or short SHA)" default:"HEAD"`
	Message             string            `help:"Custom build message (if omitted, uses commit message)" placeholder:"Deploy to production"`
	Env                 map[string]string `help:"Environment variables in KEY=VALUE format" placeholder:"NODE_ENV=production"`
	EnvFile             string            `help:"Environment file path (.env format)" name:"env-file" type:"existingfile" placeholder:".env"`
	MetaData            map[string]string `help:"Build metadata in KEY=VALUE format" placeholder:"deploy-target=production"`
	NoWait              bool              `help:"Don't wait for build to start (exit immediately after creation)"`
	JSON                bool              `help:"Output build details as JSON instead of human-readable format (deprecated: use --output json)"`
	Web                 bool              `help:"Open build in web browser after creation"`
	Yes                 bool              `help:"Skip confirmation prompt (useful for automation)"`
	IgnoreBranchFilters bool              `help:"Ignore branch filters configured for the pipeline" name:"ignore-branch-filters"`
	PipelineFlag        string            `help:"Pipeline slug (alternative to positional argument)" name:"pipeline" placeholder:"my-pipeline"`
}

func (b *BuildNewCmd) Help() string {
	return `EXAMPLES:
  # Trigger build on current branch (waits for build to start)
  bk build new my-pipeline

  # Trigger build on specific commit with metadata
  bk build new my-pipeline --commit 9fceb02 --meta-data deploy-target=staging

  # Trigger build and exit immediately without waiting
  bk build new my-pipeline --no-wait

  # Get JSON output for automation
  bk build new my-pipeline --json`
}

type BuildRebuildCmd struct {
	Build string `arg:"" help:"Build number (pipeline auto-detected from git repo) or build URL"`
}

type BuildViewCmd struct {
	OutputFlag `embed:""`
	Build      string `arg:"" optional:"" help:"Build number (pipeline auto-detected from git repo) or build URL. If omitted, shows the most recent build on current branch."`
	Web        bool   `help:"Open build in web browser instead of displaying details"`
	Mine       bool   `help:"Filter to builds created by current user only"`
	Branch     string `help:"Filter to builds on specific branch" placeholder:"main"`
	User       string `help:"Filter to builds by specific user (name or email)" placeholder:"user@example.com"`
	Pipeline   string `help:"Pipeline to search. Format: 'pipeline-slug' or 'org/pipeline-slug' (if omitted, auto-detected from git repo)." placeholder:"my-pipeline"`
}

type BuildWatchCmd struct {
	OutputFlag `embed:""`
	Build      string `arg:"" help:"Build number (pipeline auto-detected from git repo) or build URL"`
	Interval   int    `help:"Polling interval in seconds"`
}

func (b *BuildWatchCmd) Help() string {
	return `EXAMPLES:
  # Watch build status with live progress
  bk build watch 42

  # Watch build silently and get final result as JSON
  bk build watch 42 --output json`
}

// Build command implementations
func (b *BuildCancelCmd) Run(ctx context.Context, f *factory.Factory) error {
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Parse build URL or UUID to extract org, pipeline, and build number
	org, pipeline, buildNumber, err := parseBuildIdentifier(b.Build, f.Config.OrganizationSlug())
	if err != nil {
		return fmt.Errorf("error parsing build identifier: %w", err)
	}

	// Convert build number to int
	buildNum, parseErr := strconv.Atoi(buildNumber)
	if parseErr != nil {
		return fmt.Errorf("invalid build number '%s': %w", buildNumber, parseErr)
	}

	// Cancel the build
	var build buildkite.Build
	spinErr := bk_io.SpinWhile(fmt.Sprintf("Canceling build #%d", buildNum), func() {
		build, err = f.RestAPIClient.Builds.Cancel(ctx, org, pipeline, fmt.Sprint(buildNum))
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error canceling build: %w", err)
	}

	fmt.Printf("Build #%d canceled successfully\n", build.Number)
	return nil
}

func (b *BuildDownloadCmd) Run(ctx context.Context, f *factory.Factory) error {
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Parse build URL or UUID to extract org, pipeline, and build number
	org, pipeline, buildNumber, err := parseBuildIdentifier(b.Build, f.Config.OrganizationSlug())
	if err != nil {
		return fmt.Errorf("error parsing build identifier: %w", err)
	}

	// Convert build number to int
	buildNum, parseErr := strconv.Atoi(buildNumber)
	if parseErr != nil {
		return fmt.Errorf("invalid build number '%s': %w", buildNumber, parseErr)
	}

	// Set default destination
	destination := b.Destination
	if destination == "" {
		destination = fmt.Sprintf("build-%d-artifacts", buildNum)
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
	if b.Job != "" || b.Pattern != "" {
		filteredArtifacts = []buildkite.Artifact{}
		for _, artifact := range artifacts {
			// Simple pattern matching - could be enhanced
			include := true
			if b.Job != "" && artifact.JobID != b.Job {
				include = false
			}
			if b.Pattern != "" && !strings.Contains(artifact.Filename, b.Pattern) {
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
	for i, artifact := range filteredArtifacts {
		filename := filepath.Join(destination, artifact.Filename)

		// Create directory structure if needed
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			fmt.Printf("Warning: failed to create directory for %s: %v\n", filename, err)
			continue
		}

		fmt.Printf("[%d/%d] Downloading %s...\n", i+1, len(filteredArtifacts), artifact.Filename)

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
	}

	fmt.Printf("✓ Downloaded %d artifacts to %s/\n", len(filteredArtifacts), destination)
	return nil
}

func (b *BuildNewCmd) Run(ctx context.Context, f *factory.Factory) error {
	b.Apply(f)
	// Handle legacy --json flag (backward compatibility)
	if b.JSON {
		fmt.Fprintf(os.Stderr, "Warning: --json flag is deprecated, use --output json instead\n")
		f.Output = "json"
	}

	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Resolve pipeline - prioritize flag over positional argument
	org := f.Config.OrganizationSlug()
	pipeline := b.Pipeline
	if b.PipelineFlag != "" {
		pipeline = b.PipelineFlag
	}

	// If still no pipeline, try to resolve from current directory or config
	if pipeline == "" {
		// This could be enhanced with pipeline resolution logic
		return fmt.Errorf("pipeline is required - specify with positional argument or --pipeline flag")
	}

	// Process environment variables from file if specified
	envMap := make(map[string]string)
	if b.Env != nil {
		for k, v := range b.Env {
			envMap[k] = v
		}
	}

	if b.EnvFile != "" {
		file, err := os.Open(b.EnvFile)
		if err != nil {
			return fmt.Errorf("could not open environment file '%s': %w", b.EnvFile, err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
					envMap[parts[0]] = parts[1]
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading environment file: %w", err)
		}
	}

	// Get default branch if not specified
	branch := b.Branch
	if branch == "" {
		var p buildkite.Pipeline
		var err error
		spinErr := bk_io.SpinWhile("Fetching pipeline information", func() {
			p, _, err = f.RestAPIClient.Pipelines.Get(ctx, org, pipeline)
		})
		if spinErr != nil {
			return spinErr
		}
		if err != nil {
			return fmt.Errorf("error fetching pipeline information: %w", err)
		}
		if p.DefaultBranch != "" {
			branch = p.DefaultBranch
		}
	}

	// Confirmation prompt (unless --yes flag is used)
	if !b.Yes {
		confirmed := false
		err := bk_io.Confirm(&confirmed, fmt.Sprintf("Create new build on %s?", pipeline))
		if err != nil {
			return fmt.Errorf("confirmation canceled: %w", err)
		}
		if !confirmed {
			fmt.Println("Build creation canceled")
			return nil
		}
	}

	// Prepare build request
	createBuild := buildkite.CreateBuild{
		Message:                     b.Message,
		Commit:                      b.Commit,
		Branch:                      branch,
		Env:                         envMap,
		MetaData:                    b.MetaData,
		IgnorePipelineBranchFilters: b.IgnoreBranchFilters,
	}

	if b.NoWait {
		fmt.Println("Creating build without waiting...")
	}

	// Create the build
	var err error
	var build buildkite.Build
	spinErr := bk_io.SpinWhile(fmt.Sprintf("Creating build for %s", pipeline), func() {
		build, _, err = f.RestAPIClient.Builds.Create(ctx, org, pipeline, createBuild)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error creating build: %w", err)
	}

	// Output build information in requested format
	if ShouldUseStructuredOutput(f) {
		if err := Print(build, f); err != nil {
			return fmt.Errorf("error formatting build output: %w", err)
		}
	} else {
		fmt.Printf("Build created: %s\n", build.WebURL)
	}

	// Open in web browser if --web flag is used
	if b.Web {
		if build.WebURL != "" {
			fmt.Printf("Opening %s in your browser\n", build.WebURL)
			err := browser.OpenURL(build.WebURL)
			if err != nil {
				return fmt.Errorf("failed to open web browser: %w", err)
			}
		}
	}

	return nil
}

func (b *BuildRebuildCmd) Run(ctx context.Context, f *factory.Factory) error {
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Parse build URL or UUID to extract org, pipeline, and build number
	org, pipeline, buildNumber, err := parseBuildIdentifier(b.Build, f.Config.OrganizationSlug())
	if err != nil {
		return fmt.Errorf("error parsing build identifier: %w", err)
	}

	// Convert build number to int
	buildNum, parseErr := strconv.Atoi(buildNumber)
	if parseErr != nil {
		return fmt.Errorf("invalid build number '%s': %w", buildNumber, parseErr)
	}

	// Rebuild the build
	var newBuild buildkite.Build
	spinErr := bk_io.SpinWhile(fmt.Sprintf("Rebuilding build #%d", buildNum), func() {
		newBuild, err = f.RestAPIClient.Builds.Rebuild(ctx, org, pipeline, fmt.Sprint(buildNum))
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error rebuilding build: %w", err)
	}

	fmt.Printf("Build #%d rebuilt successfully as build #%d\n", buildNum, newBuild.Number)
	if newBuild.WebURL != "" {
		fmt.Printf("View new build: %s\n", newBuild.WebURL)
	}

	return nil
}

func (b *BuildViewCmd) Run(ctx context.Context, f *factory.Factory) error {
	b.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Use global output format, defaulting to "text" for backwards compatibility
	format := f.Output
	if format == "table" || format == "raw" {
		format = "text" // Map table/raw to text for backwards compatibility
	}
	switch format {
	case "json", "yaml", "text", "table", "raw":
		// Valid formats
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}

	// Validate mutually exclusive flags
	if b.Mine && b.User != "" {
		return fmt.Errorf("cannot specify both --mine and --user")
	}

	var build *buildkite.Build
	var err error

	// If build argument is provided, resolve it directly
	if b.Build != "" {
		build, err = b.resolveBuildFromArgument(ctx, f)
		if err != nil {
			return err
		}
	} else {
		// Otherwise resolve the most recent build with filters
		build, err = b.resolveMostRecentBuild(ctx, f)
		if err != nil {
			return err
		}
	}

	if build == nil {
		fmt.Printf("No build found.\n")
		return nil
	}

	if b.Web {
		buildURL := fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d",
			f.Config.OrganizationSlug(), build.Pipeline.Slug, build.Number)
		fmt.Printf("Opening %s in your browser\n", buildURL)
		return browser.OpenURL(buildURL)
	}

	// Get additional build details including artifacts and annotations
	var artifacts []buildkite.Artifact
	var annotations []buildkite.Annotation
	var buildErr error

	spinErr := bk_io.SpinWhile("Loading build information", func() {
		// Get full build details
		*build, _, buildErr = f.RestAPIClient.Builds.Get(ctx,
			f.Config.OrganizationSlug(),
			build.Pipeline.Slug,
			fmt.Sprint(build.Number),
			nil)
		if buildErr != nil {
			return
		}

		// Get artifacts
		artifacts, _, err = f.RestAPIClient.Artifacts.ListByBuild(ctx,
			f.Config.OrganizationSlug(),
			build.Pipeline.Slug,
			fmt.Sprint(build.Number),
			nil)
		if err != nil {
			return
		}

		// Get annotations
		annotations, _, err = f.RestAPIClient.Annotations.ListByBuild(ctx,
			f.Config.OrganizationSlug(),
			build.Pipeline.Slug,
			fmt.Sprint(build.Number),
			nil)
	})
	if spinErr != nil {
		return spinErr
	}
	if buildErr != nil {
		return buildErr
	}
	if err != nil {
		return err
	}

	// Format and output the build information
	return b.outputBuild(build, artifacts, annotations, format)
}

func (b *BuildWatchCmd) Run(ctx context.Context, f *factory.Factory) error {
	b.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Parse build URL or UUID to extract org, pipeline, and build number
	org, pipeline, buildNumber, err := parseBuildIdentifier(b.Build, f.Config.OrganizationSlug())
	if err != nil {
		return fmt.Errorf("error parsing build identifier: %w", err)
	}

	// Convert build number to int
	buildNum, parseErr := strconv.Atoi(buildNumber)
	if parseErr != nil {
		return fmt.Errorf("invalid build number '%s': %w", buildNumber, parseErr)
	}

	// Set default interval if not specified
	interval := b.Interval
	if interval <= 0 {
		interval = 5 // Default to 5 seconds
	}

	if !ShouldUseStructuredOutput(f) {
		fmt.Printf("Watching build #%d (polling every %d seconds, press Ctrl+C to stop)\n", buildNum, interval)
	}

	var lastState string
	var lastLoggedJobs = make(map[string]string) // job ID -> last logged state

	for {
		// Get current build state
		var build buildkite.Build
		err = nil
		spinErr := bk_io.SpinWhile("", func() { // Empty message to avoid spam
			build, _, err = f.RestAPIClient.Builds.Get(ctx, org, pipeline, fmt.Sprint(buildNum), nil)
		})
		if spinErr != nil {
			return spinErr
		}
		if err != nil {
			if !ShouldUseStructuredOutput(f) {
				fmt.Printf("\nError getting build: %v\n", err)
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		// Check if build state changed
		if build.State != lastState {
			if !ShouldUseStructuredOutput(f) {
				fmt.Printf("\n[%s] Build state: %s\n",
					time.Now().Format("15:04:05"),
					build.State)
			}
			lastState = build.State
		}

		// Show job state changes
		for _, job := range build.Jobs {
			if job.Type == "script" {
				jobID := job.ID
				currentState := job.State
				if lastLoggedJobs[jobID] != currentState {
					if !ShouldUseStructuredOutput(f) {
						fmt.Printf("[%s] Job '%s': %s\n",
							time.Now().Format("15:04:05"),
							job.Name,
							currentState)
					}
					lastLoggedJobs[jobID] = currentState
				}
			}
		}

		// Check if build is finished
		if build.State == "passed" || build.State == "failed" || build.State == "canceled" {
			if ShouldUseStructuredOutput(f) {
				return Print(build, f)
			}
			fmt.Printf("\n✓ Build finished with state: %s\n", build.State)
			if build.WebURL != "" {
				fmt.Printf("View details: %s\n", build.WebURL)
			}
			break
		}

		// Wait before next poll
		time.Sleep(time.Duration(interval) * time.Second)
	}

	return nil
}

// resolveBuildFromArgument resolves a build when an explicit build argument is provided
func (b *BuildViewCmd) resolveBuildFromArgument(ctx context.Context, f *factory.Factory) (*buildkite.Build, error) {
	// Check if it's a build URL
	if strings.HasPrefix(b.Build, "http") {
		org, pipeline, buildNumber, err := parseBuildIdentifier(b.Build, f.Config.OrganizationSlug())
		if err != nil {
			return nil, fmt.Errorf("error parsing build identifier: %w", err)
		}
		buildNum, parseErr := strconv.Atoi(buildNumber)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid build number '%s': %w", buildNumber, parseErr)
		}
		build, _, err := f.RestAPIClient.Builds.Get(ctx, org, pipeline, fmt.Sprint(buildNum), nil)
		if err != nil {
			return nil, fmt.Errorf("error getting build: %w", err)
		}
		return &build, nil
	}

	// Check if it's a number (build number)
	if buildNum, err := strconv.Atoi(b.Build); err == nil {
		// Need to resolve pipeline first
		pipeline, err := b.resolvePipeline(ctx, f)
		if err != nil {
			return nil, err
		}
		if pipeline == "" {
			return nil, fmt.Errorf("failed to resolve pipeline - use --pipeline flag or run from a git repository")
		}

		build, _, err := f.RestAPIClient.Builds.Get(ctx, f.Config.OrganizationSlug(), pipeline, fmt.Sprint(buildNum), nil)
		if err != nil {
			return nil, fmt.Errorf("error getting build: %w", err)
		}
		return &build, nil
	}

	// Check if it's in org/pipeline/build format
	parts := strings.Split(b.Build, "/")
	if len(parts) == 3 {
		org, pipeline, buildNumber := parts[0], parts[1], parts[2]
		buildNum, parseErr := strconv.Atoi(buildNumber)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid build number '%s': %w", buildNumber, parseErr)
		}
		build, _, err := f.RestAPIClient.Builds.Get(ctx, org, pipeline, fmt.Sprint(buildNum), nil)
		if err != nil {
			return nil, fmt.Errorf("error getting build: %w", err)
		}
		return &build, nil
	}

	return nil, fmt.Errorf("unable to parse build argument: %s", b.Build)
}

// resolveMostRecentBuild resolves the most recent build with filters
func (b *BuildViewCmd) resolveMostRecentBuild(ctx context.Context, f *factory.Factory) (*buildkite.Build, error) {
	// Resolve pipeline first
	pipeline, err := b.resolvePipeline(ctx, f)
	if err != nil {
		return nil, err
	}
	if pipeline == "" {
		return nil, fmt.Errorf("failed to resolve pipeline - use --pipeline flag or run from a git repository")
	}

	// Build the options for listing builds
	opts := &buildkite.BuildsListOptions{
		ListOptions: buildkite.ListOptions{
			PerPage: 1, // We only want the most recent
		},
	}

	// Apply branch filter
	if b.Branch != "" {
		opts.Branch = append(opts.Branch, b.Branch)
	} else {
		// Try to get branch from git repo
		if branch := b.getBranchFromGit(f); branch != "" {
			opts.Branch = append(opts.Branch, branch)
		}
	}

	// Apply user filter
	if b.User != "" {
		opts.Creator = b.User
	} else if b.Mine {
		// Get current user
		user, _, err := f.RestAPIClient.User.CurrentUser(ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting current user: %w", err)
		}
		opts.Creator = user.ID
	}

	// List builds
	builds, _, err := f.RestAPIClient.Builds.ListByPipeline(ctx, f.Config.OrganizationSlug(), pipeline, opts)
	if err != nil {
		return nil, fmt.Errorf("error listing builds: %w", err)
	}

	if len(builds) == 0 {
		return nil, nil
	}

	return &builds[0], nil
}

// resolvePipeline resolves the pipeline to use
func (b *BuildViewCmd) resolvePipeline(ctx context.Context, f *factory.Factory) (string, error) {
	// If pipeline is explicitly provided, use it
	if b.Pipeline != "" {
		// Handle org/pipeline format
		if strings.Contains(b.Pipeline, "/") {
			parts := strings.Split(b.Pipeline, "/")
			if len(parts) == 2 {
				return parts[1], nil // Return just the pipeline part
			}
		}
		return b.Pipeline, nil
	}

	// Try to resolve from git repository
	if pipeline := b.getPipelineFromGit(f); pipeline != "" {
		return pipeline, nil
	}

	// Try to resolve from config
	preferredPipelines := f.Config.PreferredPipelines()
	if len(preferredPipelines) > 0 {
		return preferredPipelines[0].Name, nil
	}

	return "", nil
}

// getBranchFromGit gets the current branch from git
func (b *BuildViewCmd) getBranchFromGit(f *factory.Factory) string {
	if f.GitRepository == nil {
		return ""
	}
	head, err := f.GitRepository.Head()
	if err != nil {
		return ""
	}
	return head.Name().Short()
}

// getPipelineFromGit gets the pipeline from git repository
func (b *BuildViewCmd) getPipelineFromGit(f *factory.Factory) string {
	// This could be enhanced to parse .buildkite/pipeline.yml or similar
	// For now, just return empty string
	return ""
}

// outputBuild formats and outputs the build information
func (b *BuildViewCmd) outputBuild(build *buildkite.Build, artifacts []buildkite.Artifact, annotations []buildkite.Annotation, format string) error {
	switch format {
	case "json":
		buildJSON, err := json.MarshalIndent(build, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshaling build to JSON: %w", err)
		}
		fmt.Println(string(buildJSON))
	case "yaml":
		buildYAML, err := yaml.Marshal(build)
		if err != nil {
			return fmt.Errorf("error marshaling build to YAML: %w", err)
		}
		fmt.Print(string(buildYAML))
	case "text":
		// Use the same output format as the existing implementation
		fmt.Printf("Build #%d: %s\n", build.Number, build.Message)
		if build.Pipeline != nil {
			fmt.Printf("Pipeline: %s\n", build.Pipeline.Name)
		}
		fmt.Printf("State: %s\n", build.State)
		if build.Branch != "" {
			fmt.Printf("Branch: %s\n", build.Branch)
		}
		if build.Commit != "" {
			fmt.Printf("Commit: %s\n", build.Commit)
		}
		if build.Author.Name != "" {
			fmt.Printf("Author: %s\n", build.Author.Name)
		}
		if build.CreatedAt != nil {
			fmt.Printf("Created: %s\n", build.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		if build.WebURL != "" {
			fmt.Printf("URL: %s\n", build.WebURL)
		}
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
	return nil
}
