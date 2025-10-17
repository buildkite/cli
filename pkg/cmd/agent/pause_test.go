package agent_test

import (
	"bytes"
	"io"
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
			if r.Method == "PUT" && strings.Contains(r.URL.Path, "/agents/123/pause") {
				w.WriteHeader(http.StatusOK)
			} else {
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

	t.Run("it validates negative timeout", func(t *testing.T) {
		t.Parallel()

		factory := &factory.Factory{}
		cmd := agent.NewCmdAgentPause(factory)
		cmd.SetArgs([]string{"123", "--timeout-in-minutes", "-1"})

		err := cmd.Execute()
		if err == nil {
			t.Error("Expected validation error for negative timeout")
		}

		got := err.Error()
		want := "timeout-in-minutes must be 1 or more"
		if !strings.Contains(got, want) {
			t.Errorf("Expected error message %q, got %q", want, got)
		}
	})

	t.Run("it validates excessively large timeout", func(t *testing.T) {
		t.Parallel()

		factory := &factory.Factory{}
		cmd := agent.NewCmdAgentPause(factory)
		cmd.SetArgs([]string{"123", "--timeout-in-minutes", "1000000"})

		err := cmd.Execute()
		if err == nil {
			t.Error("Expected validation error for large timeout")
		}

		got := err.Error()
		want := "timeout-in-minutes cannot exceed"
		if !strings.Contains(got, want) {
			t.Errorf("Expected error message containing %q, got %q", want, got)
		}
	})

	t.Run("it handles pause with note and timeout", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "PUT" && strings.Contains(r.URL.Path, "/agents/123/pause") {
				// Verify the request body contains note and timeout
				body, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(body), `"note":"test note"`) ||
					!strings.Contains(string(body), `"timeout_in_minutes":60`) {
					t.Errorf("Request body missing expected fields: %s", string(body))
				}
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
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
			Config:        conf,
			RestAPIClient: apiClient,
		}

		cmd := agent.NewCmdAgentPause(factory)
		cmd.SetArgs([]string{"123", "--note", "test note", "--timeout-in-minutes", "60"})

		var b bytes.Buffer
		cmd.SetOut(&b)

		err = cmd.Execute()
		if err != nil {
			t.Error(err)
		}

		got := b.String()
		want := "Agent 123 paused successfully with note: test note (auto-resume in 60 minutes)"
		if !strings.Contains(got, want) {
			t.Errorf("Expected output %q, got %q", want, got)
		}
	})
}
