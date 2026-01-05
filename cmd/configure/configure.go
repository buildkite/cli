package configure

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
)

type ConfigureCmd struct {
	Org     string              `help:"Organization slug" optional:""`
	Token   string              `help:"API token" optional:""`
	Force   bool                `help:"Force setting a new token" optional:""`
	Default ConfigureDefaultCmd `cmd:"" optional:"" help:"Configure Buildkite API token" hidden:"" default:"1"`
	Add     ConfigureAddCmd     `cmd:"" optional:"" help:"Add configuration for a new organization"`
}

type ConfigureDefaultCmd struct {
}

type ConfigureAddCmd struct {
}

func (c *ConfigureAddCmd) Help() string {
	return ` 
Examples:
  # Prompt configuration to add for a new organization 
  $ bk configure add

  # Add configure Buildkite API token
  $ bk configure add --org my-org --token my-token  
`
}

func (c *ConfigureCmd) Help() string {
	return ` 
Examples:
  # Configure Buildkite API token
  $ bk configure --org my-org --token my-token

  # Force setting a new token
  $ bk configure --force --org my-org --token my-token 
`
}

func (c *ConfigureCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()

	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	if kongCtx.Command() == "configure default" {
		if !c.Force && f.Config.APIToken() != "" {
			return errors.New("API token already configured. You must use --force")
		}

	}

	// If flags are provided, use them directly
	if c.Org != "" && c.Token != "" {
		return ConfigureWithCredentials(f, c.Org, c.Token)
	}

	return ConfigureRun(f, c.Org)
}

func ConfigureWithCredentials(f *factory.Factory, org, token string) error {
	if err := f.Config.SelectOrganization(org, f.GitRepository != nil); err != nil {
		return err
	}
	return f.Config.SetTokenForOrg(org, token)
}

func ConfigureRun(f *factory.Factory, org string) error {
	// Check if we're in a Git repository
	if f.GitRepository == nil {
		return errors.New("not in a Git repository - bk should be configured at the root of a Git repository")
	}

	if org == "" {
		// Get organization slug
		inputOrg, err := promptForInput("Organization slug: ", false)

		if err != nil {
			return err
		}
		if inputOrg == "" {
			return errors.New("organization slug cannot be empty")
		}
		org = inputOrg
	}
	// Check if token already exists for this organization
	existingToken := getTokenForOrg(f, org)
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
	return ConfigureWithCredentials(f, org, token)
}

// getTokenForOrg retrieves the token for a specific organization from the user config
func getTokenForOrg(f *factory.Factory, org string) string {
	return f.Config.GetTokenForOrg(org)
}

// promptForInput handles terminal input with optional password masking
func promptForInput(prompt string, isPassword bool) (string, error) {
	fmt.Print(prompt)

	if isPassword {
		return io.ReadPassword()
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
