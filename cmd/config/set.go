package config

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type SetCmd struct {
	Key   string `arg:"" help:"Configuration key to set"`
	Value string `arg:"" help:"Value to set"`
	Local bool   `help:"Save to local (.bk.yaml) instead of user config"`
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

	conf := f.Config
	inGitRepo := f.GitRepository != nil

	if c.Local && !inGitRepo {
		return fmt.Errorf("--local requires being in a git repository")
	}

	// Determine where to save (default to user config unless --local)
	saveLocal := c.Local && inGitRepo

	switch key {
	case KeySelectedOrg:
		return conf.SelectOrganization(c.Value, saveLocal)
	case KeyOutputFormat:
		return conf.SetOutputFormat(c.Value, saveLocal)
	case KeyNoPager:
		v, _ := strconv.ParseBool(c.Value)
		return conf.SetNoPager(v, saveLocal)
	case KeyQuiet:
		v, _ := strconv.ParseBool(c.Value)
		return conf.SetQuiet(v, saveLocal)
	case KeyNoInput:
		v, _ := strconv.ParseBool(c.Value)
		return conf.SetNoInput(v)
	case KeyPager:
		return conf.SetPager(c.Value)
	}

	return nil
}
