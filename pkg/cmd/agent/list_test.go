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
