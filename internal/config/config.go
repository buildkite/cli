package config

import (
	"os"
	"path/filepath"
	"runtime"
	"errors"
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

func LoadProjectConfig() (*ProjectConfig, error) {
	var configFile string
	projectConfig := &ProjectConfig{}

	// Check for both .yaml and .yml extensions
	if _, err := os.Stat("buildkite.yaml"); err == nil {
		configFile = "buildkite.yaml"
	} else if _, err := os.Stat("buildkite.yml"); err == nil {
		configFile = "buildkite.yml"
	}

	if configFile != "" {
		yamlFile, err := os.ReadFile(configFile)
		if err != nil {
			return nil, err
		}

		err = yaml.Unmarshal(yamlFile, projectConfig)
		if err != nil {
			return nil, err
		}
	} else {
		// No configuration file found, get the current directory
		dir, err := os.Getwd()
		if err != nil {
			// Unable to get current directory, return nil or handle as needed
			return nil, err
		}

		if _, err := os.Stat(filepath.Join(dir, ".buildkite")); os.IsNotExist(err) {
			// .buildkite directory not found in current dir, so assume not in a Buildkite project
			return nil, errors.New("No `.buildkite` directory found. Are you in a valid Buildkite project? 🤔")
		}

		projectConfig.Pipeline = filepath.Base(dir)

		return writePipelineToBuildkiteYAML(projectConfig)
	}

	return projectConfig, nil
}

func writePipelineToBuildkiteYAML(projectConfig *ProjectConfig) (*ProjectConfig, error) {
	configFilePath := "buildkite.yaml"

	// Check if buildkite.yaml already exists
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		// The file does not exist; proceed to create and write to it
		configData := map[string]interface{}{
			"pipeline": projectConfig.Pipeline,
		}

		data, err := yaml.Marshal(&configData)
		if err != nil {
			return projectConfig, err
		}

		err = os.WriteFile(configFilePath, data, 0644)
		if err != nil {
			return projectConfig, err
		}
	}
	// If the file exists, do nothing

	return projectConfig, nil
}
