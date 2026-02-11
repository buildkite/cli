package config

import (
	"fmt"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type UnsetCmd struct {
	Key   string `arg:"" help:"Configuration key to unset"`
	Local bool   `help:"Unset from local (.bk.yaml) instead of user config"`
}

func (c *UnsetCmd) Help() string {
	return `Remove a configuration value, reverting to default.

Examples:
  # Reset output format to default (json)
  $ bk config unset output_format

  # Remove repo-specific setting
  $ bk config unset output_format --local

  # Reset pager to default (less -R)
  $ bk config unset pager`
}

func (c *UnsetCmd) Run() error {
	key, err := ValidateKey(c.Key)
	if err != nil {
		return err
	}

	// Check if key can be unset locally
	if c.Local && key.IsUserOnly() {
		return fmt.Errorf("%s can only be unset from user config (not --local)", key)
	}

	f, err := factory.New()
	if err != nil {
		return err
	}

	return SetConfigValue(f.Config, key, "", c.Local)
}
