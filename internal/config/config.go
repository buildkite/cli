// Package config contains the configuration for the bk CLI
//
// Configuration can come from files or environment variables. File based configuration works similar to unix config
// file hierarchy where there is a "user" config file found under $HOME, and also a local config in the current
// repository root (referred to as "local" config)
package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
	"github.com/goccy/go-yaml"
	"github.com/spf13/afero"
)

var (
	legacyTokenWarningOnce sync.Once
	envTokenWarningOnce    sync.Once
)

const (
	DefaultGraphQLEndpoint = "https://graphql.buildkite.com/v1"

	appData             = "AppData"
	configFilePath      = "bk.yaml"
	localConfigFilePath = "." + configFilePath
	xdgConfigHome       = "XDG_CONFIG_HOME"
)

type orgConfig struct {
	APIToken string `yaml:"api_token,omitempty"`
}

type fileConfig struct {
	SelectedOrg   string               `yaml:"selected_org"`
	Organizations map[string]orgConfig `yaml:"organizations,omitempty"`
	Pipelines     []string             `yaml:"pipelines,omitempty"`
	NoPager       bool                 `yaml:"no_pager,omitempty"`
	OutputFormat  string               `yaml:"output_format,omitempty"`
	Quiet         bool                 `yaml:"quiet,omitempty"`
	NoInput       bool                 `yaml:"no_input,omitempty"`
	Pager         string               `yaml:"pager,omitempty"`
	Telemetry     *bool                `yaml:"telemetry,omitempty"`
	Experiments   string               `yaml:"experiments,omitempty"`
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

// APIToken gets the API token configured for the currently selected organization.
// Precedence: environment variable > keyring > config file (legacy, read-only with warning).
// This is a side-effect-free lookup and does not refresh OAuth sessions.
func (conf *Config) APIToken() string {
	return conf.apiTokenForOrg(conf.OrganizationSlug(), false)
}

// APITokenForOrg gets the API token for a specific organization.
// Precedence: environment variable > keyring > config file (legacy, read-only with warning).
// This is a side-effect-free lookup and does not refresh OAuth sessions.
func (conf *Config) APITokenForOrg(org string) string {
	return conf.apiTokenForOrg(org, false)
}

// RefreshedAPIToken gets the API token for the currently selected organization,
// refreshing an OAuth session first when needed.
func (conf *Config) RefreshedAPIToken() string {
	return conf.apiTokenForOrg(conf.OrganizationSlug(), true)
}

// RefreshedAPITokenForOrg gets the API token for a specific organization,
// refreshing an OAuth session first when needed.
func (conf *Config) RefreshedAPITokenForOrg(org string) string {
	return conf.apiTokenForOrg(org, true)
}

func (conf *Config) apiTokenForOrg(org string, refresh bool) string {
	if token := os.Getenv("BUILDKITE_API_TOKEN"); token != "" {
		envTokenWarningOnce.Do(func() {
			fmt.Fprintln(os.Stderr, "Warning: using BUILDKITE_API_TOKEN environment variable for authentication.")
		})
		return token
	}

	kr := keyring.New()
	if kr.IsAvailable() {
		if session, err := kr.GetSession(org); err == nil && session != nil && session.AccessToken != "" {
			if refresh {
				now := time.Now()
				if !session.CanRefresh() {
					if !session.ExpiresAt.IsZero() && !now.Before(session.ExpiresAt) {
						return ""
					}
					return session.AccessToken
				}
				refreshedSession, refreshErr := conf.refreshOAuthSession(org, kr, session, now)
				if refreshedSession != nil && refreshedSession.AccessToken != "" {
					if refreshErr != nil {
						fmt.Fprintf(os.Stderr, "Warning: failed to refresh OAuth token for %q: %v\n", org, refreshErr)
					}
					return refreshedSession.AccessToken
				}
			} else {
				now := time.Now()
				if !session.ExpiresAt.IsZero() && !now.Before(session.ExpiresAt) {
					return ""
				}
				return session.AccessToken
			}
		}
	}

	// Legacy fallback: read tokens from config files (read-only)
	if token := firstNonEmpty(
		conf.user.getToken(org),
		conf.local.getToken(org),
	); token != "" {
		legacyTokenWarningOnce.Do(func() {
			fmt.Fprintln(os.Stderr, "Warning: reading API token from config file is deprecated. Run `bk auth login` to store your token securely in the system keychain.")
		})
		return token
	}

	return ""
}

func (conf *Config) ShouldFallbackToSelectedOrg(org string) bool {
	if org == "" || org == conf.OrganizationSlug() {
		return false
	}

	return !conf.HasStoredTokenForOrg(org)
}

// HasStoredTokenForOrg reports whether a token is stored for org in keyring
// or config files, excluding environment variable overrides.
func (conf *Config) HasStoredTokenForOrg(org string) bool {
	if org == "" {
		return false
	}

	kr := keyring.New()
	if kr.IsAvailable() {
		if token, err := kr.Get(org); err == nil && token != "" {
			return true
		}
	}

	// Legacy fallback: check config files (read-only)
	return firstNonEmpty(
		conf.user.getToken(org),
		conf.local.getToken(org),
	) != ""
}

// EnsureOrganization records an organization in user config without requiring
// a token value. This keeps org switching/listing functional for keychain-only
// token storage.
func (conf *Config) EnsureOrganization(org string) error {
	if org == "" {
		return nil
	}
	if conf.user.Organizations == nil {
		conf.user.Organizations = make(map[string]orgConfig)
	}
	if _, exists := conf.user.Organizations[org]; exists {
		return nil
	}
	conf.user.Organizations[org] = orgConfig{}
	return conf.writeUser()
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
		return normaliseRESTAPIEndpoint(value)
	}

	return buildkite.DefaultBaseURL
}

func normaliseRESTAPIEndpoint(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}

	trimmedPath := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(trimmedPath, "/v2") {
		return value
	}

	parsed.Path = strings.TrimSuffix(trimmedPath, "/v2")
	if parsed.Path == "" {
		parsed.Path = "/"
	} else {
		parsed.Path += "/"
	}

	return parsed.String()
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

func (conf *Config) SetNoPager(v bool, saveLocal bool) error {
	if !saveLocal {
		conf.user.NoPager = v
		return conf.writeUser()
	}
	conf.local.NoPager = v
	return conf.writeLocal()
}

// OutputFormat returns the configured output format (json, yaml, text).
// Precedence: env > local > user > default (json)
func (conf *Config) OutputFormat() string {
	return firstNonEmpty(
		os.Getenv("BUILDKITE_OUTPUT_FORMAT"),
		conf.local.OutputFormat,
		conf.user.OutputFormat,
		"json",
	)
}

func (conf *Config) SetOutputFormat(v string, saveLocal bool) error {
	if !saveLocal {
		conf.user.OutputFormat = v
		return conf.writeUser()
	}
	conf.local.OutputFormat = v
	return conf.writeLocal()
}

// Quiet returns whether quiet mode is enabled.
// Precedence: env > local > user
func (conf *Config) Quiet() bool {
	if v, ok := lookupBoolEnv("BUILDKITE_QUIET"); ok {
		return v
	}

	if conf.local.Quiet {
		return true
	}

	return conf.user.Quiet
}

func (conf *Config) SetQuiet(v bool, saveLocal bool) error {
	if !saveLocal {
		conf.user.Quiet = v
		return conf.writeUser()
	}
	conf.local.Quiet = v
	return conf.writeLocal()
}

// NoInput returns whether interactive input is disabled.
// Precedence: env > user (not stored in local config)
func (conf *Config) NoInput() bool {
	if v, ok := lookupBoolEnv("BUILDKITE_NO_INPUT"); ok {
		return v
	}

	return conf.user.NoInput
}

// SetNoInput sets whether interactive input is disabled (user config only)
func (conf *Config) SetNoInput(v bool) error {
	conf.user.NoInput = v
	return conf.writeUser()
}

// Pager returns the configured pager command.
// Precedence: PAGER env > user config > default (less -R)
func (conf *Config) Pager() string {
	return firstNonEmpty(
		os.Getenv("PAGER"),
		conf.user.Pager,
		"less -R",
	)
}

// SetPager sets the pager command (user config only)
func (conf *Config) SetPager(v string) error {
	conf.user.Pager = v
	return conf.writeUser()
}

// TelemetryEnabled returns whether telemetry is enabled.
// Defaults to true if not explicitly set.
// Precedence: env > user config
func (conf *Config) TelemetryEnabled() bool {
	if v, ok := lookupBoolEnv("BK_TELEMETRY"); ok {
		return v
	}

	if conf.user.Telemetry != nil {
		return *conf.user.Telemetry
	}

	return true
}

// SetTelemetry sets whether telemetry is enabled (user config only)
func (conf *Config) SetTelemetry(v bool) error {
	conf.user.Telemetry = &v
	return conf.writeUser()
}

// Experiments returns the comma-separated list of enabled experiments.
// Precedence: env (even if empty) > user config
func (conf *Config) Experiments() string {
	if v, ok := os.LookupEnv("BUILDKITE_EXPERIMENTS"); ok {
		return v
	}
	return conf.user.Experiments
}

// HasExperiment reports whether the given experiment name is enabled.
func (conf *Config) HasExperiment(name string) bool {
	for _, exp := range strings.Split(conf.Experiments(), ",") {
		if exp := strings.TrimSpace(exp); exp != "" && exp == name {
			return true
		}
	}
	return false
}

// SetExperiments sets the experiments string (user config only)
func (conf *Config) SetExperiments(v string) error {
	conf.user.Experiments = v
	return conf.writeUser()
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

// ClearAllOrganizations removes all organization entries and the selected
// organization from the user configuration file.
func (conf *Config) ClearAllOrganizations() error {
	conf.user.Organizations = make(map[string]orgConfig)
	conf.user.SelectedOrg = ""
	return conf.writeUser()
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
func (conf *Config) SetPreferredPipelines(pipelines []pipeline.Pipeline) error {
	// only save pipelines if they are present
	if len(pipelines) == 0 {
		return nil
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
	if err := fs.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if cfg.Organizations == nil {
		cfg.Organizations = make(map[string]orgConfig)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return afero.WriteFile(fs, path, data, 0o600)
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

func (conf *Config) refreshOAuthSession(org string, kr *keyring.Keyring, session *oauth.Session, now time.Time) (*oauth.Session, error) {
	if session == nil || session.AccessToken == "" {
		return nil, nil
	}
	if !session.NeedsRefresh(now) {
		return session, nil
	}

	refreshedToken, err := oauth.RefreshAccessToken(context.Background(), &oauth.Config{Host: session.Host, ClientID: session.ClientID}, session.RefreshToken, session.Scope)
	if err != nil {
		if !session.ExpiresAt.IsZero() && !now.Before(session.ExpiresAt) {
			return nil, err
		}
		return session, err
	}

	refreshedSession := session.Update(refreshedToken, now)
	if err := kr.SetSession(org, refreshedSession); err != nil {
		return refreshedSession, err
	}
	conf.propagateOAuthSessionUpdate(kr, org, session, refreshedSession)

	return refreshedSession, nil
}

func (conf *Config) propagateOAuthSessionUpdate(kr *keyring.Keyring, sourceOrg string, previous, updated *oauth.Session) {
	for _, org := range conf.ConfiguredOrganizations() {
		if org == "" || org == sourceOrg {
			continue
		}

		sibling, err := kr.GetSession(org)
		if err != nil || sibling == nil {
			continue
		}
		if sibling.Host != previous.Host || sibling.ClientID != previous.ClientID {
			continue
		}
		if sibling.RefreshToken != previous.RefreshToken || sibling.AccessToken != previous.AccessToken {
			continue
		}

		_ = kr.SetSession(org, updated)
	}
}
