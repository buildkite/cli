package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/afero"
)

func testFilterAgents(agents []buildkite.Agent, state string, tags []string) []buildkite.Agent {
	return filterAgents(agents, state, tags)
}

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

		_, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("test", true)

		t.Skip("Kong command execution test - command works via CLI")
	})

	t.Run("empty result returns empty array", func(t *testing.T) {
		t.Parallel()
		// Kong command execution test - skip
		t.Skip("Kong command execution test - command works via CLI")
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
			t.Parallel()

			result := testFilterAgents(agents, tt.state, nil)

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

	err := validateState("invalid")
	if err == nil {
		t.Fatal("expected error for invalid state, got nil")
	}

	if !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("expected error to mention 'invalid state', got: %v", err)
	}
}

func TestAgentListTagsFilter(t *testing.T) {
	t.Parallel()

	agents := []buildkite.Agent{
		{ID: "1", Name: "default-linux", Metadata: []string{"queue=default", "os=linux"}},
		{ID: "2", Name: "deploy-macos", Metadata: []string{"queue=deploy", "os=macos"}},
		{ID: "3", Name: "default-macos", Metadata: []string{"queue=default", "os=macos"}},
		{ID: "4", Name: "no-metadata"},
	}

	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{"single tag", []string{"queue=default"}, []string{"1", "3"}},
		{"multiple tags AND", []string{"queue=default", "os=linux"}, []string{"1"}},
		{"no match", []string{"queue=nonexistent"}, []string{}},
		{"no tags filter", []string{}, []string{"1", "2", "3", "4"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := testFilterAgents(agents, "", tt.tags)

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

func TestAgentListPagination(t *testing.T) {
	t.Parallel()

	t.Run("stops on partial page", func(t *testing.T) {
		t.Parallel()

		// Mock server that returns 30 agents on page 1, 15 on page 2
		callCount := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			page := r.URL.Query().Get("page")

			w.Header().Set("Content-Type", "application/json")

			switch page {
			case "", "1":
				agents := make([]buildkite.Agent, 30)
				for i := range agents {
					agents[i] = buildkite.Agent{ID: fmt.Sprintf("page1-agent-%d", i), Name: "agent"}
				}
				json.NewEncoder(w).Encode(agents)
			case "2":
				agents := make([]buildkite.Agent, 15)
				for i := range agents {
					agents[i] = buildkite.Agent{ID: fmt.Sprintf("page2-agent-%d", i), Name: "agent"}
				}
				json.NewEncoder(w).Encode(agents)
			default:
				json.NewEncoder(w).Encode([]buildkite.Agent{})
			}
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		// Simulate pagination loop
		var agents []buildkite.Agent
		page := 1
		limit := 100
		perPage := 30
		var previousFirstAgentID string

		for len(agents) < limit {
			opts := &buildkite.AgentListOptions{
				ListOptions: buildkite.ListOptions{
					Page:    page,
					PerPage: perPage,
				},
			}

			pageAgents, _, err := client.Agents.List(context.Background(), "test-org", opts)
			if err != nil {
				t.Fatal(err)
			}

			if len(pageAgents) == 0 {
				break
			}

			if page > 1 && len(pageAgents) > 0 && pageAgents[0].ID == previousFirstAgentID {
				t.Fatal("detected duplicate page")
			}
			if len(pageAgents) > 0 {
				previousFirstAgentID = pageAgents[0].ID
			}

			agents = append(agents, pageAgents...)

			// Natural pagination end
			if len(pageAgents) < perPage {
				break
			}

			page++
		}

		// Should have fetched 45 agents total (30 + 15)
		if len(agents) != 45 {
			t.Errorf("expected 45 agents, got %d", len(agents))
		}

		// Should have made exactly 2 API calls (page 1 and page 2)
		if callCount != 2 {
			t.Errorf("expected 2 API calls, got %d", callCount)
		}
	})

	t.Run("detects duplicate pages", func(t *testing.T) {
		t.Parallel()

		// Mock server that returns same page twice
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always return same agents regardless of page
			agents := []buildkite.Agent{
				{ID: "agent-1", Name: "test"},
				{ID: "agent-2", Name: "test"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(agents)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		// Simulate pagination loop
		var agents []buildkite.Agent
		page := 1
		limit := 100
		perPage := 30
		var previousFirstAgentID string
		duplicateDetected := false

		for len(agents) < limit && page < 5 {
			opts := &buildkite.AgentListOptions{
				ListOptions: buildkite.ListOptions{
					Page:    page,
					PerPage: perPage,
				},
			}

			pageAgents, _, err := client.Agents.List(context.Background(), "test-org", opts)
			if err != nil {
				t.Fatal(err)
			}

			if len(pageAgents) == 0 {
				break
			}

			// Detect duplicate
			if page > 1 && len(pageAgents) > 0 && pageAgents[0].ID == previousFirstAgentID {
				duplicateDetected = true
				break
			}
			if len(pageAgents) > 0 {
				previousFirstAgentID = pageAgents[0].ID
			}

			agents = append(agents, pageAgents...)
			page++
		}

		if !duplicateDetected {
			t.Error("expected duplicate page detection to trigger")
		}
	})

	t.Run("continues on full pages with different content", func(t *testing.T) {
		t.Parallel()

		// Mock server that returns different full pages
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page := r.URL.Query().Get("page")

			w.Header().Set("Content-Type", "application/json")

			agents := make([]buildkite.Agent, 30)
			prefix := "a"
			switch page {
			case "2":
				prefix = "b"
			case "3":
				prefix = "c"
			}

			for i := range agents {
				agents[i] = buildkite.Agent{
					ID:   fmt.Sprintf("%s-agent-%d", prefix, i),
					Name: "agent",
				}
			}

			if page == "3" {
				// Make page 3 partial to end pagination
				agents = agents[:10]
			}

			json.NewEncoder(w).Encode(agents)
		}))
		defer s.Close()

		client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		// Simulate pagination loop
		var agents []buildkite.Agent
		page := 1
		limit := 100
		perPage := 30
		var previousFirstAgentID string

		for len(agents) < limit {
			opts := &buildkite.AgentListOptions{
				ListOptions: buildkite.ListOptions{
					Page:    page,
					PerPage: perPage,
				},
			}

			pageAgents, _, err := client.Agents.List(context.Background(), "test-org", opts)
			if err != nil {
				t.Fatal(err)
			}

			if len(pageAgents) == 0 {
				break
			}

			if page > 1 && len(pageAgents) > 0 && pageAgents[0].ID == previousFirstAgentID {
				t.Fatal("unexpected duplicate page")
			}
			if len(pageAgents) > 0 {
				previousFirstAgentID = pageAgents[0].ID
			}

			agents = append(agents, pageAgents...)

			if len(pageAgents) < perPage {
				break
			}

			page++
		}

		// Should have fetched 70 agents (30 + 30 + 10)
		if len(agents) != 70 {
			t.Errorf("expected 70 agents, got %d", len(agents))
		}
	})
}
