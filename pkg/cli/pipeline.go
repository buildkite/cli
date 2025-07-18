package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/ghodss/yaml"
	"github.com/pkg/browser"
	"github.com/xeipuuv/gojsonschema"
)

const (
	schemaURL = "https://raw.githubusercontent.com/buildkite/pipeline-schema/main/schema.json"
)

// fallbackSchema is a simplified schema used when the online schema cannot be accessed
var fallbackSchema = []byte(`{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"properties": {
		"steps": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"label": { "type": "string" },
					"command": { "type": "string" },
					"plugins": { "type": "object" },
					"agents": { "type": "object" },
					"env": { "type": "object" },
					"branches": { "type": ["string", "object"] },
					"if": { "type": "string" },
					"depends_on": { "type": ["string", "array"] }
				}
			}
		},
		"env": { "type": "object" },
		"agents": { "type": "object" }
	},
	"required": ["steps"]
}`)

// Pipeline commands
type PipelineCmd struct {
	Create   PipelineCreateCmd   `cmd:"" help:"Create a pipeline"`
	View     PipelineViewCmd     `cmd:"" help:"View pipeline details"`
	Validate PipelineValidateCmd `cmd:"" help:"Validate pipeline configuration"`
}

type PipelineCreateCmd struct {
	File                            string            `help:"Pipeline YAML file" predictor:"file"`
	Organization                    string            `help:"Organization slug (if omitted, uses configured organization)" placeholder:"my-org"`
	Name                            string            `help:"Pipeline name"`
	Description                     string            `help:"Pipeline description"`
	Repository                      string            `help:"Repository URL"`
	Branch                          string            `help:"Default branch"`
	Slug                            string            `help:"Custom pipeline slug"`
	Visibility                      string            `help:"Pipeline visibility (public or private)" default:"private" enum:"public,private"`
	Env                             map[string]string `help:"Environment variables (key=value)"`
	Tags                            []string          `help:"Tags to add to the pipeline"`
	BranchConfiguration             string            `help:"Branch filter pattern (e.g., 'main feature/*')"`
	SkipQueuedBranchBuilds          bool              `help:"Skip queued builds for the same branch"`
	SkipQueuedBranchBuildsFilter    string            `help:"Branch pattern for skipping queued builds"`
	CancelRunningBranchBuilds       bool              `help:"Cancel running builds on the same branch"`
	CancelRunningBranchBuildsFilter string            `help:"Branch pattern for canceling running builds"`
	Interactive                     bool              `short:"i" help:"Launch an interactive wizard to collect the required fields"`
}

func (p *PipelineCreateCmd) Help() string {
	return `EXAMPLES:
  # Interactive pipeline creation
  bk pipeline create --interactive

  # Create pipeline with upload step (reads .buildkite/pipeline.yml from repo)
  bk pipeline create --name "My Pipeline" --repository "https://github.com/myorg/myrepo" --branch main

  # Create pipeline with custom steps (instead of pipeline upload)
  bk pipeline create --file .buildkite/pipeline.yml --name "My Pipeline"

  # Create with environment variables and tags
  bk pipeline create --name "My Pipeline" --repository "https://github.com/myorg/myrepo" --env "NODE_ENV=production;API_KEY=secret" --tags "frontend,deploy"`
}

type PipelineViewCmd struct {
	OutputFlag `embed:""`
	Pipeline   string `arg:"" help:"Pipeline slug to view"`
	Web        bool   `short:"w" help:"Open in web browser"`
	JSON       bool   `help:"Output as JSON (deprecated: use --output json)"`
}

type PipelineValidateCmd struct {
	Files []string `short:"f" help:"Path to the pipeline YAML file(s) to validate" predictor:"file"`
	Stdin bool     `help:"Read pipeline configuration from stdin"`
}

func (p *PipelineValidateCmd) Help() string {
	return `EXAMPLES:
  # Validate default .buildkite/pipeline.yml
  bk pipeline validate

  # Validate specific file
  bk pipeline validate -f pipeline.yml

  # Validate multiple files for batch processing
  bk pipeline validate -f .buildkite/pipeline.yml -f .buildkite/deploy.yml

  # Validate pipeline from stdin
  cat generated-pipeline.yml | bk pipeline validate --stdin`
}

// Pipeline command implementations
func (p *PipelineCreateCmd) Run(ctx context.Context, f *factory.Factory) error {
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	var opts createOpts
	var err error

	if p.Interactive {
		opts, err = p.runInteractive(ctx, f)
		if err != nil {
			return err
		}
	} else {
		// Validate required fields for non-interactive mode
		if p.Name == "" {
			return fmt.Errorf("pipeline name is required (--name or -i)")
		}
		if p.Repository == "" {
			return fmt.Errorf("repository URL is required (--repository or -i)")
		}

		// Read pipeline file if provided
		var config string
		if p.File != "" {
			fileContent, err := os.ReadFile(p.File)
			if err != nil {
				return fmt.Errorf("error reading pipeline file: %w", err)
			}
			config = string(fileContent)
		}

		opts = createOpts{
			Name:                            p.Name,
			Description:                     p.Description,
			Repository:                      p.Repository,
			Branch:                          p.Branch,
			Slug:                            p.Slug,
			Visibility:                      p.Visibility,
			Env:                             p.Env,
			Tags:                            p.Tags,
			BranchConfiguration:             p.BranchConfiguration,
			SkipQueuedBranchBuilds:          p.SkipQueuedBranchBuilds,
			SkipQueuedBranchBuildsFilter:    p.SkipQueuedBranchBuildsFilter,
			CancelRunningBranchBuilds:       p.CancelRunningBranchBuilds,
			CancelRunningBranchBuildsFilter: p.CancelRunningBranchBuildsFilter,
			Configuration:                   config,
		}
	}

	return createPipeline(ctx, f, opts)
}

func (p *PipelineViewCmd) Run(ctx context.Context, f *factory.Factory) error {
	p.Apply(f)
	// Handle legacy --json flag (backward compatibility)
	if p.JSON {
		fmt.Fprintf(os.Stderr, "Warning: --json flag is deprecated, use --output json instead\n")
		f.Output = "json"
	}

	if err := validateConfig(f.Config); err != nil {
		return err
	}

	org := f.Config.OrganizationSlug()

	if p.Web {
		url := fmt.Sprintf("https://buildkite.com/organizations/%s/pipelines/%s", org, p.Pipeline)
		fmt.Printf("Opening %s in your browser\n", url)
		return browser.OpenURL(url)
	}

	// Get pipeline details
	var err error
	var pipeline buildkite.Pipeline
	spinErr := bk_io.SpinWhile("Loading pipeline", func() {
		pipeline, _, err = f.RestAPIClient.Pipelines.Get(ctx, org, p.Pipeline)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error getting pipeline: %w", err)
	}

	// Output pipeline information in requested format
	if ShouldUseStructuredOutput(f) {
		if err := Print(pipeline, f); err != nil {
			return fmt.Errorf("error formatting pipeline output: %w", err)
		}
	} else {
		// Display pipeline information
		fmt.Printf("Pipeline: %s\n", pipeline.Name)
		fmt.Printf("Slug: %s\n", pipeline.Slug)
		fmt.Printf("Repository: %s\n", pipeline.Repository)
		if pipeline.Description != "" {
			fmt.Printf("Description: %s\n", pipeline.Description)
		}
		fmt.Printf("URL: %s\n", pipeline.WebURL)
		fmt.Printf("Default Branch: %s\n", pipeline.DefaultBranch)
	}

	return nil
}

func (p *PipelineValidateCmd) Run(ctx context.Context, f *factory.Factory) error {
	// This doesn't require API authentication, just file validation

	// Handle stdin input
	if p.Stdin {
		// Read from stdin
		stdinData, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("error reading from stdin: %w", err)
		}

		fmt.Printf("Validating pipeline from stdin...\n\n")

		err = validatePipelineData(os.Stdout, stdinData, "<stdin>")
		if err != nil {
			return fmt.Errorf("pipeline validation failed")
		}

		fmt.Printf("\nPipeline from stdin passed validation successfully!\n")
		return nil
	}

	// If no file paths provided, find the default
	if len(p.Files) == 0 {
		defaultPath, err := findPipelineFile()
		if err != nil {
			return err
		}
		p.Files = []string{defaultPath}
	}

	// Track validation errors
	var validationErrors []string
	fileCount := len(p.Files)

	fmt.Printf("Validating %d pipeline file(s)...\n\n", fileCount)

	// Validate each file
	for _, filePath := range p.Files {
		err := validatePipeline(os.Stdout, filePath)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: %v", filePath, err))
			// Continue validating other files even if one fails
		}
	}

	if len(validationErrors) > 0 {
		errorCount := len(validationErrors)
		fmt.Printf("\n%d of %d file(s) failed validation.\n", errorCount, fileCount)
		return fmt.Errorf("pipeline validation failed")
	}

	fmt.Printf("\nAll pipeline files passed validation successfully!\n")
	return nil
}

// findPipelineFile attempts to locate a pipeline file in the default locations
func findPipelineFile() (string, error) {
	// Check for pipeline files in various standard locations
	// The order matches the Buildkite agent's lookup order
	paths := []string{
		"buildkite.yml",
		"buildkite.yaml",
		"buildkite.json",
		filepath.Join(".buildkite", "pipeline.yml"),
		filepath.Join(".buildkite", "pipeline.yaml"),
		filepath.Join(".buildkite", "pipeline.json"),
		filepath.Join("buildkite", "pipeline.yml"),
		filepath.Join("buildkite", "pipeline.yaml"),
		filepath.Join("buildkite", "pipeline.json"),
	}

	// Check each path
	for _, path := range paths {
		if fileExists(path) {
			return path, nil
		}
	}

	// If no file found, provide detailed error message
	return "", fmt.Errorf("could not find pipeline file in default locations. Please specify a file with --file or create one in a standard location:\n" +
		"  • .buildkite/pipeline.yml\n" +
		"  • .buildkite/pipeline.yaml\n" +
		"  • buildkite.yml\n" +
		"  • buildkite.yaml")
}

// fileExists checks if a file exists and is not a directory
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// validatePipeline validates the given pipeline file against the schema
func validatePipeline(w io.Writer, filePath string) error {
	// Read the pipeline file
	pipelineData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading pipeline file: %w", err)
	}

	return validatePipelineData(w, pipelineData, filePath)
}

// validatePipelineData validates the given pipeline data against the schema
func validatePipelineData(w io.Writer, pipelineData []byte, source string) error {
	// Trim whitespace to handle empty files more gracefully
	if len(strings.TrimSpace(string(pipelineData))) == 0 {
		fmt.Fprintf(w, "❌ Pipeline data is invalid: %s\n\n", source)
		fmt.Fprintf(w, "- Data is empty\n")
		return fmt.Errorf("empty pipeline data")
	}

	// Convert YAML to JSON for validation
	jsonData, err := yaml.YAMLToJSON(pipelineData)
	if err != nil {
		fmt.Fprintf(w, "❌ Pipeline data is invalid: %s\n\n", source)
		fmt.Fprintf(w, "- YAML parsing error: %s\n", err.Error())
		fmt.Fprintf(w, "  Hint: Check for syntax errors like improper indentation, missing quotes, or invalid characters.\n")
		return fmt.Errorf("invalid YAML format: %w", err)
	}

	// Load the schema and document
	schemaLoader := gojsonschema.NewReferenceLoader(schemaURL)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	// Try to validate against the online schema
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		// If online schema access fails, try the fallback schema
		fmt.Fprintf(w, "⚠️  Warning: Could not access online pipeline schema: %s\n", err.Error())
		fmt.Fprintf(w, "   Using simplified fallback schema for basic validation.\n\n")

		// Create a schema loader using the fallback schema
		fallbackLoader := gojsonschema.NewBytesLoader(fallbackSchema)
		result, err = gojsonschema.Validate(fallbackLoader, documentLoader)
		if err != nil {
			fmt.Fprintf(w, "❌ Pipeline data is invalid: %s\n\n", source)
			fmt.Fprintf(w, "- Schema validation error: %s\n", err.Error())
			return fmt.Errorf("schema validation error: %w", err)
		}
	}

	if result.Valid() {
		fmt.Fprintf(w, "✅ Pipeline data is valid: %s\n", source)
		return nil
	}

	// Return validation errors
	fmt.Fprintf(w, "❌ Pipeline data is invalid: %s\n\n", source)
	for _, err := range result.Errors() {
		// Format the error message for better readability
		message := formatValidationError(err)
		fmt.Fprintf(w, "- %s\n", message)
	}

	return fmt.Errorf("pipeline validation failed")
}

// formatValidationError formats a validation error for better readability
func formatValidationError(err gojsonschema.ResultError) string {
	field := err.Field()

	// For array items, make the error message more readable
	if strings.Contains(field, "[") && strings.Contains(field, "]") {
		parts := strings.Split(field, ".")
		for i, part := range parts {
			if strings.Contains(part, "[") {
				index := strings.TrimRight(strings.TrimLeft(part, "["), "]")
				name := strings.Split(part, "[")[0]
				parts[i] = fmt.Sprintf("%s item #%s", name, index)
			}
		}
		field = strings.Join(parts, " > ")
	} else if field != "" {
		field = strings.ReplaceAll(field, ".", " > ")
	}

	message := err.Description()

	// Format the message with the field highlighted
	if field != "" {
		message = fmt.Sprintf("%s: %s", field, message)
	}

	// Include details about what was received vs what was expected if available
	details := err.Details()

	// Add more context about expected values
	var contextParts []string
	if val, ok := details["field"]; ok && val != field {
		contextParts = append(contextParts, fmt.Sprintf("field: %v", val))
	}
	if val, ok := details["expected"]; ok {
		contextParts = append(contextParts, fmt.Sprintf("expected: %v", val))
	}
	if val, ok := details["actual"]; ok {
		contextParts = append(contextParts, fmt.Sprintf("got: %v", val))
	}

	// If we have context parts, add them to the message
	if len(contextParts) > 0 {
		message += fmt.Sprintf(" (%s)", strings.Join(contextParts, ", "))
	}

	// Add a helpful hint based on the error type
	switch err.Type() {
	case "required":
		message += "\n    Hint: This field is required but was not found in your pipeline."
	case "type_error":
		message += "\n    Hint: Check that you're using the correct data type for this field."
	case "enum":
		message += "\n    Hint: The value must be one of the allowed options."
	case "const":
		message += "\n    Hint: This field must have the specific required value."
	case "array_no_items":
		message += "\n    Hint: This array cannot be empty."
	}

	return message
}

// createOpts holds options for creating a pipeline
type createOpts struct {
	Name                            string
	Description                     string
	Repository                      string
	Branch                          string
	Slug                            string
	Visibility                      string
	Env                             map[string]string
	Tags                            []string
	BranchConfiguration             string
	SkipQueuedBranchBuilds          bool
	SkipQueuedBranchBuildsFilter    string
	CancelRunningBranchBuilds       bool
	CancelRunningBranchBuildsFilter string
	Configuration                   string
}

// getRepoURLS returns repository URLs from git remotes
func getRepoURLS(f *factory.Factory) []string {
	if f.GitRepository == nil {
		return []string{}
	}

	c, err := f.GitRepository.Config()
	if err != nil {
		return []string{}
	}

	if _, ok := c.Remotes["origin"]; !ok {
		return []string{}
	}
	return c.Remotes["origin"].URLs
}

// createPipeline creates a pipeline with the given options
func createPipeline(ctx context.Context, f *factory.Factory, opts createOpts) error {
	org := f.Config.OrganizationSlug()

	// Default configuration if not provided
	config := opts.Configuration
	if config == "" {
		config = "steps:\n  - label: \":pipeline:\"\n    command: buildkite-agent pipeline upload"
	}

	// Create pipeline
	createPipeline := buildkite.CreatePipeline{
		Name:          opts.Name,
		Description:   opts.Description,
		Repository:    opts.Repository,
		Configuration: config,
	}
	if opts.Branch != "" {
		createPipeline.DefaultBranch = opts.Branch
	}
	if opts.Visibility != "" {
		createPipeline.Visibility = opts.Visibility
	}
	if len(opts.Env) > 0 {
		createPipeline.Env = opts.Env
	}
	if len(opts.Tags) > 0 {
		createPipeline.Tags = opts.Tags
	}
	if opts.BranchConfiguration != "" {
		createPipeline.BranchConfiguration = opts.BranchConfiguration
	}
	if opts.SkipQueuedBranchBuilds {
		createPipeline.SkipQueuedBranchBuilds = opts.SkipQueuedBranchBuilds
	}
	if opts.SkipQueuedBranchBuildsFilter != "" {
		createPipeline.SkipQueuedBranchBuildsFilter = opts.SkipQueuedBranchBuildsFilter
	}
	if opts.CancelRunningBranchBuilds {
		createPipeline.CancelRunningBranchBuilds = opts.CancelRunningBranchBuilds
	}
	if opts.CancelRunningBranchBuildsFilter != "" {
		createPipeline.CancelRunningBranchBuildsFilter = opts.CancelRunningBranchBuildsFilter
	}

	var err error
	var pipeline buildkite.Pipeline
	spinErr := bk_io.SpinWhile(fmt.Sprintf("Creating pipeline %s", opts.Name), func() {
		pipeline, _, err = f.RestAPIClient.Pipelines.Create(ctx, org, createPipeline)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error creating pipeline: %w", err)
	}

	fmt.Printf("Pipeline created: %s\n", pipeline.WebURL)
	return nil
}

// runInteractive runs the interactive wizard to collect pipeline creation options
func (p *PipelineCreateCmd) runInteractive(ctx context.Context, f *factory.Factory) (createOpts, error) {
	opts := createOpts{
		// Pre-seed with any provided flags
		Name:                            p.Name,
		Description:                     p.Description,
		Repository:                      p.Repository,
		Branch:                          p.Branch,
		Slug:                            p.Slug,
		Visibility:                      p.Visibility,
		Env:                             p.Env,
		Tags:                            p.Tags,
		BranchConfiguration:             p.BranchConfiguration,
		SkipQueuedBranchBuilds:          p.SkipQueuedBranchBuilds,
		SkipQueuedBranchBuildsFilter:    p.SkipQueuedBranchBuildsFilter,
		CancelRunningBranchBuilds:       p.CancelRunningBranchBuilds,
		CancelRunningBranchBuildsFilter: p.CancelRunningBranchBuildsFilter,
	}

	// Read pipeline file if provided
	if p.File != "" {
		fileContent, err := os.ReadFile(p.File)
		if err != nil {
			return opts, fmt.Errorf("error reading pipeline file: %w", err)
		}
		opts.Configuration = string(fileContent)
	}

	var questions []*survey.Question

	// Ask for pipeline name if not provided
	if opts.Name == "" {
		questions = append(questions, &survey.Question{
			Name:     "name",
			Prompt:   &survey.Input{Message: "Pipeline name:"},
			Validate: survey.Required,
		})
	}

	// Ask for description if not provided
	if opts.Description == "" {
		questions = append(questions, &survey.Question{
			Name:     "description",
			Prompt:   &survey.Input{Message: "Description:"},
			Validate: survey.Required,
		})
	}

	// Ask for repository if not provided
	if opts.Repository == "" {
		repoURLs := getRepoURLS(f)
		if len(repoURLs) > 0 {
			questions = append(questions, &survey.Question{
				Name: "repository",
				Prompt: &survey.Select{
					Message: "Choose a repository:",
					Options: repoURLs,
				},
			})
		} else {
			questions = append(questions, &survey.Question{
				Name:     "repository",
				Prompt:   &survey.Input{Message: "Repository URL:"},
				Validate: survey.Required,
			})
		}
	}

	// If we have questions to ask, ask them
	if len(questions) > 0 {
		answers := make(map[string]interface{})
		err := survey.Ask(questions, &answers)
		if err != nil {
			return opts, err
		}

		// Apply the answers
		if name, ok := answers["name"]; ok {
			opts.Name = name.(string)
		}
		if description, ok := answers["description"]; ok {
			opts.Description = description.(string)
		}
		if repository, ok := answers["repository"]; ok {
			opts.Repository = repository.(string)
		}
	}

	return opts, nil
}
