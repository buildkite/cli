package config

import (
	"fmt"
	"os"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type ListCmd struct {
	Local  bool `help:"Only show local configuration" xor:"scope"`
	Global bool `help:"Only show global (user) configuration" xor:"scope"`
}

func (c *ListCmd) Run() error {
	f, err := factory.New()
	if err != nil {
		return err
	}

	conf := f.Config
	inGitRepo := f.GitRepository != nil

	type configItem struct {
		key    string
		value  string
		source string
	}

	var items []configItem

	if !c.Local {
		// Show global/user config values
		if v := conf.OrganizationSlug(); v != "" && !c.Global {
			items = append(items, configItem{string(KeySelectedOrg), v, "effective"})
		}
		if v := conf.OutputFormat(); v != "" {
			items = append(items, configItem{string(KeyOutputFormat), v, "effective"})
		}
		if conf.PagerDisabled() {
			items = append(items, configItem{string(KeyNoPager), "true", "effective"})
		}
		if conf.Quiet() {
			items = append(items, configItem{string(KeyQuiet), "true", "effective"})
		}
		if conf.NoInput() {
			items = append(items, configItem{string(KeyNoInput), "true", "effective"})
		}
		if v := conf.Pager(); v != "" && v != "less -R" {
			items = append(items, configItem{string(KeyPager), v, "effective"})
		}
	}

	if c.Local && !inGitRepo {
		fmt.Fprintln(os.Stderr, "warning: not in a git repository, no local config available")
		return nil
	}

	if len(items) == 0 {
		fmt.Println("No configuration values set.")
		return nil
	}

	for _, item := range items {
		fmt.Printf("%s=%s\n", item.key, item.value)
	}

	return nil
}
