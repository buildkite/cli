package login

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	authBaseURL  = "https://auth.buildkite.com/v1/codes"
	pollInterval = 3 * time.Second
	pollTimeout  = 10 * time.Minute
)

type DeviceCode struct {
	Code             string    `json:"code"`
	Secret           string    `json:"secret"`
	ExpiresAt        time.Time `json:"expiresAt"`
	UserAuthorizeURL string    `json:"userAuthorizeURL"`
}

type AccessTokenResponse struct {
	AccessToken string `json:"accessToken"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

func GenerateDeviceCode(ctx context.Context, clientID string) (*DeviceCode, error) {
	url := fmt.Sprintf("%s/generate", authBaseURL)

	body := map[string]string{
		"description": "Buildkite CLI",
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", clientID))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "buildkite-cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Message != "" {
			return nil, fmt.Errorf("API error: %s", errResp.Message)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var deviceCode DeviceCode
	if err := json.Unmarshal(respBody, &deviceCode); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &deviceCode, nil
}

func PollForAuthorization(ctx context.Context, clientID string, deviceCode *DeviceCode) (string, error) {
	url := fmt.Sprintf("%s/verify", authBaseURL)

	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	body := map[string]string{
		"code":   deviceCode.Code,
		"secret": deviceCode.Secret,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	for {
		select {
		case <-pollCtx.Done():
			return "", fmt.Errorf("authorization timed out or was cancelled")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(pollCtx, "POST", url, bytes.NewReader(jsonBody))
			if err != nil {
				return "", fmt.Errorf("creating request: %w", err)
			}

			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", clientID))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "buildkite-cli")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}

			respBody, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			switch resp.StatusCode {
			case http.StatusOK:
				var tokenResp AccessTokenResponse
				if err := json.Unmarshal(respBody, &tokenResp); err != nil {
					return "", fmt.Errorf("parsing token response: %w", err)
				}
				return tokenResp.AccessToken, nil

			case http.StatusAccepted:
				continue

			case http.StatusNotFound:
				return "", fmt.Errorf("authorization code not found or expired")

			case http.StatusUnprocessableEntity:
				var errResp ErrorResponse
				if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Message != "" {
					return "", fmt.Errorf("authorization failed: %s", errResp.Message)
				}
				return "", fmt.Errorf("authorization failed")

			default:
				var errResp ErrorResponse
				if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Message != "" {
					return "", fmt.Errorf("unexpected error: %s", errResp.Message)
				}
				return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
			}
		}
	}
}
