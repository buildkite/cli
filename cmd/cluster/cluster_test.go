package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestListClusters(t *testing.T) {
	t.Parallel()

	t.Run("fetches clusters through API", func(t *testing.T) {
		t.Parallel()

		clusters := []buildkite.Cluster{
			{
				ID:          "cluster-1",
				Name:        "Production",
				Description: "Production cluster",
			},
			{
				ID:          "cluster-2",
				Name:        "Staging",
				Description: "Staging cluster",
			},
		}

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if !strings.Contains(r.URL.Path, "/clusters") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(clusters)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.Clusters.List(context.Background(), "test-org", nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 clusters, got %d", len(result))
		}

		if result[0].Name != "Production" {
			t.Errorf("expected name 'Production', got %q", result[0].Name)
		}

		if result[1].ID != "cluster-2" {
			t.Errorf("expected ID 'cluster-2', got %q", result[1].ID)
		}
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]buildkite.Cluster{})
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.Clusters.List(context.Background(), "test-org", nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 0 {
			t.Errorf("expected 0 clusters, got %d", len(result))
		}
	})
}

func TestGetCluster(t *testing.T) {
	t.Parallel()

	cluster := buildkite.Cluster{
		ID:          "cluster-1",
		Name:        "Production",
		Description: "Production cluster",
		Color:       "#FF0000",
		Emoji:       ":rocket:",
	}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/clusters/cluster-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cluster)
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	result, _, err := client.Clusters.Get(context.Background(), "test-org", "cluster-1")
	if err != nil {
		t.Fatal(err)
	}

	if result.Name != "Production" {
		t.Errorf("expected name 'Production', got %q", result.Name)
	}

	if result.Color != "#FF0000" {
		t.Errorf("expected color '#FF0000', got %q", result.Color)
	}
}

func TestCreateCluster(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/clusters") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var input buildkite.ClusterCreate
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}

		if input.Name != "New Cluster" {
			t.Errorf("expected name 'New Cluster', got %q", input.Name)
		}

		if input.Description != "A brand new cluster" {
			t.Errorf("expected description 'A brand new cluster', got %q", input.Description)
		}

		if input.Color != "#FF0000" {
			t.Errorf("expected color '#FF0000', got %q", input.Color)
		}

		if input.Emoji != ":rocket:" {
			t.Errorf("expected emoji ':rocket:', got %q", input.Emoji)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(buildkite.Cluster{
			ID:          "new-cluster-id",
			Name:        input.Name,
			Description: input.Description,
			Color:       input.Color,
			Emoji:       input.Emoji,
		})
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	result, _, err := client.Clusters.Create(context.Background(), "test-org", buildkite.ClusterCreate{
		Name:        "New Cluster",
		Description: "A brand new cluster",
		Color:       "#FF0000",
		Emoji:       ":rocket:",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.ID != "new-cluster-id" {
		t.Errorf("expected ID 'new-cluster-id', got %q", result.ID)
	}

	if result.Name != "New Cluster" {
		t.Errorf("expected name 'New Cluster', got %q", result.Name)
	}
}

func TestUpdateCluster(t *testing.T) {
	t.Parallel()

	t.Run("updates cluster metadata", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PATCH" {
				t.Errorf("expected PATCH, got %s", r.Method)
			}
			if !strings.Contains(r.URL.Path, "/clusters/cluster-1") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			var input buildkite.ClusterUpdate
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}

			if input.Name != "Updated Name" {
				t.Errorf("expected name 'Updated Name', got %q", input.Name)
			}

			if input.Description != "Updated description" {
				t.Errorf("expected description 'Updated description', got %q", input.Description)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(buildkite.Cluster{
				ID:          "cluster-1",
				Name:        input.Name,
				Description: input.Description,
			})
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.Clusters.Update(context.Background(), "test-org", "cluster-1", buildkite.ClusterUpdate{
			Name:        "Updated Name",
			Description: "Updated description",
		})
		if err != nil {
			t.Fatal(err)
		}

		if result.Name != "Updated Name" {
			t.Errorf("expected name 'Updated Name', got %q", result.Name)
		}

		if result.Description != "Updated description" {
			t.Errorf("expected description 'Updated description', got %q", result.Description)
		}
	})

	t.Run("updates default queue", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PATCH" {
				t.Errorf("expected PATCH, got %s", r.Method)
			}

			var input buildkite.ClusterUpdate
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}

			if input.DefaultQueueID != "queue-123" {
				t.Errorf("expected default_queue_id 'queue-123', got %q", input.DefaultQueueID)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(buildkite.Cluster{
				ID:             "cluster-1",
				Name:           "Production",
				DefaultQueueID: input.DefaultQueueID,
			})
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.Clusters.Update(context.Background(), "test-org", "cluster-1", buildkite.ClusterUpdate{
			DefaultQueueID: "queue-123",
		})
		if err != nil {
			t.Fatal(err)
		}

		if result.DefaultQueueID != "queue-123" {
			t.Errorf("expected default_queue_id 'queue-123', got %q", result.DefaultQueueID)
		}
	})
}

func TestDeleteCluster(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/clusters/cluster-to-delete") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Clusters.Delete(context.Background(), "test-org", "cluster-to-delete")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateCmdValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     UpdateCmd
		wantErr bool
	}{
		{
			name:    "no flags set",
			cmd:     UpdateCmd{ClusterUUID: "cluster-1"},
			wantErr: true,
		},
		{
			name:    "only name",
			cmd:     UpdateCmd{ClusterUUID: "cluster-1", Name: "New Name"},
			wantErr: false,
		},
		{
			name:    "only description",
			cmd:     UpdateCmd{ClusterUUID: "cluster-1", Description: "New description"},
			wantErr: false,
		},
		{
			name:    "only emoji",
			cmd:     UpdateCmd{ClusterUUID: "cluster-1", Emoji: ":rocket:"},
			wantErr: false,
		},
		{
			name:    "only color",
			cmd:     UpdateCmd{ClusterUUID: "cluster-1", Color: "#FF0000"},
			wantErr: false,
		},
		{
			name:    "only default-queue-uuid",
			cmd:     UpdateCmd{ClusterUUID: "cluster-1", DefaultQueueUUID: "queue-123"},
			wantErr: false,
		},
		{
			name:    "multiple fields",
			cmd:     UpdateCmd{ClusterUUID: "cluster-1", Name: "New Name", Description: "New desc", Color: "#00FF00"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRenderClusterText(t *testing.T) {
	t.Parallel()

	ts := buildkite.Timestamp{}
	cluster := buildkite.Cluster{
		ID:          "cluster-123",
		GraphQLID:   "graphql-123",
		Name:        "Production",
		Description: "Production cluster",
		Color:       "#FF0000",
		Emoji:       ":rocket:",
		WebURL:      "https://buildkite.com/orgs/test-org/clusters/cluster-123",
		CreatedBy: buildkite.ClusterCreator{
			ID:   "user-1",
			Name: "Test User",
		},
		CreatedAt: &ts,
	}

	result := renderClusterText(cluster)

	expectedStrings := []string{
		"Viewing Production",
		"cluster-123",
		"Production cluster",
		"#FF0000",
		":rocket:",
		"Test User",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, result)
		}
	}
}
