package resolver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/spf13/afero"
)

func TestResolveBuildWithOptsExcludesJobsAndPipeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("exclude_jobs") != "true" || r.URL.Query().Get("exclude_pipeline") != "true" {
			t.Errorf("resolver request query = %q, want jobs and pipeline excluded", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]buildkite.Build{{Number: 42}})
	}))
	defer server.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	conf := config.New(afero.NewMemMapFs(), nil)
	conf.SelectOrganization("acme", true)
	f := &factory.Factory{Config: conf, RestAPIClient: client}
	resolvePipeline := func(context.Context) (*pipeline.Pipeline, error) {
		return &pipeline.Pipeline{Name: "widgets", Org: "acme"}, nil
	}

	build, err := ResolveBuildWithOpts(f, resolvePipeline)(context.Background())
	if err != nil {
		t.Fatalf("ResolveBuildWithOpts failed: %v", err)
	}
	if build.BuildNumber != 42 {
		t.Fatalf("resolved build = %+v, want build 42", build)
	}
}
