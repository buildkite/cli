package factory

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/buildkite/cli/v3/internal/api"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/spf13/viper"
)

type Factory struct {
	Config        *viper.Viper
	HttpClient    *http.Client
	RestAPIClient *buildkite.Client
	Version       string
}

func New(version string) *Factory {
	config := loadViper()
	client := httpClient(version, config)
	return &Factory{
		Config:        config,
		HttpClient:    client,
		RestAPIClient: buildkite.NewClient(client),
		Version:       version,
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

func httpClient(version string, v *viper.Viper) *http.Client {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", v.GetString(config.APITokenConfigKey)),
		"User-Agent":    fmt.Sprintf("Buildkite CLI/%s (%s/%s)", version, runtime.GOOS, runtime.GOARCH),
	}

	return api.NewHTTPClient(headers)
}
