package queue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func newTestQueue(key, id string, paused bool) buildkite.ClusterQueue {
	return buildkite.ClusterQueue{
		ID:             id,
		Key:            key,
		Description:    "Test queue",
		DispatchPaused: paused,
		CreatedAt:      &buildkite.Timestamp{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
}

func TestCmdQueueList(t *testing.T) {
	t.Parallel()

	t.Run("validates per-page and limit", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			perPage int
			limit   int
			wantErr bool
		}{
			{"valid defaults", 30, 100, false},
			{"per-page zero invalid", 0, 100, true},
			{"per-page negative invalid", -1, 100, true},
			{"limit zero valid", 30, 0, false},
			{"limit negative invalid", 30, -1, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				cmd := &ListCmd{PerPage: tt.perPage, Limit: tt.limit}
				err := cmd.Validate()
				if tt.wantErr && err == nil {
					t.Error("expected error, got nil")
				}
				if !tt.wantErr && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			})
		}
	})

	t.Run("fetches queues through API", func(t *testing.T) {
		t.Parallel()

		queues := []buildkite.ClusterQueue{
			newTestQueue("default", "queue-1", false),
			newTestQueue("deploy", "queue-2", true),
		}

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page := r.URL.Query().Get("page")
			if page == "" || page == "1" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(queues)
			} else {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]buildkite.ClusterQueue{})
			}
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		got, _, err := client.ClusterQueues.List(ctx, "test-org", "cluster-1", &buildkite.ClusterQueuesListOptions{
			ListOptions: buildkite.ListOptions{Page: 1, PerPage: 30},
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != 2 {
			t.Fatalf("expected 2 queues, got %d", len(got))
		}
		if got[0].Key != "default" {
			t.Errorf("expected first key 'default', got %q", got[0].Key)
		}
		if !got[1].DispatchPaused {
			t.Error("expected second queue to be paused")
		}
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]buildkite.ClusterQueue{})
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		got, _, err := client.ClusterQueues.List(ctx, "test-org", "cluster-1", nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("expected 0 queues, got %d", len(got))
		}
	})
}

func TestCmdQueueCreate(t *testing.T) {
	t.Parallel()

	t.Run("validates retry agent affinity", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			affinity string
			wantErr  bool
		}{
			{"empty affinity valid", "", false},
			{"prefer-warmest valid", string(buildkite.RetryAgentAffinityPreferWarmest), false},
			{"prefer-different valid", string(buildkite.RetryAgentAffinityPreferDifferent), false},
			{"invalid affinity", "prefer-random", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				cmd := &CreateCmd{ClusterUUID: "cluster-1", Key: "my-queue", RetryAgentAffinity: tt.affinity}
				err := cmd.Validate()
				if tt.wantErr && err == nil {
					t.Error("expected error, got nil")
				}
				if !tt.wantErr && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			})
		}
	})

	t.Run("creates queue through API", func(t *testing.T) {
		t.Parallel()

		queue := newTestQueue("my-queue", "queue-abc", false)

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(queue)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		got, _, err := client.ClusterQueues.Create(ctx, "test-org", "cluster-1", buildkite.ClusterQueueCreate{
			Key:         "my-queue",
			Description: "Test queue",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Key != "my-queue" {
			t.Errorf("expected key 'my-queue', got %q", got.Key)
		}
	})
}

func TestCmdQueueUpdate(t *testing.T) {
	t.Parallel()

	t.Run("requires at least one field", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			description string
			affinity    string
			wantErr     bool
		}{
			{"no fields provided", "", "", true},
			{"description only", "new desc", "", false},
			{"affinity only valid", string(buildkite.RetryAgentAffinityPreferWarmest), "", false},
			{"both fields", "new desc", string(buildkite.RetryAgentAffinityPreferDifferent), false},
			{"invalid affinity", "", "bad-value", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				cmd := &UpdateCmd{
					ClusterUUID:        "cluster-1",
					QueueUUID:          "queue-1",
					Description:        tt.description,
					RetryAgentAffinity: tt.affinity,
				}
				err := cmd.Validate()
				if tt.wantErr && err == nil {
					t.Error("expected error, got nil")
				}
				if !tt.wantErr && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			})
		}
	})

	t.Run("updates queue through API", func(t *testing.T) {
		t.Parallel()

		updated := newTestQueue("my-queue", "queue-abc", false)
		updated.Description = "Updated description"

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch {
				t.Errorf("expected PATCH, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(updated)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		got, _, err := client.ClusterQueues.Update(ctx, "test-org", "cluster-1", "queue-abc", buildkite.ClusterQueueUpdate{
			Description: "Updated description",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Description != "Updated description" {
			t.Errorf("expected description 'Updated description', got %q", got.Description)
		}
	})
}

func TestCmdQueueDelete(t *testing.T) {
	t.Parallel()

	t.Run("sends DELETE request", func(t *testing.T) {
		t.Parallel()

		called := false
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			if r.Method != http.MethodDelete {
				t.Errorf("expected DELETE, got %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		_, err = client.ClusterQueues.Delete(ctx, "test-org", "cluster-1", "queue-abc")
		if err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Error("expected DELETE to be called")
		}
	})
}

func TestCmdQueuePause(t *testing.T) {
	t.Parallel()

	t.Run("sends POST to pause_dispatch with note", func(t *testing.T) {
		t.Parallel()

		queue := newTestQueue("my-queue", "queue-abc", true)
		queue.DispatchPausedNote = "maintenance"

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if !strings.HasSuffix(r.URL.Path, "/pause_dispatch") {
				t.Errorf("expected path to end in /pause_dispatch, got %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(queue)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		got, _, err := client.ClusterQueues.Pause(ctx, "test-org", "cluster-1", "queue-abc", buildkite.ClusterQueuePause{
			Note: "maintenance",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !got.DispatchPaused {
			t.Error("expected queue to be paused")
		}
		if got.DispatchPausedNote != "maintenance" {
			t.Errorf("expected note 'maintenance', got %q", got.DispatchPausedNote)
		}
	})
}

func TestCmdQueueResume(t *testing.T) {
	t.Parallel()

	t.Run("sends POST to resume_dispatch", func(t *testing.T) {
		t.Parallel()

		called := false
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if !strings.HasSuffix(r.URL.Path, "/resume_dispatch") {
				t.Errorf("expected path to end in /resume_dispatch, got %q", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		_, err = client.ClusterQueues.Resume(ctx, "test-org", "cluster-1", "queue-abc")
		if err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Error("expected resume_dispatch to be called")
		}
	})
}

func TestRenderQueueText(t *testing.T) {
	t.Parallel()

	t.Run("unpaused queue", func(t *testing.T) {
		t.Parallel()
		q := newTestQueue("my-queue", "queue-abc", false)
		out := renderQueueText(q)
		if !strings.Contains(out, "my-queue") {
			t.Error("expected output to contain queue key")
		}
		if !strings.Contains(out, "No") {
			t.Error("expected output to show paused as 'No'")
		}
	})

	t.Run("paused queue with note", func(t *testing.T) {
		t.Parallel()
		q := newTestQueue("my-queue", "queue-abc", true)
		q.DispatchPausedNote = "maintenance window"
		out := renderQueueText(q)
		if !strings.Contains(out, "Yes") {
			t.Error("expected output to show paused as 'Yes'")
		}
		if !strings.Contains(out, "maintenance window") {
			t.Error("expected output to contain pause note")
		}
	})
}
