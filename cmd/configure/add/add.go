package add

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"golang.org/x/term"
)

type AddCmd struct {
	Org   string `help:"Organization slug"`
	Token string `help:"API token"`
}

func (c *AddCmd) Help() string {
	return ` 
Examples:
  # Add configuration for a new organization
  $ bk configure add
`
}

func (c *AddCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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
