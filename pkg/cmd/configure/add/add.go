package add

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func NewCmdAdd(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Args:  cobra.NoArgs,
		Short: "Add config for new organization",
		Long:  "Add configuration for a new organization.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ConfigureRun(f)
		},
	}

	return cmd
}

func ConfigureWithCredentials(f *factory.Factory, org, token string) error {
	if err := f.Config.SelectOrganization(org); err != nil {
		return err
	}
	return f.Config.SetTokenForOrg(org, token)
}

func ConfigureRun(f *factory.Factory) error {
	// Get organization slug
	org, err := promptForInput("Organization slug: ", false)
	if err != nil {
		return err
	}
	if org == "" {
		return errors.New("organization slug cannot be empty")
	}

	// Check if token already exists for this organization
	existingToken := getTokenForOrg(f, org)
	if existingToken != "" {
		fmt.Printf("Using existing API token for organization: %s\n", org)
		return f.Config.SelectOrganization(org)
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
