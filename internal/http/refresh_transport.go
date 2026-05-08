package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
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
	Base        http.RoundTripper
	TokenSource *TokenSource
	UserAgent   string
}

func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token := t.TokenSource.Token()
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
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

// RefreshTransport wraps an http.RoundTripper to automatically refresh
// expired OAuth access tokens using a stored refresh token.
//
// On a 401 response it:
//  1. Acquires a mutex to serialise concurrent refreshes.
//  2. Checks whether the token has already been refreshed by another
//     goroutine (compare-after-lock).
//  3. If not, exchanges the refresh token for new tokens.
//  4. Persists the new tokens and updates the shared TokenSource.
//  5. Retries the original request with the new token.
type RefreshTransport struct {
	Base        http.RoundTripper
	Org         string
	Keyring     *keyring.Keyring
	TokenSource *TokenSource

	mu sync.Mutex
}

func (t *RefreshTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
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

	// Only attempt refresh if we have a refresh token
	refreshToken, rtErr := t.Keyring.GetRefreshToken(t.Org)
	if rtErr != nil || refreshToken == "" {
		return resp, nil
	}

	// Extract the token that was used for the failed request so we can
	// detect whether another goroutine already refreshed it.
	failedToken := extractBearerToken(req.Header.Get("Authorization"))

	// Attempt token refresh (serialised to prevent concurrent refreshes)
	t.mu.Lock()
	newToken, refreshErr := t.doRefresh(req.Context(), failedToken)
	t.mu.Unlock()

	if refreshErr != nil {
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
	// Compare-after-lock: if the current token differs from the one that
	// failed, another goroutine already refreshed successfully. Skip the
	// refresh and use the new token.
	currentToken := t.TokenSource.Token()
	if currentToken != "" && currentToken != failedToken {
		return currentToken, nil
	}

	// Re-read the refresh token under the lock — it may have been rotated
	// by a concurrent refresh.
	refreshToken, err := t.Keyring.GetRefreshToken(t.Org)
	if err != nil || refreshToken == "" {
		return "", fmt.Errorf("no refresh token available")
	}

	tokenResp, err := oauth.RefreshAccessToken(ctx, "", "", refreshToken)
	if err != nil {
		// Only clear the stored refresh token on explicit grant errors
		// (invalid/expired/revoked). Transient failures (network, 5xx)
		// should not destroy the user's session.
		if isTerminalRefreshError(err) {
			_ = t.Keyring.DeleteRefreshToken(t.Org)
		}
		return "", err
	}

	// Persist the new access token
	if err := t.Keyring.Set(t.Org, tokenResp.AccessToken); err != nil {
		return "", fmt.Errorf("failed to store refreshed access token: %w", err)
	}
	t.TokenSource.SetToken(tokenResp.AccessToken)

	// Rotate the refresh token if a new one was issued
	if tokenResp.RefreshToken != "" {
		if err := t.Keyring.SetRefreshToken(t.Org, tokenResp.RefreshToken); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store rotated refresh token: %v\n", err)
		}
	}

	return tokenResp.AccessToken, nil
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
	if strings.HasPrefix(header, "Bearer ") {
		return header[len("Bearer "):]
	}
	return header
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
