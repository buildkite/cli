// Package config contains the configuration for the bk CLI
//
// Configuration can come from files or environment variables. File based configuration works similar to unix config
// file hierarchy where there is a "user" config file found under $HOME, and also a local config in the current
// repository root (referred to as "local" config)
package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"

	"github.com/buildkite/cli/v3/internal/pipeline"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

const (
	DefaultGraphQLEndpoint = "https://graphql.buildkite.com/v1"

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
//	pipelines: # (only in local config)
//	  - first-pipeline
//	  - second-pipeline
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
		os.Getenv("BUILDKITE_ORGANIZATION_SLUG"),
		conf.localConfig.GetString("selected_org"),
		conf.userConfig.GetString("selected_org"),
	)
}

// SelectOrganization sets the selected organization in the configuration file
func (conf *Config) SelectOrganization(org string, inGitRepo bool) error {
	if !inGitRepo {
		conf.userConfig.Set("selected_org", org)
		return conf.userConfig.WriteConfig()
	}

	conf.localConfig.Set("selected_org", org)
	return conf.localConfig.WriteConfig()
}

// APIToken gets the API token configured for the currently selected organization
func (conf *Config) APIToken() string {
	slug := conf.OrganizationSlug()
	key := fmt.Sprintf("organizations.%s.api_token", slug)
	return firstNonEmpty(
		os.Getenv("BUILDKITE_API_TOKEN"),
		conf.userConfig.GetString(key),
	)
}

// SetTokenForOrg sets the token for the given org in the user configuration file. Tokens are not stored in the local
// configuration file to reduce the likelihood of tokens being committed to VCS
func (conf *Config) SetTokenForOrg(org, token string) error {
	key := fmt.Sprintf("organizations.%s.api_token", org)
	conf.userConfig.Set(key, token)
	return conf.userConfig.WriteConfig()
}

// GetTokenForOrg gets the API token for a specific organization from the user configuration
func (conf *Config) GetTokenForOrg(org string) string {
	key := fmt.Sprintf("organizations.%s.api_token", org)
	return conf.userConfig.GetString(key)
}

func (conf *Config) ConfiguredOrganizations() []string {
	m := conf.userConfig.GetStringMap("organizations")
	orgs := slices.Collect(maps.Keys(m))
	if o := os.Getenv("BUILDKITE_ORGANIZATION_SLUG"); o != "" {
		orgs = append(orgs, o)
	}
	return orgs
}

func (conf *Config) GetGraphQLEndpoint() string {
	value := os.Getenv("BUILDKITE_GRAPHQL_ENDPOINT")
	if value != "" {
		return value
	}
	return DefaultGraphQLEndpoint
}

func (conf *Config) RESTAPIEndpoint() string {
	value := os.Getenv("BUILDKITE_REST_API_ENDPOINT")
	if value != "" {
		return value
	}

	return buildkite.DefaultBaseURL
}

func (conf *Config) PagerDisabled() bool {
	if v, ok := lookupBoolEnv("BUILDKITE_NO_PAGER"); ok {
		return v
	}
	if v, ok := lookupBoolEnv("NO_PAGER"); ok {
		return v
	}

	if v := conf.localConfig.Get("no_pager"); v != nil {
		return conf.localConfig.GetBool("no_pager")
	}

	if v := conf.userConfig.Get("no_pager"); v != nil {
		return conf.userConfig.GetBool("no_pager")
	}

	return false
}

func lookupBoolEnv(key string) (bool, bool) {
	v := os.Getenv(key)
	if v == "" {
		return false, false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, false
	}
	return b, true
}

func (conf *Config) HasConfiguredOrganization(slug string) bool {
	return slices.Contains(conf.ConfiguredOrganizations(), slug)
}

// PreferredPipelines will retrieve the list of pipelines from local configuration
func (conf *Config) PreferredPipelines() []pipeline.Pipeline {
	names := conf.localConfig.GetStringSlice("pipelines")

	if len(names) == 0 {
		return []pipeline.Pipeline{}
	}

	pipelines := make([]pipeline.Pipeline, len(names))
	for i, v := range names {
		pipelines[i] = pipeline.Pipeline{
			Name: v,
			Org:  conf.OrganizationSlug(),
		}
	}

	return pipelines
}

// SetPreferredPipelines will write the provided list of pipelines to local configuration
func (conf *Config) SetPreferredPipelines(pipelines []pipeline.Pipeline, inGitRepo bool) error {
	// only save pipelines if they are present
	if len(pipelines) == 0 {
		return nil
	}

	if !inGitRepo {
		return fmt.Errorf("cannot save preferred pipelines: not in a git repository")
	}

	names := make([]string, len(pipelines))
	for i, p := range pipelines {
		names[i] = p.Name
	}
	conf.localConfig.Set("pipelines", names)
	return conf.localConfig.WriteConfig()
}

func firstNonEmpty(s ...string) string {
	for _, k := range s {
		if k != "" {
			return k
		}
	}

	return ""
}

// Config path precedence: XDG_CONFIG_HOME, AppData (windows only), HOME.
func configFile() string {
	var path string
	if a := os.Getenv(xdgConfigHome); a != "" {
		path = filepath.Join(a, configFilePath)
	} else if b := os.Getenv(appData); runtime.GOOS == "windows" && b != "" {
		path = filepath.Join(b, "Buildkite CLI", configFilePath)
	} else {
		c, err := createIfNotExistsConfigDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(c, configFilePath)
	}
	return path
}

func createIfNotExistsConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".config")
	if _, err := os.Stat(configDir); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(configDir, 0o755)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		// Other error occurred in checking the dir
		return "", err
	}
	return configDir, nil
}
