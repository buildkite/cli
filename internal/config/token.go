package config

import (
	"context"
	"fmt"
	"log"
	"sync"

	httpClient "github.com/buildkite/cli/v3/internal/http"
)

type AccessToken struct {
	UUID     string   `json:"uuid"`
	Scopes   []string `json:"scopes"`
	ClientID string   `json:"client_id"`
}

var (
	tokenInfo     *AccessToken
	tokenInfoOnce sync.Once
)

// GetTokenScopes fetches and returns the scopes associated with the current API token
func (c *Config) GetTokenScopes() []string {
	var err error
	tokenInfoOnce.Do(func() {
		tokenInfo, err = c.fetchTokenInfo(context.Background())
	})

	if err != nil {
		// Log the error but don't expose it to the caller
		// as this is called in the middle of command execution
		log.Printf("Error fetching token info: %v", err)
		return nil
	}

	if tokenInfo == nil {
		return nil
	}

	return tokenInfo.Scopes
}

// fetchTokenInfo retrieves the token information from the Buildkite API
func (c *Config) fetchTokenInfo(ctx context.Context) (*AccessToken, error) {
	client := httpClient.NewClient(
		c.APIToken(),
		httpClient.WithBaseURL(c.RESTAPIEndpoint()),
	)

	var token AccessToken
	err := client.Get(ctx, "/v2/access-token", &token)
	if err != nil {
		return nil, fmt.Errorf("fetching token info: %w", err)
	}

	return &token, nil
}
