package resolver_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
)

func TestResolveBuildFromUserId(t *testing.T) {
	t.Parallel()

	nilPipelineResolver := func(context.Context) (*pipeline.Pipeline, error) {
		return nil, nil
	}
	errorPipelineResolver := func(context.Context) (*pipeline.Pipeline, error) {
		return nil, errors.New("")
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

		_, err := resolver.ResolveBuildForUser(context.Background(), "", "", nilPipelineResolver, f)

		if err == nil {
			t.Fatal("Resolver should return error if no pipeline resolved")
		}
	})

	t.Run("Errors if pipeline resolver errors", func(t *testing.T) {
		t.Parallel()

		_, err := resolver.ResolveBuildForUser(context.Background(), "", "", errorPipelineResolver, f)

		if err == nil {
			t.Fatal("Resolver should return error if no pipeline resolved")
		}
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
