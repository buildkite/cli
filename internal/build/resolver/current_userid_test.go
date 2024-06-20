package resolver_test

import (
	"bytes"
	"context"
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

func TestResolveBuildFromUserUUID(t *testing.T) {
	t.Parallel()

	pipelineResolver := func(context.Context) (*pipeline.Pipeline, error) {
		return &pipeline.Pipeline{
			Name: "testing",
			Org:  "test org",
		}, nil
	}

	t.Run("Errors when user id is not a member of the organization", func(t *testing.T) {
		t.Parallel()
		// mock a failed repsonse
		transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
			}, nil
		})
		client := &http.Client{Transport: transport}
		fs := afero.NewMemMapFs()
		f := &factory.Factory{
			RestAPIClient: buildkite.NewClient(client),
			Config:        config.New(fs, nil),
		}

		r := resolver.ResolveBuildFromCurrentUserId("1234", pipelineResolver, f)
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
				Body: io.NopCloser(strings.NewReader(`[{
					"id": "abc123-4567-8910-...", 
					"number": 584,
					"creator": {
					"id": "0183c4e6-c88c-xxxx-b15e-7801077a9181",
					"graphql_id": "VXNlci0tLTAxODNjNGU2LWM4OGxxxxxxxxxiMTVlLTc4MDEwNzdhOTE4MQ=="
					}					
					}]`)),
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

		r := resolver.ResolveBuildFromCurrentUserId("0183c4e6-c88c-xxxx-b15e-7801077a9181", pipelineResolver, f)
		build, err := r(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		if build.BuildNumber != 584 {
			t.Fatalf("Expected build 584, got %d", build.BuildNumber)
		}
	})
}
