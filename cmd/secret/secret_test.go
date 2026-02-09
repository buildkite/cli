package secret

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestListSecrets(t *testing.T) {
	t.Parallel()

	t.Run("fetches secrets through API", func(t *testing.T) {
		t.Parallel()

		secrets := []buildkite.ClusterSecret{
			{
				ID:          "secret-1",
				Key:         "MY_SECRET",
				Description: "A test secret",
			},
			{
				ID:          "secret-2",
				Key:         "ANOTHER_SECRET",
				Description: "Another test secret",
			},
		}

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if !strings.Contains(r.URL.Path, "/clusters/cluster-123/secrets") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(secrets)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.ClusterSecrets.List(context.Background(), "test-org", "cluster-123", nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 secrets, got %d", len(result))
		}

		if result[0].Key != "MY_SECRET" {
			t.Errorf("expected key 'MY_SECRET', got %q", result[0].Key)
		}

		if result[1].ID != "secret-2" {
			t.Errorf("expected ID 'secret-2', got %q", result[1].ID)
		}
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]buildkite.ClusterSecret{})
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.ClusterSecrets.List(context.Background(), "test-org", "cluster-123", nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 0 {
			t.Errorf("expected 0 secrets, got %d", len(result))
		}
	})
}

func TestGetSecret(t *testing.T) {
	t.Parallel()

	secret := buildkite.ClusterSecret{
		ID:          "secret-1",
		Key:         "MY_SECRET",
		Description: "A test secret",
		Policy:      "- pipeline_slug: my-pipeline",
	}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/secrets/secret-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(secret)
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	result, _, err := client.ClusterSecrets.Get(context.Background(), "test-org", "cluster-123", "secret-1")
	if err != nil {
		t.Fatal(err)
	}

	if result.Key != "MY_SECRET" {
		t.Errorf("expected key 'MY_SECRET', got %q", result.Key)
	}

	if result.Policy != "- pipeline_slug: my-pipeline" {
		t.Errorf("expected policy, got %q", result.Policy)
	}
}

func TestCreateSecret(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var input buildkite.ClusterSecretCreate
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}

		if input.Key != "NEW_SECRET" {
			t.Errorf("expected key 'NEW_SECRET', got %q", input.Key)
		}

		if input.Value != "s3cr3t" {
			t.Errorf("expected value 's3cr3t', got %q", input.Value)
		}

		if input.Description != "A new secret" {
			t.Errorf("expected description 'A new secret', got %q", input.Description)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildkite.ClusterSecret{
			ID:          "new-secret-id",
			Key:         input.Key,
			Description: input.Description,
		})
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	result, _, err := client.ClusterSecrets.Create(context.Background(), "test-org", "cluster-123", buildkite.ClusterSecretCreate{
		Key:         "NEW_SECRET",
		Value:       "s3cr3t",
		Description: "A new secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.ID != "new-secret-id" {
		t.Errorf("expected ID 'new-secret-id', got %q", result.ID)
	}

	if result.Key != "NEW_SECRET" {
		t.Errorf("expected key 'NEW_SECRET', got %q", result.Key)
	}
}

func TestDeleteSecret(t *testing.T) {
	t.Parallel()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/secrets/secret-to-delete") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ClusterSecrets.Delete(context.Background(), "test-org", "cluster-123", "secret-to-delete")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdateSecret(t *testing.T) {
	t.Parallel()

	t.Run("updates metadata", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PUT" {
				t.Errorf("expected PUT, got %s", r.Method)
			}

			var input buildkite.ClusterSecretUpdate
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}

			if input.Description != "Updated description" {
				t.Errorf("expected description 'Updated description', got %q", input.Description)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(buildkite.ClusterSecret{
				ID:          "secret-1",
				Key:         "MY_SECRET",
				Description: input.Description,
			})
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		result, _, err := client.ClusterSecrets.Update(context.Background(), "test-org", "cluster-123", "secret-1", buildkite.ClusterSecretUpdate{
			Description: "Updated description",
		})
		if err != nil {
			t.Fatal(err)
		}

		if result.Description != "Updated description" {
			t.Errorf("expected description 'Updated description', got %q", result.Description)
		}
	})

	t.Run("updates value", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PUT" {
				t.Errorf("expected PUT, got %s", r.Method)
			}
			if !strings.Contains(r.URL.Path, "/secrets/secret-1/value") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			var input buildkite.ClusterSecretValueUpdate
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}

			if input.Value != "new-value" {
				t.Errorf("expected value 'new-value', got %q", input.Value)
			}

			w.WriteHeader(http.StatusOK)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		_, err = client.ClusterSecrets.UpdateValue(context.Background(), "test-org", "cluster-123", "secret-1", buildkite.ClusterSecretValueUpdate{
			Value: "new-value",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
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
			cmd:     UpdateCmd{ClusterID: "c", SecretID: "s"},
			wantErr: true,
		},
		{
			name:    "only description",
			cmd:     UpdateCmd{ClusterID: "c", SecretID: "s", Description: "new desc"},
			wantErr: false,
		},
		{
			name:    "only policy",
			cmd:     UpdateCmd{ClusterID: "c", SecretID: "s", Policy: "new policy"},
			wantErr: false,
		},
		{
			name:    "only update-value",
			cmd:     UpdateCmd{ClusterID: "c", SecretID: "s", UpdateValue: true},
			wantErr: false,
		},
		{
			name:    "description and update-value",
			cmd:     UpdateCmd{ClusterID: "c", SecretID: "s", Description: "desc", UpdateValue: true},
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

func TestRenderSecretText(t *testing.T) {
	t.Parallel()

	secret := buildkite.ClusterSecret{
		ID:          "secret-123",
		Key:         "MY_SECRET",
		Description: "Test description",
		Policy:      "- pipeline_slug: test",
		CreatedBy: buildkite.SecretCreator{
			ID:   "user-1",
			Name: "Test User",
		},
	}

	result := renderSecretText(secret)

	expectedStrings := []string{
		"Viewing secret MY_SECRET",
		"secret-123",
		"Test description",
		"- pipeline_slug: test",
		"Test User",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, result)
		}
	}
}
