package config

import (
	"os"
	"path/filepath"
	"runtime"
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

type ViperConfig interface {
	Set(string, interface{})
	GetStringMap(string) map[string]interface{}
	WriteConfig() error
}

func (conf *Config) merge() {
	orgs := conf.V.GetStringMap(OrganizationsSlugConfigKey)
	orgs[conf.Organization] = map[string]string{
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
