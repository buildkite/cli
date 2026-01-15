package config

import (
	"fmt"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type UnsetCmd struct {
	Key   string `arg:"" help:"Configuration key to unset"`
	Local bool   `help:"Unset from local (.bk.yaml) instead of user config"`
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

	conf := f.Config
	inGitRepo := f.GitRepository != nil

	if c.Local && !inGitRepo {
		return fmt.Errorf("--local requires being in a git repository")
	}

	// Determine where to unset (default to user config unless --local)
	unsetLocal := c.Local && inGitRepo

	return SetConfigValue(conf, key, "", unsetLocal)
}
