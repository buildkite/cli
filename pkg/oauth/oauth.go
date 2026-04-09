// Package oauth provides OAuth 2.0 PKCE authentication flow for CLI applications
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultHost = "buildkite.com"
)

// AllScopes is the complete set of Buildkite API token scopes. When no --scopes
// flag is provided, the CLI requests all of these and Buildkite grants only the
// ones the user actually has permission for.
//
// Reference: https://buildkite.com/docs/apis/managing-api-tokens
var AllScopes = []string{
	// CI/CD
	"read_agents",
	"read_artifacts",
	"read_build_logs",
	"read_builds",
	"read_clusters",
	"read_job_env",
	"read_pipeline_templates",
	"read_pipelines",
	"read_rules",
	"write_agents",
	"write_artifacts",
	"write_build_logs",
	"write_builds",
	"write_clusters",
	"write_pipeline_templates",
	"write_pipelines",
	"write_rules",

	// Organization and Users
	"read_organizations",
	"read_teams",
	"read_user",
	"write_teams",

	// Security
	"read_secrets_details",
	"write_secrets",

	// Test Engine
	"read_suites",
	"read_test_plan",
	"write_suites",
	"write_test_plan",

	// Packages
	"delete_packages",
	"delete_registries",
	"read_packages",
	"read_registries",
	"write_packages",
	"write_registries",

	// Portals
	"read_portals",
	"write_portals",
}

// ScopeGroups defines named groups of scopes that can be used with --scopes.
// For example, --scopes "read_only" expands to all read_* scopes.
var ScopeGroups = map[string][]string{
	"read_only": {
		"read_agents",
		"read_artifacts",
		"read_build_logs",
		"read_builds",
		"read_clusters",
		"read_job_env",
		"read_organizations",
		"read_packages",
		"read_pipeline_templates",
		"read_pipelines",
		"read_portals",
		"read_registries",
		"read_rules",
		"read_secrets_details",
		"read_suites",
		"read_teams",
		"read_test_plan",
		"read_user",
	},
}

// ResolveScopes expands scope group names (e.g., "read_only") into their
// individual scopes. Unknown tokens are passed through as literal scopes.
// Multiple groups and individual scopes can be mixed:
//
//	"read_only write_builds" → "read_agents read_artifacts ... write_builds"
func ResolveScopes(input string) string {
	if input == "" {
		return ""
	}

	seen := make(map[string]bool)
	var resolved []string

	for _, token := range strings.Fields(input) {
		if group, ok := ScopeGroups[token]; ok {
			for _, s := range group {
				if !seen[s] {
					seen[s] = true
					resolved = append(resolved, s)
				}
			}
		} else {
			if !seen[token] {
				seen[token] = true
				resolved = append(resolved, token)
			}
		}
	}

	return strings.Join(resolved, " ")
}

// DefaultClientID is the OAuth client ID for the Buildkite CLI
// This can be overridden with ldflags
var DefaultClientID = "5214b230f06b48938ab5"

// Config holds OAuth configuration
type Config struct {
	Host        string // e.g., "buildkite.com"
	ClientID    string // OAuth client ID
	OrgSlug     string // Organization slug (used for organization_uuid lookup)
	OrgUUID     string // Organization UUID
	CallbackURL string // e.g., "http://127.0.0.1:8080/callback"
	Scopes      string // Space-separated OAuth scopes
}

// CallbackResult holds the result from the OAuth callback
type CallbackResult struct {
	Code  string
	State string
	Error string
}

// TokenResponse holds the token exchange response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// Flow manages an OAuth authentication flow
type Flow struct {
	config       *Config
	codeVerifier string
	state        string
	listener     net.Listener
}

// NewFlow creates a new OAuth flow
func NewFlow(cfg *Config) (*Flow, error) {
	if cfg.Host == "" {
		// Allow override via environment variable for local development
		if envHost := os.Getenv("BUILDKITE_HOST"); envHost != "" {
			cfg.Host = envHost
		} else {
			cfg.Host = DefaultHost
		}
	}
	if cfg.ClientID == "" {
		cfg.ClientID = DefaultClientID
	}
	if cfg.Scopes == "" {
		cfg.Scopes = strings.Join(AllScopes, " ")
	}

	// Generate PKCE verifier and state
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}

	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	var listener net.Listener

	// Only start local callback server if no custom redirect URI provided
	if cfg.CallbackURL == "" {
		var err error
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, fmt.Errorf("failed to start callback server: %w", err)
		}
		cfg.CallbackURL = fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)
	}

	return &Flow{
		config:       cfg,
		codeVerifier: codeVerifier,
		state:        state,
		listener:     listener,
	}, nil
}

// AuthorizationURL returns the URL to open in the browser
func (f *Flow) AuthorizationURL() string {
	codeChallenge := generateCodeChallenge(f.codeVerifier)

	params := url.Values{
		"client_id":             {f.config.ClientID},
		"response_type":         {"code"},
		"scope":                 {f.config.Scopes},
		"redirect_uri":          {f.config.CallbackURL},
		"state":                 {f.state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}

	if f.config.OrgUUID != "" {
		params.Set("organization_uuid", f.config.OrgUUID)
	}

	return fmt.Sprintf("https://%s/oauth/authorize?%s", f.config.Host, params.Encode())
}

// WaitForCallback waits for the OAuth callback and returns the authorization code
func (f *Flow) WaitForCallback(ctx context.Context) (*CallbackResult, error) {
	if f.listener == nil {
		return nil, fmt.Errorf("callback listener not available as a custom CallbackURL was provided")
	}

	resultCh := make(chan *CallbackResult, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		errMsg := r.URL.Query().Get("error")

		result := &CallbackResult{
			Code:  code,
			State: state,
			Error: errMsg,
		}

		// Validate state
		if state != f.state {
			result.Error = "state mismatch - possible CSRF attack"
		}

		// Send response to browser
		w.Header().Set("Content-Type", "text/html")
		if result.Error == "" && result.Code != "" {
			// This is the page which lets folks know if they have been auth'd etc
			// then that they can close the window, I tried adding an emoji in here
			// but it renders weird
			fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Authentication Successful</title></head>
<body style="font-family: system-ui, sans-serif; text-align: center; padding: 50px;">
<h1>&#10003; Authentication Successful</h1>
<p>You can close this window and return to your terminal.</p>
</body>
</html>`)
		} else {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Authentication Failed</title></head>
<body style="font-family: system-ui, sans-serif; text-align: center; padding: 50px;">
<h1>&#10005; Authentication Failed</h1>
<p>Error: %s</p>
</body>
</html>`, result.Error)
		}

		resultCh <- result
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(f.listener); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	select {
	case result := <-resultCh:
		if result.Error != "" {
			return nil, fmt.Errorf("authorization failed: %s", result.Error)
		}
		return result, nil
	case err := <-errCh:
		return nil, fmt.Errorf("callback server error: %w", err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ExchangeCode exchanges the authorization code for an access token
func (f *Flow) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	tokenURL := fmt.Sprintf("https://%s/oauth/token", f.config.Host)

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {f.config.ClientID},
		"redirect_uri":  {f.config.CallbackURL},
		"code_verifier": {f.codeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("token error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	return &tokenResp, nil
}

// RefreshAccessToken exchanges a refresh token for a new access token and refresh token.
func RefreshAccessToken(ctx context.Context, host, clientID, refreshToken string) (*TokenResponse, error) {
	if host == "" {
		if envHost := os.Getenv("BUILDKITE_HOST"); envHost != "" {
			host = envHost
		} else {
			host = DefaultHost
		}
	}
	if clientID == "" {
		clientID = DefaultClientID
	}

	tokenURL := fmt.Sprintf("https://%s/oauth/token", host)

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("token refresh error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in refresh response")
	}

	return &tokenResp, nil
}

// Close cleans up the OAuth flow resources
func (f *Flow) Close() error {
	if f.listener != nil {
		return f.listener.Close()
	}
	return nil
}

// generateCodeVerifier generates a PKCE code verifier
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge generates a PKCE code challenge from the verifier
func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// generateState generates a random state parameter
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
