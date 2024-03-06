package config

import (
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
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

type LocalConfig struct {
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
	projectConfig := &ProjectConfig{}

	// Determine the current directory name for fallback
	dir, err := os.Getwd()
	if err != nil {
		return nil, err // Unable to get current directory
	}
	currentDirName := filepath.Base(dir)

	var configFile string
	if _, err := os.Stat("buildkite.yaml"); err == nil {
		configFile = "buildkite.yaml"
	} else if _, err := os.Stat("buildkite.yml"); err == nil {
		configFile = "buildkite.yml"
	}

	// If a configuration file is found, try to read and parse it
	if configFile != "" {
		yamlFile, err := os.ReadFile(configFile)
		if err != nil {
			return nil, err
		}

		err = yaml.Unmarshal(yamlFile, projectConfig)
		if err != nil {
			return nil, err
		}

		// Check if the "pipeline" key is already set
		if projectConfig.Pipeline != "" {
			return projectConfig, nil // Pipeline is already defined
		}
	}

	projectConfig.Pipeline = currentDirName
	return writePipelineToBuildkiteYAML(projectConfig)
}

func writePipelineToBuildkiteYAML(projectConfig *ProjectConfig) (*ProjectConfig, error) {
	configFilePath := "buildkite.yaml"

	configData := make(map[string]interface{})
	// Attempt to read the existing buildkite.yaml file
	if fileData, err := os.ReadFile(configFilePath); err == nil {
		// File exists, try to parse it
		err := yaml.Unmarshal(fileData, &configData)
		if err != nil {
			return nil, err // Return error if unable to parse the file
		}
	}

	// Add or update the "pipeline" key only if it's not set or empty
	if _, exists := configData["pipeline"]; !exists || configData["pipeline"] == "" {
		configData["pipeline"] = projectConfig.Pipeline
	} else {
		return projectConfig, nil // If "pipeline" is already set, don't modify the file
	}

	// Marshal the data back into YAML format
	data, err := yaml.Marshal(&configData)
	if err != nil {
		return nil, err
	}

	// Write or overwrite the buildkite.yaml file with the updated content
	err = os.WriteFile(configFilePath, data, 0o644)
	if err != nil {
		return nil, err
	}

	return projectConfig, nil
}

// Local config .bk.yaml or .bk.yml may or may not exist. Load it if it does.
func (config *LocalConfig) Read() error {

	var configFile string
	if _, err := os.Stat(".bk.yaml"); err == nil {
		configFile = ".bk.yaml"
	} else if _, err := os.Stat(".bk.yml"); err == nil {
		configFile = ".bk.yml"
	}

	// If a configuration file is found, try to read and parse it
	if configFile != "" {
		yamlFile, err := os.ReadFile(configFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(yamlFile, config)
		if err != nil {
			return err
		}

	}
	return nil
}
