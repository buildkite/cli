package resolver_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
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

	nilPipelineResolver := func(context.Context) (*pipeline.Pipeline, error) {
		return nil, nil
	}
	errorPipelineResolver := func(context.Context) (*pipeline.Pipeline, error) {
		return nil, errors.New("")
	}
	pipelineResolver := func(context.Context) (*pipeline.Pipeline, error) {
		return &pipeline.Pipeline{
			Name: "testing",
			Org:  "test org",
		}, nil
	}

	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(``)),
		}, nil
	})
	client := &http.Client{Transport: transport}
	f := &factory.Factory{
		RestAPIClient: buildkite.NewClient(client),
	}

	t.Run("Errors if pipeline cannot be resolved", func(t *testing.T) {
		t.Parallel()

		r := resolver.ResolveBuildForCurrentUser("main", nilPipelineResolver, f)
		_, err := r(context.Background())

		if err == nil {
			t.Fatal("Resolver should return error if no pipeline resolved")
		}
	})

	t.Run("Errors if pipeline resolver errors", func(t *testing.T) {
		t.Parallel()

		r := resolver.ResolveBuildForCurrentUser("main", errorPipelineResolver, f)
		_, err := r(context.Background())

		if err == nil {
			t.Fatal("Resolver should return error if no pipeline resolved")
		}
	})

	t.Run("Errors if user cannot be found", func(t *testing.T) {
		t.Parallel()

		// mock a failed repsonse
		transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
			}, nil
		})
		client := &http.Client{Transport: transport}
		f := &factory.Factory{
			RestAPIClient: buildkite.NewClient(client),
		}

		r := resolver.ResolveBuildForCurrentUser("main", pipelineResolver, f)
		_, err := r(context.Background())

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
		transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			resp := responses[callIndex]
			callIndex++
			return &resp, nil
		})
		client := &http.Client{Transport: transport}
		fs := afero.NewMemMapFs()
		f := &factory.Factory{
			RestAPIClient: buildkite.NewClient(client),
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
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
