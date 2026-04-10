package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimitTransport(t *testing.T) {
	t.Run("passes through non-429 responses", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}))
		defer s.Close()

		rt := NewRateLimitTransport(http.DefaultTransport)
		req, _ := http.NewRequest("GET", s.URL, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("retries on 429 and succeeds", func(t *testing.T) {
		var attempts atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempts.Add(1)
			if n <= 2 {
				w.Header().Set("RateLimit-Reset", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("rate limited"))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}))
		defer s.Close()

		var callbackCalls int
		rt := NewRateLimitTransport(http.DefaultTransport)
		rt.MaxRetries = 3
		rt.OnRateLimit = func(attempt int, delay time.Duration) {
			callbackCalls++
		}

		req, _ := http.NewRequest("GET", s.URL, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 after retries, got %d", resp.StatusCode)
		}
		if got := attempts.Load(); got != 3 {
			t.Errorf("expected 3 total attempts, got %d", got)
		}
		if callbackCalls != 2 {
			t.Errorf("expected 2 callback calls, got %d", callbackCalls)
		}
	})

	t.Run("returns 429 after exhausting retries", func(t *testing.T) {
		var attempts atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			w.Header().Set("RateLimit-Reset", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer s.Close()

		rt := NewRateLimitTransport(http.DefaultTransport)
		rt.MaxRetries = 2

		req, _ := http.NewRequest("GET", s.URL, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("expected 429 after exhausting retries, got %d", resp.StatusCode)
		}
		// 1 initial + 2 retries = 3 total
		if got := attempts.Load(); got != 3 {
			t.Errorf("expected 3 total attempts, got %d", got)
		}
	})

	t.Run("respects context cancellation during backoff", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("RateLimit-Reset", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer s.Close()

		rt := NewRateLimitTransport(http.DefaultTransport)
		rt.MaxRetries = 3

		ctx, cancel := context.WithCancel(context.Background())
		// Cancel shortly after the first 429 is received.
		rt.OnRateLimit = func(attempt int, delay time.Duration) {
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()
		}

		req, _ := http.NewRequestWithContext(ctx, "GET", s.URL, nil)
		_, err := rt.RoundTrip(req)
		if err == nil {
			t.Fatal("expected error from cancelled context, got nil")
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("expected context canceled error, got: %v", err)
		}
	})

	t.Run("uses fallback delay when header missing", func(t *testing.T) {
		var attempts atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempts.Add(1)
			if n == 1 {
				// No RateLimit-Reset header
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer s.Close()

		var gotDelay time.Duration
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rt := NewRateLimitTransport(http.DefaultTransport)
		rt.MaxRetries = 1
		// Cancel quickly to avoid waiting the full fallback delay.
		rt.OnRateLimit = func(attempt int, delay time.Duration) {
			gotDelay = delay
			cancel()
		}

		req, _ := http.NewRequestWithContext(ctx, "GET", s.URL, nil)
		rt.RoundTrip(req)

		if gotDelay != defaultFallbackDelay {
			t.Errorf("expected fallback delay %v, got %v", defaultFallbackDelay, gotDelay)
		}
	})

	t.Run("uses zero delay when header is zero", func(t *testing.T) {
		var attempts atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempts.Add(1)
			if n == 1 {
				w.Header().Set("RateLimit-Reset", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer s.Close()

		var gotDelay time.Duration
		rt := NewRateLimitTransport(http.DefaultTransport)
		rt.MaxRetries = 1
		rt.OnRateLimit = func(attempt int, delay time.Duration) {
			gotDelay = delay
		}

		req, _ := http.NewRequest("GET", s.URL, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if gotDelay != 0 {
			t.Errorf("expected zero delay, got %v", gotDelay)
		}
	})

	t.Run("caps delay at MaxRetryDelay", func(t *testing.T) {
		var attempts atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempts.Add(1)
			if n == 1 {
				w.Header().Set("RateLimit-Reset", "3600")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer s.Close()

		rt := NewRateLimitTransport(http.DefaultTransport)
		rt.MaxRetries = 1
		rt.MaxRetryDelay = 10 * time.Millisecond

		req, _ := http.NewRequest("GET", s.URL, nil)
		start := time.Now()
		resp, err := rt.RoundTrip(req)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		if elapsed > 1*time.Second {
			t.Errorf("expected delay to be capped, but took %v", elapsed)
		}
	})

	t.Run("replays request body on retry", func(t *testing.T) {
		var attempts atomic.Int32
		var bodies []string
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(b))
			n := attempts.Add(1)
			if n == 1 {
				w.Header().Set("RateLimit-Reset", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer s.Close()

		rt := NewRateLimitTransport(http.DefaultTransport)
		rt.MaxRetries = 1

		body := `{"key":"value"}`
		req, _ := http.NewRequest("POST", s.URL, strings.NewReader(body))
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if len(bodies) != 2 {
			t.Fatalf("expected 2 requests, got %d", len(bodies))
		}
		for i, got := range bodies {
			if got != body {
				t.Errorf("attempt %d: body = %q, want %q", i, got, body)
			}
		}
	})
}

func TestParseRateLimitReset(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected time.Duration
		ok       bool
	}{
		{"valid seconds", "30", 30 * time.Second, true},
		{"one second", "1", 1 * time.Second, true},
		{"empty", "", 0, false},
		{"negative", "-1", 0, false},
		{"zero means retry now", "0", 0, true},
		{"non-numeric", "abc", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.header != "" {
				resp.Header.Set("RateLimit-Reset", tt.header)
			}
			got, ok := parseRateLimitReset(resp)
			if got != tt.expected {
				t.Errorf("parseRateLimitReset(%q) = %v, want %v", tt.header, got, tt.expected)
			}
			if ok != tt.ok {
				t.Errorf("parseRateLimitReset(%q) ok = %v, want %v", tt.header, ok, tt.ok)
			}
		})
	}
}
