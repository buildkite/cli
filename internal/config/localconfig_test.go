package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestLocalConfig(t *testing.T) {
	t.Parallel()

	t.Run("empty viper adds  current local config", func(t *testing.T) {
		t.Parallel()

		v := viper.New()

		l := LocalConfig{
			DefaultPipeline: "bk-1",
			Organization:    "bk",
			Pipelines:       []string{"bk-1"},
			V:               v,
		}

		l.merge()

		p := v.GetString(DefaultPipelineConfigKey)
		if len(p) == 0 {
			t.Error("should have default pipeline present")
		}

		m := v.GetStringMap(OrganizationsSlugConfigKey)
		if len(m) != 1 {
			t.Error("should have config items present")
		}
		if _, ok := m["bk"]; ok {
			switch m["bk"].(type) {
			case map[string]interface{}:
				pipelines := m["bk"].(map[string]interface{})[PipelinesSlugConfigKey].([]string)
				if len(pipelines) != 1 {
					t.Error("should have pipelines present")
				}
				return
			default:
				t.Error("incorrect type in config")
			}
		} else {
			t.Error("org is not present")
		}

	})

}
