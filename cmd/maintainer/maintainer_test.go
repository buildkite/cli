package maintainer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestListMaintainers(t *testing.T) {
	t.Parallel()

	t.Run("fetches maintainers through API", func(t *testing.T) {
		t.Parallel()

		maintainers := []buildkite.ClusterMaintainerEntry{
			{
				ID: "maintainer-1",
				Actor: buildkite.ClusterMaintainerActor{
					Type: "user",
					Name: "Jurgen Klopp",
				},
			},
			{
				ID: "maintainer-2",
				Actor: buildkite.ClusterMaintainerActor{
					Type: "team",
					Slug: "platform",
				},
			},
		}

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if !strings.Contains(r.URL.Path, "/clusters/cluster-123/maintainers") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(maintainers)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.ClusterMaintainers.List(context.Background(), "test-org", "cluster-123", nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 maintainers, got %d", len(result))
		}

		if result[0].Actor.Name != "Jurgen Klopp" {
			t.Errorf("expected name 'Jurgen Klopp', got %q", result[0].Actor.Name)
		}

		if result[1].Actor.Slug != "platform" {
			t.Errorf("expected slug 'platform', got %q", result[1].Actor.Slug)
		}
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]buildkite.ClusterMaintainerEntry{})
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.ClusterMaintainers.List(context.Background(), "test-org", "cluster-123", nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 0 {
			t.Errorf("expected 0 maintainers, got %d", len(result))
		}
	})
}

func TestCreateMaintainer(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/clusters/cluster-123/maintainers") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var input buildkite.ClusterMaintainer
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}

		if input.UserID != "user-123" {
			t.Errorf("expected user id 'user-123', got %q", input.UserID)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(buildkite.ClusterMaintainerEntry{
			ID: "maintainer-123",
			Actor: buildkite.ClusterMaintainerActor{
				Type: "user",
				Name: "Jurgen Klopp",
			},
		})
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	result, _, err := client.ClusterMaintainers.Create(context.Background(), "test-org", "cluster-123", buildkite.ClusterMaintainer{UserID: "user-123"})
	if err != nil {
		t.Fatal(err)
	}

	if result.ID != "maintainer-123" {
		t.Errorf("expected ID 'maintainer-123', got %q", result.ID)
	}

	if result.Actor.Type != "user" {
		t.Errorf("expected actor type 'user', got %q", result.Actor.Type)
	}
}

func TestDeleteMaintainer(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/maintainers/maintainer-123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ClusterMaintainers.Delete(context.Background(), "test-org", "cluster-123", "maintainer-123")
	if err != nil {
		t.Fatal(err)
	}
}
