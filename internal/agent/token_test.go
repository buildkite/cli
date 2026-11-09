package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

func TestWriteAgentConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "subdir", "buildkite-agent.cfg")

	err := WriteAgentConfig(configPath, "test-token-123", "/tmp/builds", []string{"queue=default"})
	if err != nil {
		t.Fatalf("WriteAgentConfig() error: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	expected := "token=\"test-token-123\"\nbuild-path=\"/tmp/builds\"\ntags=\"queue=default\"\n"
	if string(content) != expected {
		t.Errorf("config content = %q, want %q", content, expected)
	}

	// Verify file permissions are restrictive
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config permissions = %o, want 600", perm)
	}
}

func TestWriteAgentConfig_CreatesParentDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "a", "b", "c", "buildkite-agent.cfg")

	err := WriteAgentConfig(configPath, "token", "/builds", nil)
	if err != nil {
		t.Fatalf("WriteAgentConfig() error: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}

func TestFindCluster(t *testing.T) {
	t.Parallel()
	clusters := []buildkite.Cluster{
		{ID: "z-id", Name: "Staging"},
		{ID: "default-id", Name: "Default"},
		{ID: "a-id", Name: "AWS"},
	}
	tests := []struct {
		name, uuid, clusterName string
		clusters                []buildkite.Cluster
		status                  int
		wantID, wantErr         string
		noLookup                bool
	}{
		{name: "UUID bypasses lookup", uuid: "explicit-id", wantID: "explicit-id", noLookup: true},
		{name: "conflicting selectors", uuid: "explicit-id", clusterName: "AWS", wantErr: "only one of --cluster-uuid or --cluster-name", noLookup: true},
		{name: "Default fallback", clusters: clusters, wantID: "default-id"},
		{name: "explicit name overrides Default", clusters: clusters, clusterName: "AWS", wantID: "a-id"},
		{name: "name with spaces", clusters: []buildkite.Cluster{{ID: "id", Name: "Default cluster"}}, clusterName: "Default cluster", wantID: "id"},
		{name: "missing Default", clusters: []buildkite.Cluster{{ID: "id", Name: "Default cluster"}}, wantErr: `no cluster named "Default"`},
		{name: "missing name", clusters: clusters, clusterName: "Missing", wantErr: `no cluster named "Missing"`},
		{name: "case sensitive", clusters: clusters, clusterName: "default", wantErr: `no cluster named "default"`},
		{name: "no clusters", wantErr: `no accessible clusters found in organization "test-org"`},
		{name: "API failure", status: http.StatusForbidden, wantErr: "listing clusters", clusters: clusters},
		{name: "duplicate names", clusters: []buildkite.Cluster{{ID: "2", Name: "AWS"}, {ID: "1", Name: "AWS"}}, clusterName: "AWS", wantErr: `multiple clusters named "AWS"`},
		{name: "duplicate Defaults", clusters: []buildkite.Cluster{{ID: "2", Name: "Default"}, {ID: "1", Name: "Default"}}, wantErr: `multiple clusters named "Default"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v2/organizations/test-org/clusters" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if tt.status != 0 {
					w.WriteHeader(tt.status)
					fmt.Fprint(w, `{"message":"Forbidden"}`)
					return
				}
				json.NewEncoder(w).Encode(tt.clusters)
			}))
			defer server.Close()
			client, err := buildkite.NewOpts(buildkite.WithBaseURL(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			got, err := FindCluster(context.Background(), &factory.Factory{RestAPIClient: client}, "test-org", tt.uuid, tt.clusterName)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				if len(tt.clusters) > 0 && tt.status == 0 {
					for _, c := range tt.clusters {
						if !strings.Contains(err.Error(), c.Name+"  "+c.ID) {
							t.Errorf("error does not list cluster %q (%s): %v", c.Name, c.ID, err)
						}
					}
					if !strings.Contains(err.Error(), "--cluster-uuid <uuid>") {
						t.Errorf("missing retry instructions: %v", err)
					}
				}
			} else if err != nil || got != tt.wantID {
				t.Fatalf("FindCluster() = %q, %v; want %q, nil", got, err, tt.wantID)
			}
			if tt.noLookup && requests.Load() != 0 {
				t.Errorf("made %d unexpected requests", requests.Load())
			}
			if tt.name == "missing name" {
				if strings.Index(err.Error(), "AWS  a-id") > strings.Index(err.Error(), "Staging  z-id") {
					t.Errorf("cluster list not sorted: %v", err)
				}
			}
		})
	}
}

func TestFindClusterPagination(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "Production", "Missing"} {
		t.Run("selector="+name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Query().Get("page") {
				case "1":
					w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, r.URL.Path))
					fmt.Fprint(w, `[{"id":"aws-id","name":"AWS"}]`)
				case "2":
					fmt.Fprint(w, `[{"id":"default-id","name":"Default"},{"id":"prod-id","name":"Production"}]`)
				default:
					t.Errorf("unexpected page: %s", r.URL.RawQuery)
					fmt.Fprint(w, `[]`)
				}
			}))
			defer server.Close()
			client, err := buildkite.NewOpts(buildkite.WithBaseURL(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			id, err := FindCluster(context.Background(), &factory.Factory{RestAPIClient: client}, "test-org", "", name)
			switch name {
			case "":
				if err != nil || id != "default-id" {
					t.Fatalf("got %q, %v", id, err)
				}
			case "Production":
				if err != nil || id != "prod-id" {
					t.Fatalf("got %q, %v", id, err)
				}
			case "Missing":
				if err == nil {
					t.Fatal("expected error")
				}
				for _, cluster := range []string{"AWS  aws-id", "Default  default-id", "Production  prod-id"} {
					if !strings.Contains(err.Error(), cluster) {
						t.Errorf("missing %s in error: %v", cluster, err)
					}
				}
			}
			if requests.Load() != 2 {
				t.Errorf("requests = %d, want 2", requests.Load())
			}
		})
	}
}
