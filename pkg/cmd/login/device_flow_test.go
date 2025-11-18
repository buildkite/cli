package login

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerateDeviceCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/codes/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-client-id" {
			t.Errorf("unexpected authorization: %s", auth)
		}

		resp := DeviceCode{
			Code:             "test-code",
			Secret:           "test-secret",
			ExpiresAt:        time.Now().Add(10 * time.Minute),
			UserAuthorizeURL: "https://buildkite.com/authorize/test-code",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	oldURL := authBaseURL
	defer func() { authBaseURL = oldURL }()
	authBaseURL = server.URL + "/v1/codes"

	ctx := context.Background()
	code, err := GenerateDeviceCode(ctx, "test-client-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code.Code != "test-code" {
		t.Errorf("expected code 'test-code', got '%s'", code.Code)
	}

	if code.Secret != "test-secret" {
		t.Errorf("expected secret 'test-secret', got '%s'", code.Secret)
	}
}

func TestGenerateDeviceCodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{
			Message: "Invalid client ID",
		})
	}))
	defer server.Close()

	oldURL := authBaseURL
	defer func() { authBaseURL = oldURL }()
	authBaseURL = server.URL + "/v1/codes"

	ctx := context.Background()
	_, err := GenerateDeviceCode(ctx, "invalid-client-id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPollForAuthorizationSuccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if callCount < 2 {
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(ErrorResponse{
				Message: "The code has not yet been authorized by the user",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(AccessTokenResponse{
			AccessToken: "test-token",
		})
	}))
	defer server.Close()

	oldURL := authBaseURL
	oldInterval := pollInterval
	defer func() {
		authBaseURL = oldURL
		pollInterval = oldInterval
	}()
	authBaseURL = server.URL + "/v1/codes"
	pollInterval = 100 * time.Millisecond

	ctx := context.Background()
	deviceCode := &DeviceCode{
		Code:   "test-code",
		Secret: "test-secret",
	}

	token, err := PollForAuthorization(ctx, "test-client-id", deviceCode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token != "test-token" {
		t.Errorf("expected token 'test-token', got '%s'", token)
	}

	if callCount < 2 {
		t.Errorf("expected at least 2 calls, got %d", callCount)
	}
}

func TestPollForAuthorizationTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(ErrorResponse{
			Message: "The code has not yet been authorized by the user",
		})
	}))
	defer server.Close()

	oldURL := authBaseURL
	oldInterval := pollInterval
	oldTimeout := pollTimeout
	defer func() {
		authBaseURL = oldURL
		pollInterval = oldInterval
		pollTimeout = oldTimeout
	}()
	authBaseURL = server.URL + "/v1/codes"
	pollInterval = 50 * time.Millisecond
	pollTimeout = 200 * time.Millisecond

	ctx := context.Background()
	deviceCode := &DeviceCode{
		Code:   "test-code",
		Secret: "test-secret",
	}

	_, err := PollForAuthorization(ctx, "test-client-id", deviceCode)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestPollForAuthorizationNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{
			Message: "Code not found",
		})
	}))
	defer server.Close()

	oldURL := authBaseURL
	oldInterval := pollInterval
	defer func() {
		authBaseURL = oldURL
		pollInterval = oldInterval
	}()
	authBaseURL = server.URL + "/v1/codes"
	pollInterval = 100 * time.Millisecond

	ctx := context.Background()
	deviceCode := &DeviceCode{
		Code:   "invalid-code",
		Secret: "test-secret",
	}

	_, err := PollForAuthorization(ctx, "test-client-id", deviceCode)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
