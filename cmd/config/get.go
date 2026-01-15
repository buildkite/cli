package config

import (
	"fmt"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type GetCmd struct {
	Key string `arg:"" help:"Configuration key to get"`
}

func (c *GetCmd) Run() error {
	key, err := ValidateKey(c.Key)
	if err != nil {
		return err
	}

	f, err := factory.New()
	if err != nil {
		return err
	}

	conf := f.Config

	var value string
	switch key {
	case KeySelectedOrg:
		value = conf.OrganizationSlug()
	case KeyOutputFormat:
		value = conf.OutputFormat()
	case KeyNoPager:
		if conf.PagerDisabled() {
			value = "true"
		} else {
			value = "false"
		}
	case KeyQuiet:
		if conf.Quiet() {
			value = "true"
		} else {
			value = "false"
		}
	case KeyNoInput:
		if conf.NoInput() {
			value = "true"
		} else {
			value = "false"
		}
	case KeyPager:
		value = conf.Pager()
	}

	if value != "" {
		fmt.Println(value)
	}

	return nil
}
