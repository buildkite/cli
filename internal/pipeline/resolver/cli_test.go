package resolver_test

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
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

			c := config.Config{
				Organization: "testing",
			}
			f := resolver.ResolveFromPositionalArgument([]string{testcase.url}, 0, &c)
			pipeline, err := f()
			if err != nil {
				t.Error(err)
			}
			if pipeline.Org != testcase.org {
				t.Error("parsed organization slug did not match expected")
			}
			if pipeline.Name != testcase.pipeline {
				t.Error("parsed pipeline name did not match expected")
			}
		})
	}

	t.Run("Returns error if failed parsing", func(t *testing.T) {
		t.Parallel()

		c := config.Config{
			Organization: "testing",
		}
		f := resolver.ResolveFromPositionalArgument([]string{"https://buildkite.com/"}, 0, &c)
		pipeline, err := f()
		if err == nil {
			t.Error("Should have failed parsing pipeline")
		}
		if pipeline != nil {
			t.Error("No pipeline should be returned")
		}
	})
}
