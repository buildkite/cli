package config

import (
	"fmt"
	"slices"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type SetCmd struct {
	Key   string `arg:"" help:"Configuration key to set"`
	Value string `arg:"" help:"Value to set"`
	Local bool   `help:"Save to local (.bk.yaml) instead of user config"`
}

func (c *SetCmd) Help() string {
	return `Set a configuration value.

Valid keys:
  selected_org   Organization slug to use
  output_format  Default output format (json, yaml, text)
  no_pager       Disable pager for text output (true, false)
  quiet          Suppress progress output (true, false)
  no_input       Disable interactive prompts (true, false) [user config only]
  pager          Custom pager command [user config only]
  telemetry      Enable anonymous usage telemetry (true, false) [user config only]

Examples:
  # Set default output format to YAML
  $ bk config set output_format yaml

  # Disable pager globally
  $ bk config set no_pager true

  # Set repo-specific output format
  $ bk config set output_format text --local

  # Set a custom pager
  $ bk config set pager "less -RS"`
}

func (c *SetCmd) Run() error {
	key, err := ValidateKey(c.Key)
	if err != nil {
		return err
	}

	// Validate the value
	if validValues := key.ValidValues(); validValues != nil {
		if !slices.Contains(validValues, c.Value) {
			return fmt.Errorf("invalid value %q for %s\nvalid values: %v", c.Value, key, validValues)
		}
	}

	// Check if key can be set locally
	if c.Local && key.IsUserOnly() {
		return fmt.Errorf("%s can only be set in user config (not --local)", key)
	}

	f, err := factory.New()
	if err != nil {
		return err
	}

	return SetConfigValue(f.Config, key, c.Value, c.Local)
}
