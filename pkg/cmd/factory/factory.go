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
	Config        *config.Config
	HttpClient    *http.Client
	RestAPIClient *buildkite.Client
	Version       string
}

func New(version string) *Factory {
	config := loadFromViper()
	client := httpClient(version, config)
	return &Factory{
		Config:        config,
		HttpClient:    client,
		RestAPIClient: buildkite.NewClient(client),
		Version:       version,
	}
}

func loadFromViper() *config.Config {
	v := viper.New()
	v.SetConfigFile(config.ConfigFile())
	v.AutomaticEnv()
	// attempt to read in config file but it might not exist
	_ = v.ReadInConfig()

	selected := v.GetString(config.SelectedOrgKey)
	orgs := v.GetStringMap(config.OrganizationsSlugConfigKey)

	if org, ok := orgs[selected]; ok {
		return &config.Config{
			Organization: selected,
			APIToken:     org.(map[string]interface{})[config.APITokenConfigKey].(string),
			V:            v,
		}
	}

	// if there is no matching org selected, return an empty config
	// this will be validated elsewhere before a command actually runs
	return &config.Config{V: v}
}

func httpClient(version string, conf *config.Config) *http.Client {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", conf.APIToken),
		"User-Agent":    fmt.Sprintf("Buildkite CLI/%s (%s/%s)", version, runtime.GOOS, runtime.GOARCH),
	}

	return api.NewHTTPClient(headers)
}
