package factory

import (
	"net/http"

	"github.com/Khan/genqlient/graphql"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/go-git/go-git/v5"
	"github.com/sashabaranov/go-openai"
)

type Factory struct {
	Config        *config.Config
	GitRepository *git.Repository
	GraphQLClient graphql.Client
	HttpClient    *http.Client
	OpenAIClient  *openai.Client
	RestAPIClient *buildkite.Client
	Version       string
}

func New(version string) *Factory {
	repo, _ := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	conf := config.New(nil, repo)
	// we use separate clients for now until we upgrade the go-buildkite package to the 3.11+
	tk, err := buildkite.NewTokenConfig(conf.APIToken(), false)
	gtk, _ := buildkite.NewTokenConfig(conf.APIToken(), false)
	var httpClient, graphqlClient *http.Client
	if err == nil {
		// separate clients until we upgrade go-buildkite library
		httpClient = tk.Client()
	}
	graphqlClient = &http.Client{Transport: gtk}

	return &Factory{
		Config:        conf,
		GitRepository: repo,
		GraphQLClient: graphql.NewClient(config.DefaultGraphQLEndpoint, graphqlClient),
		HttpClient:    httpClient,
		OpenAIClient:  openai.NewClient(conf.GetOpenAIToken()),
		RestAPIClient: buildkite.NewClient(httpClient),
		Version:       version,
	}
}
