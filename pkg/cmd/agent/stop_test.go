package agent

import (
	"testing"

	"github.com/spf13/viper"
)

func TestParseAgentArg(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		url, org, agent string
	}{
		"slug": {
			url:   "buildkite/abcd",
			org:   "buildkite",
			agent: "abcd",
		},
		"id": {
			url:   "abcd",
			org:   "testing",
			agent: "abcd",
		},
		"url": {
			url:   "https://buildkite.com/organizations/buildkite/agents/018a4a65-bfdb-4841-831a-ff7c1ddbad99",
			org:   "buildkite",
			agent: "018a4a65-bfdb-4841-831a-ff7c1ddbad99",
		},
	}

	for name, testcase := range testcases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			v := viper.GetViper()
			v.Set("organization", "testing")
			org, agent := parseAgentArg(testcase.url, v)

			if org != testcase.org {
				t.Error("parsed organization slug did not match expected")
			}
			if agent != testcase.agent {
				t.Error("parsed agent ID did not match expected")
			}
		})
	}
}
