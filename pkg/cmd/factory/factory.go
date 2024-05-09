package factory

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/Khan/genqlient/graphql"
	"github.com/buildkite/cli/v3/internal/api"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/go-git/go-git/v5"
)

type Factory struct {
	Config        *config.Config
	GitRepository *git.Repository
	GraphQLClient graphql.Client
	HttpClient    *http.Client
	LocalConfig   *config.LocalConfig
	RestAPIClient *buildkite.Client
	Version       string
}

func New(version string) *Factory {
	repo, _ := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	conf := config.New(nil, repo)
	client := httpClient(version, conf)

	return &Factory{
		Config:        conf,
		GitRepository: repo,
		GraphQLClient: graphql.NewClient(config.DefaultGraphQLEndpoint, client),
		HttpClient:    client,
		RestAPIClient: buildkite.NewClient(client),
		Version:       version,
	}
}

func httpClient(version string, conf *config.Config) *http.Client {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", conf.APIToken()),
		"User-Agent":    fmt.Sprintf("Buildkite CLI/%s (%s/%s)", version, runtime.GOOS, runtime.GOARCH),
	}

	return api.NewHTTPClient(headers)
}
