package build

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
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
			org, agent := parsePipelineArg(testcase.url, &c)
			if org != testcase.org {
				t.Error("parsed organization slug did not match expected")
			}
			if agent != testcase.pipeline {
				t.Error("parsed pipeline name did not match expected")
			}
		})
	}
}
