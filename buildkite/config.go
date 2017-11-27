package buildkite

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
)

const FormatVersion = 1

type Config struct {
	Path           string `json:"-"`
	Version        int    `json:"version"`
	BuildkiteEmail string `json:"email"`
	BuildkiteUUID  string `json:"uuid"`
}

// ConfigPath returns either $BUILDKITE_CLI_CONFIG_FILE or ~/.buildkite/config.json
func ConfigPath() (string, error) {
	file := os.Getenv("BUILDKITE_CLI_CONFIG_FILE")
	if file == "" {
		home, err := homedir.Dir()
		if err != nil {
			return "", err
		}
		file = filepath.Join(home, ".buildkite", "config.json")
	}
	return file, nil
}

// Open opens and parses the Config, returns a empty Config if one doesn't exist
func OpenConfig() (*Config, error) {
	path, err := ConfigPath()
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
