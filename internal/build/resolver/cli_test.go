package resolver_test

import (
	"context"
	"testing"

	"github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/spf13/afero"
)

func TestParseBuildArg(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		url, org, pipeline string
		num                int
	}{
		"org_pipeline_slug": {
			url:      "buildkite/cli/34",
			org:      "buildkite",
			pipeline: "cli",
			num:      34,
		},
		"pipeline_slug": {
			url:      "42",
			org:      "testing",
			pipeline: "abcd",
			num:      42,
		},
		"url": {
			url:      "https://buildkite.com/buildkite/buildkite-cli/builds/99",
			org:      "buildkite",
			pipeline: "buildkite-cli",
			num:      99,
		},
	}

	for name, testcase := range testcases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			conf := config.New(afero.NewMemMapFs(), nil)
			conf.SelectOrganization("testing")
			res := func(context.Context) (*pipeline.Pipeline, error) {
				return &pipeline.Pipeline{
					Name: testcase.pipeline,
					Org:  testcase.org,
				}, nil
			}
			f := resolver.ResolveFromPositionalArgument([]string{testcase.url}, 0, res, conf)
			build, err := f(context.Background())
			if err != nil {
				t.Error(err)
			}
			if build.Organization != testcase.org {
				t.Error("parsed organization slug did not match expected")
			}
			if build.Pipeline != testcase.pipeline {
				t.Error("parsed pipeline name did not match expected")
			}
			if build.BuildNumber != testcase.num {
				t.Error("parsed build number did not match expected")
			}
		})
	}

	t.Run("Returns error if failed parsing", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testing")
		f := resolver.ResolveFromPositionalArgument([]string{"https://buildkite.com/"}, 0, nil, conf)
		build, err := f(context.Background())
		if err == nil {
			t.Error("should have failed parsing build")
		}
		if build != nil {
			t.Error("no build should be returned")
		}
	})
}
