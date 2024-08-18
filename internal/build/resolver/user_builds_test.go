package resolver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)

	apiClient, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	f := &factory.Factory{
		RestAPIClient: apiClient,
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
