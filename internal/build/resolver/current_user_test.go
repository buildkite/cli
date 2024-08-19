package resolver_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/spf13/afero"
)

func TestResolveBuildForCurrentUser(t *testing.T) {
	t.Parallel()

	pipelineResolver := func(context.Context) (*pipeline.Pipeline, error) {
		return &pipeline.Pipeline{
			Name: "testing",
			Org:  "test org",
		}, nil
	}

	t.Run("Errors if user cannot be found", func(t *testing.T) {
		t.Parallel()

		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(s.Close)

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		// mock a failed repsonse
		f := &factory.Factory{RestAPIClient: apiClient}

		r := resolver.ResolveBuildForCurrentUser("main", pipelineResolver, f)
		_, err = r(context.Background())
		if err == nil {
			t.Fatal("Resolver should return error if user not found")
		}
	})

	t.Run("Returns first build found", func(t *testing.T) {
		t.Parallel()

		in, _ := os.ReadFile("../../../fixtures/build.json")
		callIndex := 0
		responses := []http.Response{
			{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"id": "abc123-4567-8910-...",
					"graphql_id": "VXNlci0tLWU1N2ZiYTBmLWFiMTQtNGNjMC1iYjViLTY5NTc3NGZmYmZiZQ==",
					"name": "John Smith",
					"email": "john.smith@example.com",
					"avatar_url": "https://www.gravatar.com/avatar/abc123...",
					"created_at": "2012-03-04T06:07:08.910Z"
					}
				`)),
			},
			{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(in)),
			},
		}

		// mock a failed repsonse
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := responses[callIndex]
			callIndex++
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		}))
		t.Cleanup(s.Close)

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		fs := afero.NewMemMapFs()
		f := &factory.Factory{
			RestAPIClient: apiClient,
			Config:        config.New(fs, nil),
		}

		r := resolver.ResolveBuildForCurrentUser("main", pipelineResolver, f)
		build, err := r(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		if build.BuildNumber != 584 {
			t.Fatalf("Expected build 584, got %d", build.BuildNumber)
		}
	})

	t.Run("Errors if no matching builds found", func(t *testing.T) {
		t.Parallel()

		callIndex := 0
		responses := []http.Response{
			{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"id": "abc123-4567-8910-...",
					"graphql_id": "VXNlci0tLWU1N2ZiYTBmLWFiMTQtNGNjMC1iYjViLTY5NTc3NGZmYmZiZQ==",
					"name": "John Smith",
					"email": "john.smith@example.com",
					"avatar_url": "https://www.gravatar.com/avatar/abc123...",
					"created_at": "2012-03-04T06:07:08.910Z"
					}
				`)),
			},
			{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("[]")),
			},
		}

		// mock a failed repsonse
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := responses[callIndex]
			callIndex++
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		}))
		t.Cleanup(s.Close)

		apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}

		fs := afero.NewMemMapFs()
		f := &factory.Factory{
			RestAPIClient: apiClient,
			Config:        config.New(fs, nil),
		}

		r := resolver.ResolveBuildForCurrentUser("main", pipelineResolver, f)
		build, err := r(context.Background())

		if err == nil {
			t.Fatal("Should return an error when no build is found")
		}

		if build != nil {
			t.Fatal("Expected no build to be found")
		}
	})
}
