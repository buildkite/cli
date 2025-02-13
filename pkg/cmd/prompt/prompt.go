package prompt

import (
	"fmt"
	"os"
	"strings"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdPrompt(f *factory.Factory) *cobra.Command {
	var format string
	var shellType string

	cmd := &cobra.Command{
		Use:    "prompt",
		Hidden: true, // Hide from general help as it's meant for shell integration
		Short:  "Print shell prompt integration",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			// Format the prompt based on shell type
			switch shellType {
			case "bash":
				if format == "" {
					format = "\\[\\033[35m\\](bk:%s%s)\\[\\033[0m\\]" // Purple color by default
				}
				fmt.Printf(format, org, strings.Join(warnings, ""))
			case "zsh":
				if format == "" {
					format = "%%F{magenta}(bk:%s%s)%%f"
				}
				fmt.Printf(format, org, strings.Join(warnings, ""))
			case "fish":
				if format == "" {
					format = "set_color magenta;echo -n '(bk:%s%s)';set_color normal"
				}
				fmt.Printf(format, org, strings.Join(warnings, ""))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "", "Custom format string for the prompt")
	cmd.Flags().StringVar(&shellType, "shell", detectShell(), "Shell type (bash, zsh, or fish)")

	return cmd
}

func detectShell() string {
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return "zsh"
	} else if strings.Contains(shell, "fish") {
		return "fish"
	}
	return "bash"
}
