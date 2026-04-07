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
		b, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) error {
			statusCalls++
			return nil
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
		_, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) error {
			t.Error("onStatus should not be called on errors")
			return nil
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
		b, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) error { return nil })
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
		_, err := WatchBuild(ctx, client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) error { return nil })
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
		_, err := WatchBuild(ctx, client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) error {
			pollCount++
			if pollCount >= 2 {
				cancel()
			}
			return nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})

	t.Run("nil onStatus callback", func(t *testing.T) {
		now := time.Now()
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(buildkite.Build{
				Number:     1,
				State:      "passed",
				FinishedAt: &buildkite.Timestamp{Time: now},
			})
		}))
		defer s.Close()

		client := newTestClient(t, s.URL)
		b, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.State != "passed" {
			t.Errorf("expected state passed, got %s", b.State)
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
		b, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) error { return nil })
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

	t.Run("returns callback error", func(t *testing.T) {
		callbackErr := errors.New("render failed")
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(buildkite.Build{Number: 1, State: "running"})
		}))
		defer s.Close()

		client := newTestClient(t, s.URL)
		_, err := WatchBuild(context.Background(), client, "org", "pipe", 1, 10*time.Millisecond, func(b buildkite.Build) error {
			return callbackErr
		})
		if !errors.Is(err, callbackErr) {
			t.Fatalf("expected callback error, got %v", err)
		}
	})
}

func TestPollTestFailures(t *testing.T) {
	t.Run("follows pagination", func(t *testing.T) {
		var requestedPages []string
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if r.Method != "GET" || !strings.Contains(r.URL.Path, "/builds/build-123/tests") {
				http.NotFound(w, r)
				return
			}

			requestedPages = append(requestedPages, r.URL.Query().Get("page"))
			if got, want := r.URL.Query().Get("include"), "executions"; got != want {
				t.Fatalf("include = %q, want %q", got, want)
			}
			switch r.URL.Query().Get("page") {
			case "1":
				w.Header().Set("Link", "</v2/analytics/organizations/org/builds/build-123/tests?page=2&per_page=10>; rel=\"next\"")
				json.NewEncoder(w).Encode([]buildkite.BuildTest{
					{ID: "test-1", Name: "first-page failure", Executions: []buildkite.BuildTestExecution{{ID: "exec-1", Status: "failed"}}},
				})
			case "2":
				json.NewEncoder(w).Encode([]buildkite.BuildTest{
					{ID: "test-2", Name: "second-page failure", Executions: []buildkite.BuildTestExecution{{ID: "exec-2", Status: "failed"}}},
				})
			default:
				t.Fatalf("unexpected page %q", r.URL.Query().Get("page"))
			}
		}))
		defer s.Close()

		client := newTestClient(t, s.URL)
		var reported []buildkite.BuildTest
		err := pollTestFailures(context.Background(), client, "org", "build-123", NewTestTracker(), func(newTestChanges []buildkite.BuildTest) error {
			reported = append(reported, newTestChanges...)
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, want := requestedPages, []string{"1", "2"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("requested pages = %v, want %v", got, want)
		}
		if got, want := len(reported), 2; got != want {
			t.Fatalf("reported %d test changes, want %d", got, want)
		}
		if got, want := reported[1].Name, "second-page failure"; got != want {
			t.Fatalf("reported second page failure %q, want %q", got, want)
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
