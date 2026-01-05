// Package config contains the configuration for the bk CLI
//
// Configuration can come from files or environment variables. File based configuration works similar to unix config
// file hierarchy where there is a "user" config file found under $HOME, and also a local config in the current
// repository root (referred to as "local" config)
package config

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"

	"github.com/buildkite/cli/v3/internal/pipeline"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
	"github.com/goccy/go-yaml"
	"github.com/spf13/afero"
)

const (
	DefaultGraphQLEndpoint = "https://graphql.buildkite.com/v1"

	appData             = "AppData"
	configFilePath      = "bk.yaml"
	localConfigFilePath = "." + configFilePath
	xdgConfigHome       = "XDG_CONFIG_HOME"
)

type orgConfig struct {
	APIToken string `yaml:"api_token"`
}

type fileConfig struct {
	SelectedOrg   string               `yaml:"selected_org"`
	Organizations map[string]orgConfig `yaml:"organizations,omitempty"`
	Pipelines     []string             `yaml:"pipelines,omitempty"`
	NoPager       bool                 `yaml:"no_pager,omitempty"`
}

// Config contains the configuration for the currently selected organization
// to operate on with the CLI application
type Config struct {
	fs        afero.Fs
	userPath  string
	localPath string

	user  fileConfig
	local fileConfig
}

func New(fs afero.Fs, repo *git.Repository) *Config {
	if fs == nil {
		fs = afero.NewOsFs()
	}

	userPath := configFile()
	localPath := localConfigFilePath
	if repo != nil {
		if wt, _ := repo.Worktree(); wt != nil {
			localPath = filepath.Join(wt.Filesystem.Root(), localConfigFilePath)
		}
	}

	userCfg, userErr := loadFileConfig(fs, userPath)
	if userErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to read config %s: %v\n", userPath, userErr)
	}

	localCfg, localErr := loadFileConfig(fs, localPath)
	if localErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to read config %s: %v\n", localPath, localErr)
	}

	return &Config{
		fs:        fs,
		userPath:  userPath,
		localPath: localPath,
		user:      userCfg,
		local:     localCfg,
	}
}

// OrganizationSlug gets the slug for the currently selected organization. This can be configured locally or per user.
// This will search for configuration in that order.
func (conf *Config) OrganizationSlug() string {
	return firstNonEmpty(
		os.Getenv("BUILDKITE_ORGANIZATION_SLUG"),
		conf.local.SelectedOrg,
		conf.user.SelectedOrg,
	)
}

// SelectOrganization sets the selected organization in the configuration file
func (conf *Config) SelectOrganization(org string, inGitRepo bool) error {
	if !inGitRepo {
		conf.user.SelectedOrg = org
		return conf.writeUser()
	}

	conf.local.SelectedOrg = org
	return conf.writeLocal()
}

// APIToken gets the API token configured for the currently selected organization
func (conf *Config) APIToken() string {
	slug := conf.OrganizationSlug()
	return firstNonEmpty(
		os.Getenv("BUILDKITE_API_TOKEN"),
		conf.user.getToken(slug),
		conf.local.getToken(slug),
	)
}

// SetTokenForOrg sets the token for the given org in the user configuration file. Tokens are not stored in the local
// configuration file to reduce the likelihood of tokens being committed to VCS
func (conf *Config) SetTokenForOrg(org, token string) error {
	if conf.user.Organizations == nil {
		conf.user.Organizations = make(map[string]orgConfig)
	}
	conf.user.Organizations[org] = orgConfig{APIToken: token}
	return conf.writeUser()
}

// GetTokenForOrg gets the API token for a specific organization from the user configuration
func (conf *Config) GetTokenForOrg(org string) string {
	return conf.user.getToken(org)
}

func (conf *Config) ConfiguredOrganizations() []string {
	orgs := slices.Collect(maps.Keys(conf.user.Organizations))
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

	if conf.local.NoPager {
		return true
	}

	return conf.user.NoPager
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
	names := conf.local.Pipelines

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
	conf.local.Pipelines = names
	return conf.writeLocal()
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

func loadFileConfig(fs afero.Fs, path string) (fileConfig, error) {
	cfg := fileConfig{Organizations: make(map[string]orgConfig)}
	if path == "" {
		return cfg, nil
	}

	file, err := fs.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return cfg, err
	}
	if len(content) == 0 {
		return cfg, nil
	}

	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Organizations == nil {
		cfg.Organizations = make(map[string]orgConfig)
	}
	return cfg, nil
}

func writeFileConfig(fs afero.Fs, path string, cfg fileConfig) error {
	if path == "" {
		return nil
	}

	dir := filepath.Dir(path)
	if err := fs.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if cfg.Organizations == nil {
		cfg.Organizations = make(map[string]orgConfig)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return afero.WriteFile(fs, path, data, 0600)
}

func (cfg fileConfig) getToken(org string) string {
	if org == "" {
		return ""
	}
	if cfg.Organizations == nil {
		return ""
	}
	if v, ok := cfg.Organizations[org]; ok {
		return v.APIToken
	}
	return ""
}

func (conf *Config) writeUser() error {
	return writeFileConfig(conf.fs, conf.userPath, conf.user)
}

func (conf *Config) writeLocal() error {
	return writeFileConfig(conf.fs, conf.localPath, conf.local)
}
