package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/buildkite/cli/v3/pkg/oauth"
	"golang.org/x/net/idna"
)

// TokenSource provides thread-safe access to the current access token.
// It is shared between auth-injection points (REST, GraphQL) and
// RefreshTransport so that a refreshed token is immediately visible
// to all subsequent requests.
type TokenSource struct {
	mu    sync.RWMutex
	token string
}

// NewTokenSource creates a TokenSource initialised with the given token.
func NewTokenSource(token string) *TokenSource {
	return &TokenSource{token: token}
}

// Token returns the current access token.
func (ts *TokenSource) Token() string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.token
}

// SetToken updates the current access token.
func (ts *TokenSource) SetToken(token string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.token = token
}

// AuthTransport injects the Authorization header from a TokenSource
// on every outgoing request. It should wrap the base transport so that
// RefreshTransport (which sits outside it) can override the header on
// retries.
type AuthTransport struct {
	Base         http.RoundTripper
	TokenSource  *TokenSource
	UserAgent    string
	APIEndpoints []string
}

func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Go snapshots redirect headers before transport-only auth is injected.
	// Restore missing auth only when the preceding hop used Bearer auth on this
	// same configured API origin; never replace another authentication identity.
	injectAuth := req.Response == nil
	if req.Response != nil {
		previous := req.Response.Request
		injectAuth = req.Header.Get("Authorization") == "" && previous != nil &&
			extractBearerToken(previous.Header.Get("Authorization")) != "" &&
			isAPIRequest(req, t.APIEndpoints) && sameOrigin(req.URL, previous.URL)
	}
	if injectAuth {
		token := t.TokenSource.Token()
		if token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		}
	}
	if t.UserAgent != "" {
		req.Header.Set("User-Agent", t.UserAgent)
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

type credentialStore interface {
	Lock(context.Context, string) (func(), error)
	Set(org, token string) error
	Get(org string) (string, error)
	GetRefreshToken(org string) (string, error)
	SetRefreshToken(org, token string) error
	DeleteRefreshToken(org string) error
	CheckWritable() error
}

var errCredentialPersistence = errors.New("credential persistence failed")

// RefreshTransport wraps an http.RoundTripper to automatically refresh
// expired OAuth access tokens using a stored refresh token.
//
// On a 401 response it:
//  1. Acquires the organization's cross-process credential lock.
//  2. Rereads stored credentials to detect refresh, replacement, or logout.
//  3. If not, exchanges the refresh token for new tokens.
//  4. Persists the new tokens and updates the shared TokenSource.
//  5. Retries the original request with the new token.
type RefreshTransport struct {
	Base        http.RoundTripper
	Org         string
	Keyring     credentialStore
	TokenSource *TokenSource

	// APIEndpoints defines the trusted REST and GraphQL origins for refresh.
	APIEndpoints []string
}

func (t *RefreshTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

func isAPIRequest(req *http.Request, endpoints []string) bool {
	for _, endpoint := range endpoints {
		u, err := url.Parse(endpoint)
		if err == nil && sameOrigin(req.URL, u) {
			return true
		}
	}
	return false
}

func sameOrigin(a, b *url.URL) bool {
	port := effectivePort(a)
	if port < 0 || !strings.EqualFold(a.Scheme, b.Scheme) || port != effectivePort(b) {
		return false
	}
	dialA, wireA, err := canonicalHostnames(a.Hostname())
	if err != nil {
		return false
	}
	dialB, wireB, err := canonicalHostnames(b.Hostname())
	return err == nil && dialA == dialB && wireA == wireB
}

func canonicalHostnames(host string) (string, string, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		// Normalize IP spelling without folding case-sensitive IPv6 zones.
		return ip.String(), ip.String(), nil
	}
	// net/http uses IDNA Lookup for dialing but raw Punycode for Host and
	// forward-proxy authorities. Require both interpretations to agree.
	for _, c := range host {
		if c > 127 {
			dial, err := idna.Lookup.ToASCII(host)
			if err != nil {
				return "", "", err
			}
			wire, err := idna.ToASCII(host)
			return strings.ToLower(dial), strings.ToLower(wire), err
		}
	}
	host = strings.ToLower(host)
	return host, host, nil
}

func effectivePort(u *url.URL) int {
	if port := u.Port(); port != "" {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return -1
		}
		return int(n)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return 443
	case "http":
		return 80
	default:
		return -1
	}
}

func (t *RefreshTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Buffer the request body so it can be replayed on retry.
	// http.NewRequest sets GetBody for standard body types, but
	// custom readers (e.g. from GraphQL clients) may not.
	bufferRequestBody(req)

	resp, err := t.base().RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	// A redirected download can return 401 after stdlib stripped Authorization.
	// Only retry authenticated requests to configured API origins, never add API
	// credentials to an unauthenticated request or an external download retry.
	failedToken := extractBearerToken(req.Header.Get("Authorization"))
	if failedToken == "" || !isAPIRequest(req, t.APIEndpoints) {
		return resp, nil
	}

	newToken, refreshErr := t.doRefresh(req.Context(), failedToken)

	if refreshErr != nil {
		if errors.Is(refreshErr, errCredentialPersistence) || req.Context().Err() != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return nil, refreshErr
		}
		fmt.Fprintf(os.Stderr, "Warning: token refresh failed: %v\n", refreshErr)
		return resp, nil
	}

	// Drain and close the original 401 response body
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	// Clone the request with the new token and retry
	retryReq := req.Clone(req.Context())
	retryReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", newToken))

	// Re-create the body for the retry
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("failed to get request body for retry: %w", err)
		}
		retryReq.Body = body
	}

	return t.base().RoundTrip(retryReq)
}

func (t *RefreshTransport) doRefresh(ctx context.Context, failedToken string) (string, error) {
	release, err := t.Keyring.Lock(ctx, t.Org)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errCredentialPersistence, err)
	}
	defer release()

	currentToken, err := t.Keyring.Get(t.Org)
	if err != nil || currentToken == "" {
		// A queued request must not restore credentials removed by logout.
		return "", errors.New("stored access token unavailable")
	}
	if currentToken != failedToken {
		t.TokenSource.SetToken(currentToken)
		return currentToken, nil
	}

	// Re-read the refresh token under the lock — it may have been rotated
	// by a concurrent refresh.
	refreshToken, err := t.Keyring.GetRefreshToken(t.Org)
	if err != nil || refreshToken == "" {
		return "", fmt.Errorf("no refresh token available")
	}

	// OAuth refresh tokens rotate when exchanged, so verify persistence before
	// asking the server to invalidate the credential currently stored on disk.
	if err := t.Keyring.CheckWritable(); err != nil {
		return "", fmt.Errorf("%w: credential store is not writable: %w", errCredentialPersistence, err)
	}

	tokenResp, err := oauth.RefreshAccessToken(ctx, "", "", refreshToken)
	if err != nil {
		// Only clear the stored refresh token on explicit grant errors
		// (invalid/expired/revoked). Transient failures (network, 5xx)
		// should not destroy the user's session.
		if isTerminalRefreshError(err) {
			token, recovered, rotated := t.recoverRotatedCredentials(refreshToken, failedToken)
			if recovered {
				return token, nil
			}
			if !rotated {
				_ = t.Keyring.DeleteRefreshToken(t.Org)
			}
		}
		return "", err
	}

	// Rotate the refresh token before persisting the new access token so a
	// failed refresh-token write does not leave us holding a newly-issued
	// access token with a stale rotated refresh token.
	if tokenResp.RefreshToken != "" {
		if err := t.Keyring.SetRefreshToken(t.Org, tokenResp.RefreshToken); err != nil {
			return "", fmt.Errorf("%w: failed to store rotated refresh token: %w", errCredentialPersistence, err)
		}
	}

	if err := t.Keyring.Set(t.Org, tokenResp.AccessToken); err != nil {
		return "", fmt.Errorf("%w: failed to store refreshed access token: %w", errCredentialPersistence, err)
	}
	t.TokenSource.SetToken(tokenResp.AccessToken)

	return tokenResp.AccessToken, nil
}

func (t *RefreshTransport) recoverRotatedCredentials(failedRefreshToken, failedToken string) (string, bool, bool) {
	currentRefreshToken, err := t.Keyring.GetRefreshToken(t.Org)
	if err != nil || currentRefreshToken == "" || currentRefreshToken == failedRefreshToken {
		return "", false, false
	}

	accessToken, err := t.Keyring.Get(t.Org)
	if err != nil || accessToken == "" || accessToken == failedToken {
		return "", false, true
	}
	t.TokenSource.SetToken(accessToken)
	return accessToken, true, true
}

// isTerminalRefreshError returns true for OAuth errors that indicate the
// refresh token is permanently invalid and should be cleared.
func isTerminalRefreshError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "invalid_grant") ||
		strings.Contains(msg, "unauthorized_client") ||
		strings.Contains(msg, "invalid_client")
}

// extractBearerToken extracts the token value from a "Bearer <token>" header.
func extractBearerToken(header string) string {
	scheme, token, ok := strings.Cut(header, " ")
	if ok && strings.EqualFold(scheme, "Bearer") {
		return token
	}
	return ""
}

// bufferRequestBody ensures the request body can be replayed for retries.
// If the body is nil or already replayable (GetBody is set), this is a no-op.
func bufferRequestBody(req *http.Request) {
	if req.Body == nil || req.GetBody != nil {
		return
	}
	bodyBytes, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return
	}
	req.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(bodyBytes))), nil
	}
}
