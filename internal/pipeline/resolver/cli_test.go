package resolver_test

import (
	"context"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/internal/testutil"
	"github.com/spf13/afero"
)

func TestParsePipelineArg(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		url, org, pipeline string
	}{
		"org_pipeline_slug": {
			url:      "buildkite/cli",
			org:      "buildkite",
			pipeline: "cli",
		},
		"pipeline_slug": {
			url:      "abcd",
			org:      "testing",
			pipeline: "abcd",
		},
		"url": {
			url:      "https://buildkite.com/buildkite/buildkite-cli",
			org:      "buildkite",
			pipeline: "buildkite-cli",
		},
	}

	for name, testcase := range testcases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			conf := config.New(afero.NewMemMapFs(), nil)
			conf.SelectOrganization("testing")
			f := resolver.ResolveFromPositionalArgument([]string{testcase.url}, 0, conf)
			pipeline, err := f(context.Background())

			testutil.AssertNoError(t, err)
			testutil.AssertEqual(t, pipeline.Org, testcase.org, "organization slug")
			testutil.AssertEqual(t, pipeline.Name, testcase.pipeline, "pipeline name")
		})
	}

	t.Run("Returns error if failed parsing", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testing")
		f := resolver.ResolveFromPositionalArgument([]string{"https://buildkite.com/"}, 0, conf)
		pipeline, err := f(context.Background())

		testutil.AssertEqual(t, err != nil, true, "expected error")
		testutil.AssertEqual(t, pipeline == nil, true, "expected nil pipeline")
	})
}
