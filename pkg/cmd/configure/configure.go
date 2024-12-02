package configure

import (
	"errors"

	addCmd "github.com/buildkite/cli/v3/pkg/cmd/configure/add"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdConfigure(f *factory.Factory) *cobra.Command {
	var (
		force bool
		org   string
		token string
	)

	cmd := &cobra.Command{
		Use:     "configure",
		Aliases: []string{"config"},
		Args:    cobra.NoArgs,
		Short:   "Configure Buildkite API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			// if the token already exists and --force is not used
			if !force && f.Config.APIToken() != "" {
				return errors.New("API token already configured. You must use --force.")
			}

			// If flags are provided, use them directly
			if org != "" && token != "" {
				return addCmd.ConfigureWithCredentials(f, org, token)
			}

			// Otherwise fall back to interactive mode
			return addCmd.ConfigureRun(f)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force setting a new token")
	cmd.Flags().StringVar(&org, "org", "", "Organization slug")
	cmd.Flags().StringVar(&token, "token", "", "API token")

	cmd.AddCommand(addCmd.NewCmdAdd(f))

	return cmd
}
