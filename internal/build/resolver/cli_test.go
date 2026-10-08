package resolver_test

import (
	"context"
	"errors"
	"strings"
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
			conf.SelectOrganization("testing", true)
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
		conf.SelectOrganization("testing", true)
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

func TestBuildPipelineResolutionErrors(t *testing.T) {
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "other-org")
	upstreamErr := errors.New("pipeline API unavailable")
	for _, tt := range []struct {
		name, arg, want string
		pipelineErr     error
	}{
		{"no pipeline", "42", "no pipeline found", nil},
		{"upstream error", "42", "pipeline API unavailable", upstreamErr},
		{"invalid input", "not-a-number", "unable to parse the input build argument", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			resolvePipeline := func(context.Context) (*pipeline.Pipeline, error) {
				called = true
				return nil, tt.pipelineErr
			}
			conf := config.New(afero.NewMemMapFs(), nil)
			resolve := resolver.ResolveFromPositionalArgument([]string{tt.arg}, 0, resolvePipeline, conf)
			build, err := resolve(context.Background())
			if build != nil || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("build=%v, err=%v; want %q", build, err, tt.want)
			}
			if tt.arg == "42" {
				if !called || !strings.Contains(err.Error(), `in "other-org"; use --pipeline`) {
					t.Fatalf("missing pipeline resolution context: called=%t, err=%v", called, err)
				}
			} else if called {
				t.Fatal("invalid build input invoked pipeline resolver")
			}
			if tt.pipelineErr != nil && !errors.Is(err, tt.pipelineErr) {
				t.Fatalf("lost underlying pipeline error: %v", err)
			}
		})
	}
}
