package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestConfigMergeOrganizations(t *testing.T) {
	t.Parallel()

	t.Run("empty viper adds current config", func(t *testing.T) {
		t.Parallel()

		v := viper.New()

		c := Config{
			Organization: "testing",
			APIToken:     "test token",
			V:            v,
		}

		c.merge()

		m := v.GetStringMap(organizationsSlugConfigKey)
		if len(m) != 1 {
			t.Error("should have config items present")
		}
		if _, ok := m["testing"]; ok {
			switch m["testing"].(type) {
			case map[string]string:
				return
			default:
				t.Error("incorrect type in config")
			}
		} else {
			t.Error("org is not present")
		}
	})

	t.Run("existing config gets added", func(t *testing.T) {
		t.Parallel()

		v := viper.New()

		c := Config{
			Organization: "testing",
			APIToken:     "test token",
			V:            v,
		}

		c.merge()

		c.Organization = "extra"
		c.merge()

		m := v.GetStringMap(organizationsSlugConfigKey)
		if len(m) != 2 {
			t.Error("should have config items present")
		}
		if _, ok := m["extra"]; ok {
			switch m["extra"].(type) {
			case map[string]string:
				return
			default:
				t.Error("incorrect type in config")
			}
		} else {
			t.Error("org is not present")
		}
	})

	t.Run("existing config gets updated", func(t *testing.T) {
		t.Parallel()

		v := viper.New()

		c := Config{
			Organization: "testing",
			APIToken:     "test token",
			V:            v,
		}

		c.merge()

		c.APIToken = "extra"
		c.merge()

		m := v.GetStringMap(organizationsSlugConfigKey)
		if len(m) != 1 {
			t.Error("should have config items present")
		}
		if org, ok := m["testing"]; ok {
			o := org.(map[string]string)
			if o[apiTokenConfigKey] != "extra" {
				t.Error("api token is not updated")
			}
		} else {
			t.Error("org is not present")
		}
	})
}
