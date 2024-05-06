package resolver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

func TestResolveBuildFromCurrentBranch(t *testing.T) {
	t.Parallel()
	t.Run("Can resolve a build from graphql", func(t *testing.T) {
		t.Parallel()
		mock := genqlient.Response{
			Data: &graphql.RecentBuildsForBranchResponse{
				Pipeline: &graphql.RecentBuildsForBranchPipeline{
					Builds: &graphql.RecentBuildsForBranchPipelineBuildsBuildConnection{
						Edges: []*graphql.RecentBuildsForBranchPipelineBuildsBuildConnectionEdgesBuildEdge{
							{
								Node: &graphql.RecentBuildsForBranchPipelineBuildsBuildConnectionEdgesBuildEdgeNodeBuild{
									Number: 42,
								},
							},
						},
					},
				},
			},
		}
		f := factory.Factory{
			GraphQLClient: genqlient.NewClient("", mockDoer(&mock)),
		}
		r := resolver.ResolveBuildFromCurrentBranch("testing", pipelineResolver, &f)
		b, err := r(context.Background())

		if err != nil {
			t.Fatal(err)
		}

		if b.BuildNumber != 42 {
			t.Errorf("Build number did not match 42: %d", b.BuildNumber)
		}
	})

	t.Run("Errors if no builds are found", func(t *testing.T) {
		t.Parallel()
		mock := genqlient.Response{
			Data: &graphql.RecentBuildsForBranchResponse{
				Pipeline: &graphql.RecentBuildsForBranchPipeline{
					Builds: &graphql.RecentBuildsForBranchPipelineBuildsBuildConnection{
						Edges: []*graphql.RecentBuildsForBranchPipelineBuildsBuildConnectionEdgesBuildEdge{},
					},
				},
			},
		}
		f := factory.Factory{
			GraphQLClient: genqlient.NewClient("", mockDoer(&mock)),
		}
		r := resolver.ResolveBuildFromCurrentBranch("testing", pipelineResolver, &f)
		_, err := r(context.Background())

		if err.Error() != "No builds found for pipeline test pipeline, branch testing" {
			t.Fatal(err)
		}
	})

	t.Run("Errors if pipeline cannot be resovled", func(t *testing.T) {
		t.Parallel()
		r := resolver.ResolveBuildFromCurrentBranch("testing", func(context.Context) (*pipeline.Pipeline, error) { return nil, nil }, &factory.Factory{})
		_, err := r(context.Background())

		if err.Error() != "Failed to resolve a pipeline to query builds on." {
			t.Fatal(err)
		}
	})
}

type doer struct {
	obj any
}

// Do implements graphql.Doer.
func (d doer) Do(*http.Request) (*http.Response, error) {
	j, _ := json.Marshal(d.obj)

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBuffer(j)),
	}, nil
}

func mockDoer(obj any) genqlient.Doer {
	return doer{obj}
}

func pipelineResolver(context.Context) (*pipeline.Pipeline, error) {
	return &pipeline.Pipeline{
		Org:  "test org",
		Name: "test pipeline",
	}, nil
}
