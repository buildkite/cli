package add

import (
	"errors"
	"fmt"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
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
	if err := f.Config.SelectOrganization(org, true); err != nil {
		return err
	}

	if f.GitRepository == nil {
		fmt.Println("Notice: Not in a git repository. Organization selection will only apply globally.")
		fmt.Println("If you want to configure a specific project, run this command from a git repository root.")
	}

	return f.Config.SetTokenForOrg(org, token)
}

func ConfigureRun(f *factory.Factory) error {
	var org, token string
	nonEmpty := func(s string) error {
		if len(s) == 0 {
			return errors.New("value cannot be empty")
		}
		return nil
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Organization slug: ").Value(&org).Validate(nonEmpty).Inline(true).Prompt(""),
		),
		huh.NewGroup(
			huh.NewInput().Title("API Token: ").Value(&token).EchoMode(huh.EchoModePassword).Validate(nonEmpty).Inline(true).Prompt(""),
		),
	).WithTheme(huh.ThemeBase16())
	err := form.Run()
	if err != nil {
		return err
	}

	return ConfigureWithCredentials(f, org, token)
}
