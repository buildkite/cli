// Package config contains the configuration for the bk CLI
//
// Configuration can come from files or environment variables. File based configuration works similar to unix config
// file hierarchy where there is a "user" config file found under $HOME, and also a local config in the current
// repository root (referred to as "local" config)
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-git/go-git/v5"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

const (
	DefaultGraphQLEndpoint     = "https://graphql.buildkite.com/v1"
	OrganizationsSlugConfigKey = "organizations"
)

const (
	appData             = "AppData"
	configFilePath      = "bk.yaml"
	localConfigFilePath = "." + configFilePath
	xdgConfigHome       = "XDG_CONFIG_HOME"
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
	// localConfig is the configuration stored in the current directory or any directory above that, stopping at the git
	// root. This file should never contain the `organizations` property because that will include the API token and it
	// could be committed to VCS.
	localConfig *viper.Viper
	// userConfig is the configuration stored in the users HOME directory.
	userConfig *viper.Viper
}

func New(fs afero.Fs, repo *git.Repository) *Config {
	userConfig := viper.New()
	userConfig.SetConfigFile(configFile())
	userConfig.SetConfigType("yaml")
	userConfig.AutomaticEnv()
	if fs != nil {
		userConfig.SetFs(fs)
	}
	// attempt to read in config file but it might not exist
	_ = userConfig.ReadInConfig()

	localConfig := viper.New()
	localConfig.SetConfigType("yaml")
	// if a valid repository is provided, use that as the location for the local config file
	localConfigFile := localConfigFilePath
	if repo != nil {
		wt, _ := repo.Worktree()
		if wt != nil {
			localConfigFile = filepath.Join(wt.Filesystem.Root(), localConfigFilePath)
		}
	}
	localConfig.SetConfigFile(localConfigFile)
	if fs != nil {
		localConfig.SetFs(fs)
	}
	_ = localConfig.ReadInConfig()

	return &Config{
		localConfig: localConfig,
		userConfig:  userConfig,
	}
}

// OrganizationSlug gets the slug for the currently selected organization. This can be configured locally or per user.
// This will search for configuration in that order.
func (conf *Config) OrganizationSlug() string {
	return firstNonEmpty(
		conf.localConfig.GetString("selected_org"),
		conf.userConfig.GetString("selected_org"),
	)
}

// SelectOrganization sets the selected organization in the local configuration file
func (conf *Config) SelectOrganization(org string) error {
	conf.localConfig.Set("selected_org", org)
	return conf.localConfig.WriteConfig()
}

// APIToken gets the API token configured for the currently selected organization
func (conf *Config) APIToken() string {
	slug := conf.OrganizationSlug()
	key := fmt.Sprintf("organizations.%s.api_token", slug)
	return conf.userConfig.GetString(key)
}

// SetTokenForOrg sets the token for the given org in the user configuration file. Tokens are not stored in the local
// configuration file to reduce the likelihood of tokens being committed to VCS
func (conf *Config) SetTokenForOrg(org, token string) error {
	key := fmt.Sprintf("organizations.%s.api_token", org)
	conf.userConfig.Set(key, token)
	return conf.userConfig.WriteConfig()
}

func (conf *Config) ConfiguredOrganizations() []string {
	m := conf.userConfig.GetStringMap("organizations")
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (conf *Config) HasConfiguredOrganization(slug string) bool {
	m := conf.userConfig.GetStringMap("organizations")
	_, ok := m[slug]
	return ok
}

func firstNonEmpty[T comparable](t ...T) T {
	var empty T

	for _, k := range t {
		if k != empty {
			return k
		}
	}

	return empty
}

// Config path precedence: XDG_CONFIG_HOME, AppData (windows only), HOME.
func configFile() string {
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
