package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
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
	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		fmt.Sprintf("%s/v2/access-token", c.RESTAPIEndpoint()),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIToken()))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching token info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var token AccessToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &token, nil
}
