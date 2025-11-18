package login

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

const (
	defaultClientID = "43053b652d3093a5dfc9"
)

func NewCmdLogin(f *factory.Factory) *cobra.Command {
	var org string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Buildkite",
		Long: `Authenticate with Buildkite using device flow.

This command will open your browser to authorize the CLI. Once authorized,
your API token will be stored in your configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd.Context(), f, org)
		},
	}

	cmd.Flags().StringVarP(&org, "org", "o", "", "Organization slug")

	return cmd
}

func runLogin(ctx context.Context, f *factory.Factory, org string) error {
	clientID := getClientID()

	fmt.Println("Generating authorization code...")
	deviceCode, err := GenerateDeviceCode(ctx, clientID)
	if err != nil {
		return fmt.Errorf("failed to generate device code: %w", err)
	}

	fmt.Printf("\nPlease authorize in your browser:\n")
	fmt.Printf("  %s\n\n", deviceCode.UserAuthorizeURL)
	fmt.Printf("Code: %s\n", deviceCode.Code)
	fmt.Printf("Expires: %s\n\n", deviceCode.ExpiresAt.Format(time.RFC3339))

	if err := browser.OpenURL(deviceCode.UserAuthorizeURL); err != nil {
		fmt.Printf("Could not open browser automatically: %v\n", err)
		fmt.Println("Please open the URL manually.")
	}

	fmt.Println("Waiting for authorization...")
	token, err := PollForAuthorization(ctx, clientID, deviceCode)
	if err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	if org == "" {
		fmt.Print("\nEnter organization slug: ")
		fmt.Scanln(&org)
	}

	if org == "" {
		return fmt.Errorf("organization slug is required")
	}

	if err := f.Config.SetTokenForOrg(org, token); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	inGitRepo := f.GitRepository != nil
	if err := f.Config.SelectOrganization(org, inGitRepo); err != nil {
		return fmt.Errorf("failed to select organization: %w", err)
	}

	fmt.Printf("\nSuccessfully authenticated!\n")
	fmt.Printf("Organization: %s\n", org)

	return nil
}

func getClientID() string {
	if id := os.Getenv("BUILDKITE_OAUTH_CLIENT_ID"); id != "" {
		return id
	}
	return defaultClientID
}
