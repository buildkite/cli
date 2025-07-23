package whoami

import (
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	"github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/cobra"
)

type whoamiOutput struct {
	OrganizationSlug string                `json:"organization_slug"`
	Token            buildkite.AccessToken `json:"token"`
}

func (w whoamiOutput) TextOutput() string {
	b := strings.Builder{}

	b.WriteString(fmt.Sprintf("Current organization: %s\n", w.OrganizationSlug))
	b.WriteRune('\n')
	b.WriteString(fmt.Sprintf("API Token UUID:        %s\n", w.Token.UUID))
	b.WriteString(fmt.Sprintf("API Token Description: %s\n", w.Token.Description))
	b.WriteString(fmt.Sprintf("API Token Scopes:      %v\n", w.Token.Scopes))
	b.WriteRune('\n')
	b.WriteString(fmt.Sprintf("API Token user name:  %s\n", w.Token.User.Name))
	b.WriteString(fmt.Sprintf("API Token user email: %s\n", w.Token.User.Email))

	return b.String()
}

func NewCmdWhoami(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Print the current user and organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := output.GetFormat(cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to get output format: %w", err)
			}

			orgSlug := f.Config.OrganizationSlug()
			if orgSlug == "" {
				orgSlug = "<None>"
			}

			token, _, err := f.RestAPIClient.AccessTokens.Get(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get access token: %w", err)
			}

			w := whoamiOutput{
				OrganizationSlug: orgSlug,
				Token:            token,
			}

			err = output.Write(cmd.OutOrStdout(), w, format)
			if err != nil {
				return fmt.Errorf("failed to write output: %w", err)
			}

			return nil
		},
	}

	output.AddFlags(cmd.Flags())
	return cmd
}
