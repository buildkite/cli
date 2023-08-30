package factory

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/buildkite/cli/v3/internal/api"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/viper"
)

type Factory struct {
	Config     *viper.Viper
	HttpClient func() (*http.Client, error)
	Version    string
}

func New(version string) *Factory {
	config := loadViper()
	return &Factory{
		Config:     config,
		HttpClient: httpClientFunc(version, config),
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

func httpClientFunc(version string, v *viper.Viper) func() (*http.Client, error) {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", v.GetString(config.APITokenConfigKey)),
		"User-Agent":    fmt.Sprintf("Buildkite CLI/%s (%s/%s)", version, runtime.GOOS, runtime.GOARCH),
	}

	return func() (*http.Client, error) {
		return api.NewHTTPClient(headers), nil
	}
}
