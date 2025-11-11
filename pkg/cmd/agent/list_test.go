package agent_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/agent"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/afero"
)

func TestCmdAgentList(t *testing.T) {
	t.Parallel()

	t.Run("returns agents as JSON", func(t *testing.T) {
		t.Parallel()

		agents := []buildkite.Agent{
			{ID: "123", Name: "my-agent"},
			{ID: "456", Name: "another-agent"},
		}

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page := r.URL.Query().Get("page")
			if page == "" || page == "1" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(agents)
			} else {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]buildkite.Agent{})
			}
		}))
		defer s.Close()

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("test", true)

		factory := &factory.Factory{
			RestAPIClient: apiClient,
			Config:        conf,
		}

		cmd := agent.NewCmdAgentList(factory)
		cmd.SetArgs([]string{"-o", "json"})

		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = cmd.Execute()
		if err != nil {
			t.Fatal(err)
		}

		var result []buildkite.Agent
		err = json.Unmarshal(buf.Bytes(), &result)
		if err != nil {
			t.Fatal(err)
		}

		if len(result) != 2 {
			t.Errorf("got %d agents, want 2", len(result))
		}

		if result[0].Name != "my-agent" {
			t.Errorf("got agent name %q, want %q", result[0].Name, "my-agent")
		}
	})

	t.Run("empty result returns empty array", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		}))
		defer s.Close()

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("test", true)

		factory := &factory.Factory{
			RestAPIClient: apiClient,
			Config:        conf,
		}

		cmd := agent.NewCmdAgentList(factory)
		cmd.SetArgs([]string{"-o", "json"})

		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = cmd.Execute()
		if err != nil {
			t.Fatal(err)
		}

		got := strings.TrimSpace(buf.String())
		if got != "[]" {
			t.Errorf("got %q, want %q", got, "[]")
		}
	})
}

func TestAgentListStateFilter(t *testing.T) {
	t.Parallel()

	paused := true
	notPaused := false

	agents := []buildkite.Agent{
		{ID: "1", Name: "running-agent", Job: &buildkite.Job{ID: "job-1"}},
		{ID: "2", Name: "idle-agent"},
		{ID: "3", Name: "paused-agent", Paused: &paused},
		{ID: "4", Name: "idle-not-paused", Paused: &notPaused},
	}

	tests := []struct {
		state string
		want  []string // agent IDs
	}{
		{"running", []string{"1"}},
		{"RUNNING", []string{"1"}},
		{"idle", []string{"2", "4"}},
		{"paused", []string{"3"}},
		{"", []string{"1", "2", "3", "4"}},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				page := r.URL.Query().Get("page")
				if page == "" || page == "1" {
					json.NewEncoder(w).Encode(agents)
				} else {
					json.NewEncoder(w).Encode([]buildkite.Agent{})
				}
			}))
			defer s.Close()

			apiClient, _ := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
			conf := config.New(afero.NewMemMapFs(), nil)
			conf.SelectOrganization("test", true)

			factory := &factory.Factory{
				RestAPIClient: apiClient,
				Config:        conf,
			}

			cmd := agent.NewCmdAgentList(factory)
			args := []string{"-o", "json"}
			if tt.state != "" {
				args = append(args, "--state", tt.state)
			}
			cmd.SetArgs(args)

			var buf bytes.Buffer
			cmd.SetOut(&buf)

			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}

			var result []buildkite.Agent
			if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
				t.Fatal(err)
			}

			if len(result) != len(tt.want) {
				t.Errorf("got %d agents, want %d", len(result), len(tt.want))
			}

			for i, id := range tt.want {
				if i >= len(result) || result[i].ID != id {
					t.Errorf("agent %d: got ID %q, want %q", i, result[i].ID, id)
				}
			}
		})
	}
}

func TestAgentListInvalidState(t *testing.T) {
	t.Parallel()

	conf := config.New(afero.NewMemMapFs(), nil)
	conf.SelectOrganization("test", true)

	factory := &factory.Factory{
		Config: conf,
	}

	cmd := agent.NewCmdAgentList(factory)
	cmd.SetArgs([]string{"--state", "invalid"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid state, got nil")
	}

	if !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("expected error to mention 'invalid state', got: %v", err)
	}
}
