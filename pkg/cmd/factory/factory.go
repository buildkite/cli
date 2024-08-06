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
	tk, err := buildkite.NewTokenConfig(conf.APIToken(), false)
	var httpClient *http.Client
	if err == nil {
		httpClient = tk.Client()
	}

	return &Factory{
		Config:        conf,
		GitRepository: repo,
		GraphQLClient: graphql.NewClient(config.DefaultGraphQLEndpoint, httpClient),
		HttpClient:    httpClient,
		OpenAIClient:  openai.NewClient(conf.GetOpenAIToken()),
		RestAPIClient: buildkite.NewClient(httpClient),
		Version:       version,
	}
}
