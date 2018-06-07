package cli

import (
	"fmt"
	"os"

	"github.com/99designs/keyring"
	"github.com/buildkite/cli/config"
	"github.com/buildkite/cli/github"
	"github.com/buildkite/cli/graphql"

	githubclient "github.com/google/go-github/github"

	"golang.org/x/oauth2"
)

type TerminalContext interface {
	Header(h string)
	Println(s ...interface{})
	Printf(s string, v ...interface{})
	WaitForKeyPress(prompt string)
	Spinner() Spinner
	Try() Tryer
	ReadPassword(prompt string) (string, error)
}

type KeyringContext struct {
	Keyring keyring.Keyring
}

func (kc KeyringContext) GithubClient() (*githubclient.Client, error) {
	var token oauth2.Token

	// Try and load from env first
	if envToken := os.Getenv(`GITHUB_OAUTH_TOKEN`); envToken != "" {
		return github.NewClientFromToken(&oauth2.Token{AccessToken: envToken}), nil
	}

	// Otherwise load from keyring
	err := config.RetrieveCredential(kc.Keyring, config.GithubOAuthToken, &token)
	if err != nil {
		return nil, fmt.Errorf("Error retriving github oauth credentials: %v", err)
	}

	return github.NewClientFromToken(&token), nil
}

func (kc KeyringContext) BuildkiteGraphQLClient() (*graphql.Client, error) {
	var token string

	// Try and load from env first
	if envToken := os.Getenv(`BUILDKITE_TOKEN`); envToken != "" {
		return graphql.NewClient(envToken)
	}

	// Otherwise load from keyring
	err := config.RetrieveCredential(kc.Keyring, config.BuildkiteGraphQLToken, &token)
	if err != nil {
		return nil, NewExitError(fmt.Errorf("Error retriving buildkite graphql credentials: %v", err), 1)
	}

	client, err := graphql.NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("Failed to create a client: %v", err)
	}

	return client, nil
}
