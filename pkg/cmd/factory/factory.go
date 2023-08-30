package factory

import (
	"net/http"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/viper"
)

type Factory struct {
	Config     *viper.Viper
	HttpClient func() (*http.Client, error)
	Version    string
}

func New(version string) *Factory {
	return &Factory{
		Config:     loadViper(),
		HttpClient: httpClientFunc,
		Version:    version,
	}
}

func loadViper() *viper.Viper {
	v := viper.New()
	v.SetConfigFile(config.ConfigFile())
	v.AutomaticEnv()
	// attempt to read in config file but it might not exist
	_ = v.ReadInConfig()
	return v
}

func httpClientFunc() (*http.Client, error) {
	return nil, nil
}
