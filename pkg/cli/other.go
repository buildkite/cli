package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/config"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	httpClient "github.com/buildkite/cli/v3/internal/http"
	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/factory"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

// API command
type APICmd struct {
	Method    string     `help:"HTTP method (default: GET or POST if --data is set)"`
	Header    HeaderFlag `help:"HTTP header(s) (KEY=VAL or KEY: VAL)" name:"header"`
	Data      string     `short:"d" help:"Request body data"`
	Analytics bool       `help:"Include analytics data"`
	File      string     `help:"File to upload"`
	Path      string     `arg:"" help:"API path"`
}

func (a *APICmd) Help() string {
	return `EXAMPLES:
  # Get organization info
  bk api ""

  # List builds
  bk api builds

  # List builds with custom headers
  bk api builds --header "Content-Type: application/json"

  # Create a build with POST data
  bk api pipelines/my-pipeline/builds --data '{"commit":"HEAD","branch":"main"}'`
}

// Configure command
type ConfigureCmd struct {
	Main ConfigureMainCmd `cmd:"" default:"withargs" help:"Configure authentication credentials and API tokens"`
	Add  ConfigureAddCmd  `cmd:"" help:"Add configuration for additional organization"`
}

func (c *ConfigureCmd) Help() string {
	return `Stores global configuration (API tokens, organizations) in:
  - Linux/macOS: ~/.config/bk.yaml (or $XDG_CONFIG_HOME/bk.yaml)  
  - Windows: %AppData%\Buildkite CLI\bk.yaml

Local repo configuration can also be stored in .bk.yaml (project settings, no tokens)`
}

// Configure main command (the default)
type ConfigureMainCmd struct {
	Force bool   `help:"Force reconfiguration"`
	Org   string `help:"Organization slug"`
	Token string `help:"API token"`
}

func (c *ConfigureMainCmd) Help() string {
	return `Stores global configuration (API tokens, organizations) in:
  - Linux/macOS: ~/.config/bk.yaml (or $XDG_CONFIG_HOME/bk.yaml)
  - Windows: %AppData%\Buildkite CLI\bk.yaml

Local repo configuration can also be stored in .bk.yaml (project settings, no tokens)

EXAMPLES:
  # Interactive configuration setup
  bk configure

  # Configure with specific organization and token
  bk configure --org my-org --token bkua_abc123

  # Force reconfiguration (overwrite existing config)
  bk configure --force --org my-org --token bkua_abc123`
}

// Configure add command
type ConfigureAddCmd struct {
	Force bool   `help:"Force reconfiguration"`
	Org   string `help:"Organization slug"`
	Token string `help:"API token"`
}

func (c *ConfigureAddCmd) Help() string {
	return `EXAMPLES:
  # Interactive setup for additional organization
  bk configure add

  # Add specific organization configuration
  bk configure add --org second-org --token bkua_xyz789

  # Force add configuration (overwrite if exists)
  bk configure add --force --org second-org --token bkua_xyz789`
}

// Init command
type InitCmd struct{}

func (i *InitCmd) Help() string {
	return `Creates a basic .buildkite/pipeline.yml file to get started with Buildkite.

The interactive process will:
  - Prompt for a pipeline name
  - Prompt for a command to run (e.g., "npm test" or "make build")
  - Create .buildkite/pipeline.yml with a single build step

EXAMPLES:
  # Interactive setup
  bk init

  # Then create the pipeline on Buildkite
  bk pipeline create --name "My Pipeline"

For full pipeline.yml documentation, see:
https://buildkite.com/docs/pipelines/configure`
}

// Prompt command
type PromptCmd struct {
	Format string `help:"Output format"`
	Shell  string `help:"Shell type (bash, zsh, fish)"`
}

func (p *PromptCmd) Help() string {
	return `Examples:
  # Add to bash prompt
  PS1="$(bk prompt --shell=bash)$PS1"
  
  # Add to zsh prompt (with colors)
  PROMPT="$(bk prompt --shell=zsh)$PROMPT"
  
  # Add to fish prompt
  echo (bk prompt --shell=fish)
  
  # Use in shell function for dynamic prompts
  function bk_prompt() { bk prompt --shell=bash; }
  PS1='$(bk_prompt)'$PS1

The command shows the configured organization name in brackets, with warnings 
(!) if token permissions appear limited.`
}

// Use command
type UseCmd struct {
	Organization string `arg:"" optional:"" help:"Organization to switch to" placeholder:"my-org"`
}

func (u *UseCmd) Help() string {
	return `Examples:
  # Switch to a specific organization
  bk use my-company
  
  # Switch to another organization
  bk use acme-corp
  
  # Run without argument to choose from configured organizations
  bk use

The organization slug is saved to your configuration file and will be used 
for subsequent commands until you switch to a different organization.`
}

// Version command
type VersionSub struct{}

func (v *VersionSub) Run(ctx context.Context, f *factory.Factory) error {
	fmt.Printf("bk version %s\n", f.Version)
	return nil
}

// Whoami command
type WhoamiCmd struct {
	OutputFlag
}

func (w *WhoamiCmd) Help() string {
	return `Shows information about the current user and organization based on the configured API token.

EXAMPLES:
  # Show current user and organization
  bk whoami

  # Show information in JSON format
  bk whoami --output json`
}

type whoamiOutput struct {
	OrganizationSlug string `json:"organization_slug"`
	Token            struct {
		UUID        string   `json:"uuid"`
		Description string   `json:"description"`
		Scopes      []string `json:"scopes"`
		User        struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
	} `json:"token"`
}

func (w *WhoamiCmd) Run(ctx context.Context, f *factory.Factory) error {
	w.Apply(f)
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	orgSlug := f.Config.OrganizationSlug()
	if orgSlug == "" {
		orgSlug = "<None>"
	}

	token, _, err := f.RestAPIClient.AccessTokens.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	output := whoamiOutput{
		OrganizationSlug: orgSlug,
	}
	output.Token.UUID = token.UUID
	output.Token.Description = token.Description
	output.Token.Scopes = token.Scopes
	output.Token.User.Name = token.User.Name
	output.Token.User.Email = token.User.Email

	if ShouldUseStructuredOutput(f) {
		return Print(output, f)
	}

	fmt.Printf("Current organization: %s\n", output.OrganizationSlug)
	fmt.Println()
	fmt.Printf("API Token UUID:        %s\n", output.Token.UUID)
	fmt.Printf("API Token Description: %s\n", output.Token.Description)
	fmt.Printf("API Token Scopes:      %v\n", output.Token.Scopes)
	fmt.Println()
	fmt.Printf("API Token user name:  %s\n", output.Token.User.Name)
	fmt.Printf("API Token user email: %s\n", output.Token.User.Email)
	return nil
}

// Help command provides an alternative to flag-based help for better discoverability
// Examples:
//
//	bk help             - Show general help
//	bk help build       - Show help for build command
//	bk help build new   - Show help for build new subcommand
type HelpCmd struct {
	Commands []string `arg:"" optional:"" help:"Commands to show help for"`
}

func (h *HelpCmd) Run(ctx context.Context, f *factory.Factory) error {
	// Build help args - if no commands specified, show main help
	helpArgs := []string{"--help"}

	// If commands are provided, add them before --help
	if len(h.Commands) > 0 {
		helpArgs = append(h.Commands, "--help")
	}

	// Re-parse with help args to trigger help display
	var cli CLI
	parser, err := kong.New(
		&cli,
		kong.Name("bk"),
		kong.Description("The official Buildkite CLI"),
		kong.Vars{"version": f.Version},
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.BindTo(f, (**factory.Factory)(nil)),
		kong.UsageOnError(),
	)
	if err != nil {
		return err
	}

	// Parse the help args to trigger help display
	kongCtx, err := parser.Parse(helpArgs)
	if err != nil {
		// Check if this is an "unexpected argument" error - show help instead
		if errMsg := err.Error(); len(errMsg) > 20 && errMsg[:20] == "unexpected argument " {
			// Show the error message first
			fmt.Fprintf(parser.Stderr, "Error: %s\n\n", errMsg)

			// For subcommand errors, try to show subcommand help first
			if len(h.Commands) > 1 {
				helpArgs := []string{h.Commands[0], "--help"}
				if helpCtx, helpErr := parser.Parse(helpArgs); helpErr == nil {
					_ = helpCtx.Run()
					return nil
				}
			}
			// Fall back to main help
			if helpCtx, helpErr := parser.Parse([]string{"--help"}); helpErr == nil {
				_ = helpCtx.Run()
			}
			return nil
		}
		// Kong will show help and return an error - we can ignore this specific error
		return nil
	}

	// If parsing succeeds, run the context (which will show help)
	err = kongCtx.Run()
	if err != nil {
		return nil
	}

	return nil
}

func (c *ConfigureMainCmd) Run(ctx context.Context, f *factory.Factory) error {
	// if the token already exists and --force is not used
	if !c.Force && f.Config.APIToken() != "" {
		return fmt.Errorf("API token already configured. You must use --force")
	}

	// If flags are provided, use them directly
	if c.Org != "" && c.Token != "" {
		return configureWithCredentials(f, c.Org, c.Token)
	}

	// Otherwise fall back to interactive mode
	return configureRun(f)
}

func (c *ConfigureAddCmd) Run(ctx context.Context, f *factory.Factory) error {

	// If flags are provided, use them directly
	if c.Org != "" && c.Token != "" {
		return configureWithCredentials(f, c.Org, c.Token)
	}

	// Otherwise fall back to interactive mode
	return configureAddRun(f)
}

func (a *APICmd) Run(ctx context.Context, f *factory.Factory) error {
	// Validate configuration
	if err := validateConfig(f.Config); err != nil {
		return err
	}

	// Set default method based on data presence
	method := a.Method
	if a.Data != "" && method == "" {
		method = "POST"
	}
	if method == "" {
		method = "GET"
	}

	// Set endpoint
	endpoint := a.Path
	if endpoint == "" {
		endpoint = "/"
	}

	// Determine endpoint prefix
	var endpointPrefix string
	if a.Analytics {
		endpointPrefix = fmt.Sprintf("v2/analytics/organizations/%s", f.Config.OrganizationSlug())
	} else {
		endpointPrefix = fmt.Sprintf("v2/organizations/%s", f.Config.OrganizationSlug())
	}

	fullEndpoint := endpointPrefix + endpoint

	// Create HTTP client
	client := httpClient.NewClient(
		f.Config.APIToken(),
		httpClient.WithBaseURL(f.RestAPIClient.BaseURL.String()),
	)

	// Process custom headers
	customHeaders := a.Header.Values
	if customHeaders == nil {
		customHeaders = make(map[string]string)
	}

	// Prepare request data
	var requestData interface{}
	if a.Data != "" {
		// Try to parse as JSON first
		if err := json.Unmarshal([]byte(a.Data), &requestData); err != nil {
			// If not JSON, use raw string
			requestData = a.Data
		}
	}

	var response interface{}
	var err error

	// Make the request
	switch method {
	case "GET":
		err = client.DoWithHeaders(ctx, "GET", fullEndpoint, nil, &response, customHeaders)
	case "POST":
		err = client.DoWithHeaders(ctx, "POST", fullEndpoint, requestData, &response, customHeaders)
	case "PUT":
		err = client.DoWithHeaders(ctx, "PUT", fullEndpoint, requestData, &response, customHeaders)
	default:
		err = client.DoWithHeaders(ctx, method, fullEndpoint, requestData, &response, customHeaders)
	}

	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}

	// Format and print the response
	var prettyJSON bytes.Buffer
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("error marshaling response: %w", err)
	}

	err = json.Indent(&prettyJSON, responseBytes, "", "  ")
	if err != nil {
		return fmt.Errorf("error formatting JSON response: %w", err)
	}

	fmt.Println(prettyJSON.String())
	return nil
}

func (u *UseCmd) Run(ctx context.Context, f *factory.Factory) error {
	return useRun(u.Organization, f.Config, f.GitRepository != nil)
}

func useRun(orgArg string, conf *config.Config, inGitRepo bool) error {
	var selected string

	// if no organization provided
	if orgArg == "" {
		// if TTY, prompt to choose from configured orgs
		if isatty.IsTerminal(os.Stdout.Fd()) {
			var err error
			selected, err = bk_io.PromptForOne("organization", conf.ConfiguredOrganizations())
			if err != nil {
				return err
			}
		} else {
			// if not TTY, list configured organizations
			orgs := conf.ConfiguredOrganizations()
			if len(orgs) == 0 {
				return fmt.Errorf("no organizations configured. run `bk configure` to add one")
			}
			for _, org := range orgs {
				fmt.Println(org)
			}
			return nil
		}
	} else {
		selected = orgArg
	}

	// if already selected, do nothing
	if conf.OrganizationSlug() == selected {
		fmt.Printf("Using configuration for `%s`\n", selected)
		return nil
	}

	// if the selected org exists, use it
	if conf.HasConfiguredOrganization(selected) {
		fmt.Printf("Using configuration for `%s`\n", selected)
		return conf.SelectOrganization(selected, inGitRepo)
	}

	// if the selected org doesnt exist, recommend configuring it and error out
	return fmt.Errorf("no configuration found for `%s`. run `bk configure` to add it", selected)
}

// Prompt command implementation
func (p *PromptCmd) Run(ctx context.Context, f *factory.Factory) error {
	org := f.Config.OrganizationSlug()
	if org == "" {
		return nil // No org configured, return empty
	}

	// Get token scopes to potentially show warnings
	scopes := f.Config.GetTokenScopes()
	var warnings []string
	if len(scopes) < 3 { // Example threshold
		warnings = append(warnings, "!")
	}

	// Format output based on shell
	switch strings.ToLower(p.Shell) {
	case "bash":
		if len(warnings) > 0 {
			fmt.Printf("[%s%s] ", strings.Join(warnings, ""), org)
		} else {
			fmt.Printf("[%s] ", org)
		}
	case "zsh":
		if len(warnings) > 0 {
			fmt.Printf("%%F{yellow}[%s%s]%%f ", strings.Join(warnings, ""), org)
		} else {
			fmt.Printf("%%F{green}[%s]%%f ", org)
		}
	case "fish":
		if len(warnings) > 0 {
			fmt.Printf("(set_color yellow)[%s%s](set_color normal) ", strings.Join(warnings, ""), org)
		} else {
			fmt.Printf("(set_color green)[%s](set_color normal) ", org)
		}
	default:
		// Default format
		if len(warnings) > 0 {
			fmt.Printf("[%s%s] ", strings.Join(warnings, ""), org)
		} else {
			fmt.Printf("[%s] ", org)
		}
	}

	return nil
}

func (i *InitCmd) Run(ctx context.Context, f *factory.Factory) error {
	// Check if we're in a git repository
	if f.GitRepository == nil {
		return fmt.Errorf("not in a git repository")
	}

	// Check if pipeline.yml already exists
	if _, err := os.Stat(".buildkite/pipeline.yml"); err == nil {
		return fmt.Errorf("pipeline.yml already exists")
	}

	fmt.Println("Creating pipeline.yml interactively...")

	// Get basic pipeline info
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Pipeline name: ")
	name, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)

	fmt.Print("Command to run (e.g., 'echo hello'): ")
	command, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	command = strings.TrimSpace(command)

	// Create basic pipeline content
	pipelineContent := fmt.Sprintf(`steps:
  - label: "Build"
    command: %s
`, command)

	// Create .buildkite directory if it doesn't exist
	if err := os.MkdirAll(".buildkite", 0755); err != nil {
		return fmt.Errorf("error creating .buildkite directory: %w", err)
	}

	// Write pipeline.yml
	if err := os.WriteFile(".buildkite/pipeline.yml", []byte(pipelineContent), 0644); err != nil {
		return fmt.Errorf("error writing pipeline.yml: %w", err)
	}

	fmt.Println("✓ Created .buildkite/pipeline.yml")
	fmt.Printf("Pipeline ready! You can create it in Buildkite with: bk pipeline create --name \"%s\"\n", name)

	return nil
}

func configureWithCredentials(f *factory.Factory, org, token string) error {
	// Set the token for the organization first
	if err := f.Config.SetTokenForOrg(org, token); err != nil {
		return fmt.Errorf("error setting token: %w", err)
	}
	// Then select the organization
	return f.Config.SelectOrganization(org, false)
}

func configureRun(f *factory.Factory) error {
	fmt.Println("Let's configure the Buildkite CLI!")

	// Get API token
	fmt.Print("Enter your Buildkite API token: ")
	tokenBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("error reading token: %w", err)
	}
	fmt.Println() // New line after password input

	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return fmt.Errorf("API token is required")
	}

	// Test the token by listing organizations
	client := httpClient.NewClient(token)

	var orgs []struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}

	if err := client.Get(context.Background(), "v2/organizations", &orgs); err != nil {
		return fmt.Errorf("error listing organizations (check your token): %w", err)
	}

	if len(orgs) == 0 {
		return fmt.Errorf("no organizations found for this token")
	}

	// Select organization
	fmt.Println("\nAvailable organizations:")
	for i, org := range orgs {
		fmt.Printf("%d. %s (%s)\n", i+1, org.Name, org.Slug)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nSelect organization (number): ")
	selection, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	selection = strings.TrimSpace(selection)
	if selection == "" {
		return fmt.Errorf("organization selection is required")
	}

	// Parse selection
	var selectedOrg string
	for i, org := range orgs {
		if selection == fmt.Sprintf("%d", i+1) {
			selectedOrg = org.Slug
			break
		}
	}

	if selectedOrg == "" {
		return fmt.Errorf("invalid selection")
	}

	// Save configuration
	if err := f.Config.SetTokenForOrg(selectedOrg, token); err != nil {
		return fmt.Errorf("error setting token: %w", err)
	}
	if err := f.Config.SelectOrganization(selectedOrg, false); err != nil {
		return fmt.Errorf("error saving configuration: %w", err)
	}

	fmt.Printf("✓ Configuration saved for organization: %s\n", selectedOrg)
	return nil
}

func configureAddRun(f *factory.Factory) error {
	// Check if we're in a Git repository
	if f.GitRepository == nil {
		return errors.New("not in a Git repository - bk should be configured at the root of a Git repository")
	}

	// Get organization slug
	org, err := promptForInput("Organization slug: ", false)
	if err != nil {
		return err
	}
	if org == "" {
		return errors.New("organization slug cannot be empty")
	}

	// Check if token already exists for this organization
	existingToken := f.Config.GetTokenForOrg(org)
	if existingToken != "" {
		fmt.Printf("Using existing API token for organization: %s\n", org)
		return f.Config.SelectOrganization(org, f.GitRepository != nil)
	}

	// Get API token with password input (no echo)
	token, err := promptForInput("API Token: ", true)
	if err != nil {
		return err
	}
	if token == "" {
		return errors.New("API token cannot be empty")
	}

	fmt.Println("API token set for organization:", org)
	return configureWithCredentials(f, org, token)
}

// promptForInput handles terminal input with optional password masking
func promptForInput(prompt string, isPassword bool) (string, error) {
	fmt.Print(prompt)

	if isPassword {
		// Use term.ReadPassword for secure password input
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println() // Add a newline after password input
		if err != nil {
			return "", err
		}
		return string(passwordBytes), nil
	} else {
		// Use standard input for regular text
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		// Trim whitespace and newlines
		return strings.TrimSpace(input), nil
	}
}

func validateConfig(conf *config.Config) error {
	if conf.APIToken() == "" {
		return bkErrors.NewConfigurationError(nil, "API token not configured. run `bk configure` to set it up")
	}
	if conf.OrganizationSlug() == "" {
		return fmt.Errorf("no organization selected. run `bk use` to select one")
	}
	return nil
}

// parseBuildIdentifier parses a build identifier which can be:
// - A build URL (e.g., "https://buildkite.com/org/pipeline/builds/123")
// - An org/pipeline/number format (e.g., "my-org/my-pipeline/123")
// - A pipeline/number format (e.g., "my-pipeline/123")
// - A build number (e.g., "123") - will need pipeline context
func parseBuildIdentifier(identifier, defaultOrg string) (org, pipeline, buildNumber string, err error) {
	// If it looks like a URL, parse it
	if strings.HasPrefix(identifier, "http") {
		u, parseErr := url.Parse(identifier)
		if parseErr != nil {
			return "", "", "", fmt.Errorf("invalid build URL: %w", parseErr)
		}

		// Expected format: /org/pipeline/builds/number
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 4 && parts[2] == "builds" {
			return parts[0], parts[1], parts[3], nil
		}
		return "", "", "", fmt.Errorf("invalid build URL format")
	}

	// Check for org/pipeline/number or pipeline/number format
	if strings.Contains(identifier, "/") {
		parts := strings.Split(identifier, "/")
		if len(parts) >= 3 {
			// org/pipeline/number format
			return parts[0], parts[1], parts[2], nil
		}
		if len(parts) == 2 {
			// pipeline/number format - use default org
			return defaultOrg, parts[0], parts[1], nil
		}
		return "", "", "", fmt.Errorf("invalid format")
	}

	// Just a build number - return empty org/pipeline
	return "", "", identifier, nil
}
