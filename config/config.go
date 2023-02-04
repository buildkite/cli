package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	homedir "github.com/mitchellh/go-homedir"
	"golang.org/x/oauth2"
)

const FormatVersion = 2

type Config struct {
	Path             string        `json:"-"`
	Version          int           `json:"version"`
	BuildkiteEmail   string        `json:"email"`
	BuildkiteUUID    string        `json:"uuid"`
	GraphQLToken     string        `json:"graphql_token"`
	GitHubOAuthToken *oauth2.Token `json:"github_oauth_token"`
}

// Path returns either $BUILDKITE_CLI_CONFIG_FILE, ~/.buildkite/config.json or $XDG_CONFIG_HOME/buildkite/config.json in that order.
func Path() (string, error) {
	if file := os.Getenv("BUILDKITE_CLI_CONFIG_FILE"); file != "" {
		return file, nil
	}

	if home, err := homedir.Dir(); err == nil {
		file := filepath.Join(home, ".buildkite", "config.json")
		if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() {
			return file, nil
		}
	}

	// xdg.CacheFile will create buildkite dir if it doesn't exist but does not touch/create config.json
	return xdg.ConfigFile("buildkite/config.json")
}

func EmojiCachePath() (string, error) {
	if home, err := homedir.Dir(); err == nil {
		dir := filepath.Join(home, ".buildkite", "emoji")
		if info, err := os.Stat(dir); err == nil && info.Mode().IsDir() {
			return dir, nil
		}
	}

	relPath := filepath.Join("buildkite", "emoji")
	if err := os.MkdirAll(relPath, 0755); err != nil {
		return "", err
	}

	return filepath.Join(xdg.CacheHome, relPath), nil
}

// Open opens and parses the Config, returns a empty Config if one doesn't exist
func Open() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	var cfg Config = Config{
		Path:    path,
		Version: FormatVersion,
	}

	jsonBlob, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		return &cfg, nil
	} else if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(jsonBlob, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Write serializes the config to the Path in the config
func (c *Config) Write() error {
	if err := os.MkdirAll(filepath.Dir(c.Path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(c.Path, b, 0600)
}
