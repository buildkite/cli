package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	httpClient "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/pkg/factory"
	"golang.org/x/term"
)

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
