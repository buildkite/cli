package agent_test

import (
	"bytes"
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

func TestCmdAgentPause(t *testing.T) {
	t.Parallel()

	t.Run("it reports an error when no agent supplied", func(t *testing.T) {
		t.Parallel()

		factory := &factory.Factory{}
		cmd := agent.NewCmdAgentPause(factory)

		err := cmd.Execute()

		got := err.Error()
		want := "accepts 1 arg"
		if !strings.Contains(got, want) {
			t.Errorf("Output error did not contain expected string. %s != %s", got, want)
		}
	})

	t.Run("it handles successful pause", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/organizations/test/agents/123/pause" && r.Method == "PUT" {
				w.WriteHeader(http.StatusOK)
			} else {
				t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("test", true)

		factory := &factory.Factory{
			Config:        conf,
			RestAPIClient: apiClient,
		}

		cmd := agent.NewCmdAgentPause(factory)
		cmd.SetArgs([]string{"123"})

		var b bytes.Buffer
		cmd.SetOut(&b)

		err = cmd.Execute()
		if err != nil {
			t.Error(err)
		}

		got := b.String()
		want := "Agent 123 paused successfully"
		if !strings.Contains(got, want) {
			t.Errorf("Output error did not contain expected string. %s != %s", got, want)
		}
	})

	t.Run("it handles API error", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("test", true)

		factory := &factory.Factory{
			Config:        conf,
			RestAPIClient: apiClient,
		}

		cmd := agent.NewCmdAgentPause(factory)
		cmd.SetArgs([]string{"123"})

		var b bytes.Buffer
		cmd.SetOut(&b)

		err = cmd.Execute()
		if err == nil {
			t.Error("Expected to return an error")
		}

		got := err.Error()
		want := "failed to pause agent"
		if !strings.Contains(got, want) {
			t.Errorf("Output error did not contain expected string. %s != %s", got, want)
		}
	})
}
