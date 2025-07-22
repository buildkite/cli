package whoami

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/cobra"
)

const (
	formatPlain = "plain"
	formatJSON  = "json"
)

func NewCmdWhoami(f *factory.Factory) *cobra.Command {
	output := "plain"

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Print the current user and organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !slices.Contains([]string{formatPlain, formatJSON}, output) {
				return fmt.Errorf("invalid output: %s, must be one of: %s, %s", output, formatPlain, formatJSON)
			}

			orgSlug := f.Config.OrganizationSlug()
			if orgSlug == "" {
				orgSlug = "<None>"
			}

			token, _, err := f.RestAPIClient.AccessTokens.Get(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get access token: %w", err)
			}

			if output == formatJSON {
				type whoami struct {
					OrganizationSlug string                `json:"organization_slug"`
					Token            buildkite.AccessToken `json:"token"`
				}

				whoamiInfo := whoami{
					OrganizationSlug: orgSlug,
					Token:            token,
				}

				err := json.NewEncoder(cmd.OutOrStdout()).Encode(whoamiInfo)
				if err != nil {
					return fmt.Errorf("failed to encode whoami info to JSON: %w", err)
				}

				return nil
			}

			b := strings.Builder{}

			b.WriteString(fmt.Sprintf("Current organization: %s\n", orgSlug))
			b.WriteRune('\n')
			b.WriteString(fmt.Sprintf("API Token Description: %s\n", token.Description))
			b.WriteString(fmt.Sprintf("API Token Scopes: %v\n", token.Scopes))
			b.WriteRune('\n')
			b.WriteString(fmt.Sprintf("API Token user name: %s\n", token.User.Name))
			b.WriteString(fmt.Sprintf("API Token user email: %s\n", token.User.Email))

			fmt.Fprint(cmd.OutOrStdout(), b.String())

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", output, "Output format (plain (default) or json)")
	return cmd
}
