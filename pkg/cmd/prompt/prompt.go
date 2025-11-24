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

			// Format the prompt based on shell type
			switch shellType {
			case "bash":
				if format == "" {
					format = "\\[\\033[35m\\](bk:%s)\\[\\033[0m\\]" // Purple color by default
				}
				fmt.Printf(format, org)
			case "zsh":
				if format == "" {
					format = "%%F{magenta}(bk:%s)%%f"
				}
				fmt.Printf(format, org)
			case "fish":
				if format == "" {
					format = "set_color magenta;echo -n '(bk:%s)';set_color normal"
				}
				fmt.Printf(format, org)
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
