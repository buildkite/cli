package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type failingRoundTripper struct {
	base       http.RoundTripper
	failures   int
	failureErr error
}

func (rt *failingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.failures > 0 {
		rt.failures--
		return nil, rt.failureErr
	}
	return rt.base.RoundTrip(req)
}

func TestRequestDeviceAuthorization(t *testing.T) {
	var sawRequest bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true

		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/oauth/device_authorization" {
			t.Errorf("path = %s, want /oauth/device_authorization", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.FormValue("client_id"); got != "test-client" {
			t.Errorf("client_id = %q, want test-client", got)
		}
		if got := r.FormValue("scope"); got != "read_user read_organizations" {
			t.Errorf("scope = %q, want requested scopes", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceAuthorizationResponse{
			DeviceCode:              "device-code",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "https://buildkite.example/oauth/device",
			VerificationURIComplete: "https://buildkite.example/oauth/device/ABCD-EFGH",
			ExpiresIn:               600,
			Interval:                5,
		})
	}))
	defer server.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = origTransport }()

	deviceAuth, err := RequestDeviceAuthorization(context.Background(), &Config{
		Host:     server.URL[len("https://"):],
		ClientID: "test-client",
		Scopes:   "read_user read_organizations",
	})
	if err != nil {
		t.Fatalf("RequestDeviceAuthorization: %v", err)
	}
	if !sawRequest {
		t.Fatal("server did not receive request")
	}
	if deviceAuth.DeviceCode != "device-code" {
		t.Errorf("DeviceCode = %q, want device-code", deviceAuth.DeviceCode)
	}
	if deviceAuth.UserCode != "ABCD-EFGH" {
		t.Errorf("UserCode = %q, want ABCD-EFGH", deviceAuth.UserCode)
	}
	if deviceAuth.VerificationURIComplete != "https://buildkite.example/oauth/device/ABCD-EFGH" {
		t.Errorf("VerificationURIComplete = %q", deviceAuth.VerificationURIComplete)
	}
}

func TestRequestDeviceAuthorizationError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_scope","error_description":"Invalid scope"}`))
	}))
	defer server.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = origTransport }()

	_, err := RequestDeviceAuthorization(context.Background(), &Config{
		Host:     server.URL[len("https://"):],
		ClientID: "test-client",
		Scopes:   "read_user hack_the_planet",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := err.Error(), "device authorization error: invalid_scope - Invalid scope"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestPollDeviceAccessTokenRetriesPendingAndSlowDown(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++

		if r.URL.Path != "/oauth/token" {
			t.Errorf("path = %s, want /oauth/token", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.FormValue("grant_type"); got != deviceCodeGrantType {
			t.Errorf("grant_type = %q, want %q", got, deviceCodeGrantType)
		}
		if got := r.FormValue("device_code"); got != "device-code" {
			t.Errorf("device_code = %q, want device-code", got)
		}

		w.Header().Set("Content-Type", "application/json")
		switch requests {
		case 1:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"authorization_pending","error_description":"Device authorization is pending"}`))
		case 2:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"slow_down","error_description":"Polling too quickly"}`))
		default:
			w.Write([]byte(`{
				"access_token":"access-token",
				"token_type":"Bearer",
				"scope":"read_user",
				"refresh_token":"refresh-token",
				"expires_in":3600
			}`))
		}
	}))
	defer server.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = origTransport }()

	var sleeps []time.Duration
	tokenResp, err := pollDeviceAccessToken(
		context.Background(),
		&Config{Host: server.URL[len("https://"):], ClientID: "test-client"},
		&DeviceAuthorizationResponse{DeviceCode: "device-code", Interval: 1},
		func(_ context.Context, duration time.Duration) error {
			sleeps = append(sleeps, duration)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("pollDeviceAccessToken: %v", err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
	if tokenResp.AccessToken != "access-token" {
		t.Errorf("AccessToken = %q, want access-token", tokenResp.AccessToken)
	}
	if len(sleeps) != 2 || sleeps[0] != time.Second || sleeps[1] != 6*time.Second {
		t.Fatalf("sleeps = %v, want [1s 6s]", sleeps)
	}
}

func TestPollDeviceAccessTokenRetriesTransientErrors(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++

		if r.URL.Path != "/oauth/token" {
			t.Errorf("path = %s, want /oauth/token", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.FormValue("device_code"); got != "device-code" {
			t.Errorf("device_code = %q, want device-code", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"access_token":"access-token",
			"token_type":"Bearer",
			"scope":"read_user"
		}`))
	}))
	defer server.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = &failingRoundTripper{
		base:       server.Client().Transport,
		failures:   1,
		failureErr: errors.New("temporary network error"),
	}
	defer func() { http.DefaultTransport = origTransport }()

	var sleeps []time.Duration
	tokenResp, err := pollDeviceAccessToken(
		context.Background(),
		&Config{Host: server.URL[len("https://"):], ClientID: "test-client"},
		&DeviceAuthorizationResponse{DeviceCode: "device-code", Interval: 2},
		func(_ context.Context, duration time.Duration) error {
			sleeps = append(sleeps, duration)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("pollDeviceAccessToken: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if tokenResp.AccessToken != "access-token" {
		t.Errorf("AccessToken = %q, want access-token", tokenResp.AccessToken)
	}
	if len(sleeps) != 1 || sleeps[0] != 2*time.Second {
		t.Fatalf("sleeps = %v, want [2s]", sleeps)
	}
}

func TestPollDeviceAccessTokenReturnsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var slept bool
	_, err := pollDeviceAccessToken(
		ctx,
		&Config{Host: "127.0.0.1:1", ClientID: "test-client"},
		&DeviceAuthorizationResponse{DeviceCode: "device-code", Interval: 1},
		func(context.Context, time.Duration) error {
			slept = true
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pollDeviceAccessToken error = %v, want context.Canceled", err)
	}
	if slept {
		t.Fatal("pollDeviceAccessToken slept after context cancellation")
	}
}
