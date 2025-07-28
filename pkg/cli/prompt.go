package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/buildkite/cli/v3/pkg/factory"
)

// Prompt command
type PromptCmd struct {
	Format string `help:"Output format"`
	Shell  string `help:"Shell type (bash, zsh, fish)"`
}

func (p *PromptCmd) Help() string {
	return `Examples:
  # Add to bash prompt
  PS1="$(bk prompt --shell=bash)$PS1"
  
  # Add to zsh prompt (with colors)
  PROMPT="$(bk prompt --shell=zsh)$PROMPT"
  
  # Add to fish prompt
  echo (bk prompt --shell=fish)
  
  # Use in shell function for dynamic prompts
  function bk_prompt() { bk prompt --shell=bash; }
  PS1='$(bk_prompt)'$PS1

The command shows the configured organization name in brackets, with warnings 
(!) if token permissions appear limited.`
}

// Prompt command implementation
func (p *PromptCmd) Run(ctx context.Context, f *factory.Factory) error {
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

	// Format output based on shell
	switch strings.ToLower(p.Shell) {
	case "bash":
		if len(warnings) > 0 {
			fmt.Printf("[%s%s] ", strings.Join(warnings, ""), org)
		} else {
			fmt.Printf("[%s] ", org)
		}
	case "zsh":
		if len(warnings) > 0 {
			fmt.Printf("%%F{yellow}[%s%s]%%f ", strings.Join(warnings, ""), org)
		} else {
			fmt.Printf("%%F{green}[%s]%%f ", org)
		}
	case "fish":
		if len(warnings) > 0 {
			fmt.Printf("(set_color yellow)[%s%s](set_color normal) ", strings.Join(warnings, ""), org)
		} else {
			fmt.Printf("(set_color green)[%s](set_color normal) ", org)
		}
	default:
		// Default format
		if len(warnings) > 0 {
			fmt.Printf("[%s%s] ", strings.Join(warnings, ""), org)
		} else {
			fmt.Printf("[%s] ", org)
		}
	}

	return nil
}
