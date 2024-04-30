package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

const (
	DefaultPipelineConfigKey = "default_pipeline"
	PipelinesSlugConfigKey   = "pipelines"
)

// LocalConfig contains the configuration for the "cached" pipelines
// and the default selected pipeline
//
// config file format (yaml):
//
//	default_pipeline: buildkite-1
//	organizations:
//		buildkite
//		  pipelines:
//		  - buildkite-1
//		  - buildkite-2
//	   	buildkite-oss
//	  	  pipelines:
//	  	  - buildkite-oss-1
//	      - buildkite-oss-2
type LocalConfig struct {
	DefaultPipeline string
	Organization    string
	Pipelines       []string
	V               ViperLocalConfig
}

type ViperLocalConfig interface {
	Set(string, interface{})
	GetStringMap(string) map[string]interface{}
	WriteConfig() error
}

func LoadLocalConfig(org string) *LocalConfig {

	v := viper.New()
	v.SetConfigFile(localConfigFile())
	v.AddConfigPath(".")
	_ = v.ReadInConfig()

	default_pipeline := v.GetString(DefaultPipelineConfigKey)
	orgs := v.GetStringMap(OrganizationsSlugConfigKey)

	if _, ok := orgs[org]; ok {
		selectedOrgKey := fmt.Sprintf("%s.%s.%s", OrganizationsSlugConfigKey, org, PipelinesSlugConfigKey)
		selectedPipelines := v.GetStringSlice(selectedOrgKey)
		return &LocalConfig{
			DefaultPipeline: default_pipeline,
			Organization:    org,
			Pipelines:       selectedPipelines,
			V:               v,
		}
	}
	return nil
}

func (conf *LocalConfig) merge() {
	orgs := conf.V.GetStringMap(OrganizationsSlugConfigKey)
	orgs[conf.Organization] = map[string]interface{}{
		PipelinesSlugConfigKey: conf.Pipelines,
	}
	conf.V.Set(OrganizationsSlugConfigKey, orgs)
	conf.V.Set(DefaultPipelineConfigKey, conf.DefaultPipeline)
}

// Save sets the current config values into viper and writes the config file
func (conf *LocalConfig) Save() error {
	conf.V.Set(DefaultPipelineConfigKey, conf.DefaultPipeline)
	conf.merge()

	return conf.V.WriteConfig()
}

func localConfigFile() string {
	var path string
	if _, err := os.Stat(".bk.yaml"); err == nil {
		path = ".bk.yaml"
	} else if _, err := os.Stat(".bk.yml"); err == nil {
		path = ".bk.yml"
	}
	return path
}
