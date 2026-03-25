package watch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestWatchBuild(t *testing.T) {
	t.Run("polls until finished", func(t *testing.T) {
		pollCount := 0
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/builds/1") {
				pollCount++
				b := buildkite.Build{Number: 1, State: "running"}
				if pollCount >= 3 {
					b.State = "passed"
					b.FinishedAt = &buildkite.Timestamp{Time: now}
				}
				json.NewEncoder(w).Encode(b)
				return
			}
			http.NotFound(w, r)
		}))
		defer s.Close()

		client := newTestClient(t, s.URL)
		var statusCalls int
		b, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) {
			statusCalls++
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.State != "passed" {
			t.Errorf("expected state passed, got %s", b.State)
		}
		if pollCount < 3 {
			t.Errorf("expected at least 3 polls, got %d", pollCount)
		}
		if statusCalls < 3 {
			t.Errorf("expected at least 3 status calls, got %d", statusCalls)
		}
	})

	t.Run("aborts after consecutive errors", func(t *testing.T) {
		pollCount := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pollCount++
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer s.Close()

		client := newTestClient(t, s.URL)
		_, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) {
			t.Error("onStatus should not be called on errors")
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if pollCount < DefaultMaxConsecutiveErrors {
			t.Errorf("expected at least %d polls, got %d", DefaultMaxConsecutiveErrors, pollCount)
		}
	})

	t.Run("resets error count on success", func(t *testing.T) {
		pollCount := 0
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			pollCount++
			// Fail for the first few, then succeed
			if pollCount <= 5 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(buildkite.Build{
				Number:     1,
				State:      "passed",
				FinishedAt: &buildkite.Timestamp{Time: now},
			})
		}))
		defer s.Close()

		client := newTestClient(t, s.URL)
		b, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) {})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.State != "passed" {
			t.Errorf("expected state passed, got %s", b.State)
		}
	})

	t.Run("returns context.DeadlineExceeded on timeout", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(buildkite.Build{Number: 1, State: "running"})
		}))
		defer s.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		client := newTestClient(t, s.URL)
		_, err := WatchBuild(ctx, client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) {})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got: %v", err)
		}
	})

	t.Run("returns context.Canceled on explicit cancel", func(t *testing.T) {
		pollCount := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(buildkite.Build{Number: 1, State: "running"})
		}))
		defer s.Close()

		ctx, cancel := context.WithCancel(context.Background())

		client := newTestClient(t, s.URL)
		_, err := WatchBuild(ctx, client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) {
			pollCount++
			if pollCount >= 2 {
				cancel()
			}
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})

	t.Run("returns skipped build without finished timestamp", func(t *testing.T) {
		pollCount := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			pollCount++
			json.NewEncoder(w).Encode(buildkite.Build{
				Number: 1,
				State:  "skipped",
			})
		}))
		defer s.Close()

		client := newTestClient(t, s.URL)
		b, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) {})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.State != "skipped" {
			t.Errorf("expected state skipped, got %s", b.State)
		}
		if pollCount != 1 {
			t.Errorf("expected 1 poll, got %d", pollCount)
		}
	})
}

func newTestClient(t *testing.T, baseURL string) *buildkite.Client {
	t.Helper()
	client, err := buildkite.NewOpts(
		buildkite.WithBaseURL(baseURL),
		buildkite.WithTokenAuth("test-token"),
	)
	if err != nil {
		t.Fatalf("creating test client: %v", err)
	}
	return client
}
