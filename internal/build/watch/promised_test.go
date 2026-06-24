package watch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

func TestFetchPromisedHardFailures(t *testing.T) {
	t.Run("keeps only running jobs from the failed scope", func(t *testing.T) {
		var gotState []string
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotState = r.URL.Query()["state[]"]
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(buildkite.JobsList{
				Items: []buildkite.Job{
					{ID: "running-fail", State: "running"},
					{ID: "finished-fail", State: "failed"},
					{ID: "canceling-fail", State: "canceling"},
				},
			})
		}))
		defer s.Close()

		failing, err := FetchPromisedHardFailures(context.Background(), newTestClient(t, s.URL), "org", "pipe", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(failing) != 2 || !failing["running-fail"] || !failing["canceling-fail"] {
			t.Fatalf("expected only running/canceling jobs, got %#v", failing)
		}
		if failing["finished-fail"] {
			t.Error("finished job should not be in the running promised set")
		}
		if len(gotState) != 1 || gotState[0] != "failed" {
			t.Errorf("expected state=failed query, got %#v", gotState)
		}
	})

	t.Run("empty failed scope yields empty set", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(buildkite.JobsList{Items: []buildkite.Job{}})
		}))
		defer s.Close()

		failing, err := FetchPromisedHardFailures(context.Background(), newTestClient(t, s.URL), "org", "pipe", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(failing) != 0 {
			t.Fatalf("expected empty set, got %#v", failing)
		}
	})

	t.Run("server error is propagated", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
		}))
		defer s.Close()

		_, err := FetchPromisedHardFailures(context.Background(), newTestClient(t, s.URL), "org", "pipe", 1)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "boom") && !strings.Contains(err.Error(), "500") {
			t.Errorf("expected API error, got %v", err)
		}
	})
}
