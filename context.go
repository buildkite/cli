package cli

import (
	"fmt"
	"os"

	"github.com/buildkite/cli/v2/config"
	"github.com/buildkite/cli/v2/github"
	"github.com/buildkite/cli/v2/graphql"

	githubclient "github.com/google/go-github/github"

	"golang.org/x/oauth2"
)

type TerminalContext interface {
	Header(h string)
	Println(s ...interface{})
	Printf(s string, v ...interface{})
	Failure(s string)
	WaitForKeyPress(prompt string)
	Spinner() Spinner
	Try() Tryer
	ReadPassword(prompt string) (string, error)
}

type ConfigContext struct {
}

func (cc ConfigContext) GithubClient() (*githubclient.Client, error) {
	var token oauth2.Token

	// Try and load from env first
	if envToken := os.Getenv(`GITHUB_OAUTH_TOKEN`); envToken != "" {
		return github.NewClientFromToken(&oauth2.Token{AccessToken: envToken}), nil
	}

	// Otherwise load from config
	cfg, err := config.Open()
	if err != nil {
		return nil, fmt.Errorf("Error opening config: %v", err)
	}

	if cfg.GitHubOAuthToken == nil {
		return nil, fmt.Errorf("No github oauth token found")
	}

	return github.NewClientFromToken(&token), nil
}

func (cc ConfigContext) BuildkiteGraphQLClient() (*graphql.Client, error) {
	// Try and load from env first
	if envToken := os.Getenv(`BUILDKITE_TOKEN`); envToken != "" {
		return graphql.NewClient(envToken)
	}

	// Otherwise load from config
	cfg, err := config.Open()
	if err != nil {
		return nil, fmt.Errorf("Error opening config: %v", err)
	}

	if cfg.GraphQLToken == "" {
		return nil, fmt.Errorf("No buildkite graphql token found")
	}

	client, err := graphql.NewClient(cfg.GraphQLToken)
	if err != nil {
		return nil, fmt.Errorf("Failed to create a client: %v", err)
	}

	return client, nil
}
