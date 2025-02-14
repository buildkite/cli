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
	"github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/afero"
)

func TestCmdAgentStop(t *testing.T) {
	t.Parallel()

	t.Run("it reports an error when no agents supplied", func(t *testing.T) {
		t.Parallel()

		factory := &factory.Factory{}
		cmd := agent.NewCmdAgentStop(factory)

		err := cmd.Execute()

		got := err.Error()
		want := "Must supply agents to stop."
		if !strings.Contains(got, want) {
			t.Errorf("Output error did not contain expected string. %s != %s", got, want)
		}
	})

	t.Run("it handles invalid agents passed as arguments", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("test")

		factory := &factory.Factory{
			Config:        conf,
			RestAPIClient: apiClient,
		}

		cmd := agent.NewCmdAgentStop(factory)
		cmd.SetArgs([]string{"test agent", "", "  "})

		// capture the output to assert
		var b bytes.Buffer
		cmd.SetOut(&b)

		err = cmd.Execute()
		if err != nil {
			t.Error(err)
		}

		got := b.String()
		want := "Stopped agent test agent"
		if !strings.Contains(got, want) {
			t.Errorf("Output error did not contain expected string. %s != %s", got, want)
		}
	})

	t.Run("it can read agents from input", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("test")

		factory := &factory.Factory{
			Config:        conf,
			RestAPIClient: apiClient,
		}

		// create a command using the stubbed factory
		cmd := agent.NewCmdAgentStop(factory)

		// inject input to the command
		input := strings.NewReader(`test agent`)
		cmd.SetIn(input)
		// capture the output to assert
		var b bytes.Buffer
		cmd.SetOut(&b)

		err = cmd.Execute()
		if err != nil {
			t.Error(err)
		}

		if v := b.Bytes(); !bytes.Contains(v, []byte("Stopped agent test agent")) {
			t.Errorf("%s", v)
		}
	})

	t.Run("it handles invalid agent ids passed as input", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("test")

		factory := &factory.Factory{
			Config:        conf,
			RestAPIClient: apiClient,
		}

		// create a command using the stubbed factory
		cmd := agent.NewCmdAgentStop(factory)

		// inject input to the command
		input := strings.NewReader("test agent\n\nanother agent")
		cmd.SetIn(input)
		// capture the output to assert
		var b bytes.Buffer
		cmd.SetOut(&b)

		err = cmd.Execute()
		if err != nil {
			t.Error(err)
		}

		if v := b.Bytes(); !bytes.Contains(v, []byte("Stopped agent test agent")) {
			t.Errorf("%s", v)
		}
		if v := b.Bytes(); !bytes.Contains(v, []byte("Stopped agent another agent")) {
			t.Errorf("%s", v)
		}
	})

	t.Run("it returns an error if any agents fail", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("test")

		factory := &factory.Factory{
			Config:        conf,
			RestAPIClient: apiClient,
		}

		// create a command using the stubbed factory
		cmd := agent.NewCmdAgentStop(factory)

		// inject input to the command
		input := strings.NewReader(`test agent`)
		cmd.SetIn(input)
		// capture the output to assert
		var b bytes.Buffer
		cmd.SetOut(&b)

		err = cmd.Execute()
		if err == nil {
			t.Error("Expected to return an error")
		}

		if v := b.Bytes(); !bytes.Contains(v, []byte("Failed to stop agent test agent")) {
			t.Errorf("%s", v)
		}
	})
}
