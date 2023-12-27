package config

import (
	"os"
	"path/filepath"
	"runtime"
	"gopkg.in/yaml.v2"
)

const (
	APITokenConfigKey          = "api_token"
	OrganizationsSlugConfigKey = "organizations"
	SelectedOrgKey             = "selected_org"
)

const (
	appData        = "AppData"
	configFilePath = "bk.yaml"
	xdgConfigHome  = "XDG_CONFIG_HOME"
)

// Config contains the configuration for the currently selected organization
// to operate on with the CLI application
//
// config file format (yaml):
//
//	selected_org: buildkite
//	organizations:
//	  buildkite:
//	    api_token: <token>
//	  buildkite-oss:
//	    api_token: <token>
type Config struct {
	Organization string
	APIToken     string
	V            ViperConfig
}

type ProjectConfig struct {
	Pipeline string `yaml:"pipeline"`
}

type ViperConfig interface {
	Set(string, interface{})
	GetStringMap(string) map[string]interface{}
	WriteConfig() error
}

func (conf *Config) merge() {
	orgs := conf.V.GetStringMap(OrganizationsSlugConfigKey)
	orgs[conf.Organization] = map[string]interface{}{
		APITokenConfigKey: conf.APIToken,
	}
	conf.V.Set(OrganizationsSlugConfigKey, orgs)
}

// Save sets the current config values into viper and writes the config file
func (conf *Config) Save() error {
	conf.V.Set(SelectedOrgKey, conf.Organization)
	conf.merge()

	return conf.V.WriteConfig()
}

// Config path precedence: XDG_CONFIG_HOME, AppData (windows only), HOME.
func ConfigFile() string {
	var path string
	if a := os.Getenv(xdgConfigHome); a != "" {
		path = filepath.Join(a, configFilePath)
	} else if b := os.Getenv(appData); runtime.GOOS == "windows" && b != "" {
		path = filepath.Join(b, "Buildkite CLI", configFilePath)
	} else {
		c, _ := os.UserHomeDir()
		path = filepath.Join(c, ".config", configFilePath)
	}
	return path
}

func LoadProjectConfig() *ProjectConfig {
	var configFile string
	projectConfig := &ProjectConfig{}

	// Check for both .yaml and .yml extensions
	if _, err := os.Stat("bk.yaml"); err == nil {
		configFile = "bk.yaml"
	} else if _, err := os.Stat("bk.yml"); err == nil {
		configFile = "bk.yml"
	}

	if configFile != "" {
		yamlFile, err := os.ReadFile(configFile)
		if err != nil {
			// Unable to read the file, return nil or handle as needed
			return nil
		}

		err = yaml.Unmarshal(yamlFile, projectConfig)
		if err != nil {
			// Unable to parse the file, return nil or handle as needed
			return nil
		}
	} else {
		// No configuration file found, set the pipeline to the current directory
		dir, err := os.Getwd()
		if err != nil {
			// Unable to get current directory, return nil or handle as needed
			return nil
		}
		projectConfig.Pipeline = filepath.Base(dir) // Successfully set the current directory
	}

	return projectConfig
}
