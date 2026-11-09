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
		// The resolved pipeline's organization wins over the selected one.
		if r.URL.Path != "/v2/organizations/other/pipelines/widgets/builds" {
			t.Errorf("resolver request path = %q, want the pipeline's organization", r.URL.Path)
		}
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
		return &pipeline.Pipeline{Name: "widgets", Org: "other"}, nil
	}

	build, err := ResolveBuildWithOpts(f, resolvePipeline)(context.Background())
	if err != nil {
		t.Fatalf("ResolveBuildWithOpts failed: %v", err)
	}
	if build.BuildNumber != 42 || build.Organization != "other" || build.Pipeline != "widgets" {
		t.Fatalf("resolved build = %+v, want other/widgets build 42", build)
	}
}
