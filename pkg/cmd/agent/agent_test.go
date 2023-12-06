package agent

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
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
		"clustered url": {
			url:   "https://buildkite.com/organizations/buildkite/clusters/0b7c9944-10ba-434d-9dbb-b332431252de/queues/3d039cf8-9862-4cb0-82cd-fc5c497a265a/agents/018c3d31-1b4a-454a-87f6-190b206e3759",
			org:   "buildkite",
			agent: "018c3d31-1b4a-454a-87f6-190b206e3759",
		},
	}

	for name, testcase := range testcases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := config.Config{
				Organization: "testing",
			}
			org, agent := parseAgentArg(testcase.url, &c)

			if org != testcase.org {
				t.Error("parsed organization slug did not match expected")
			}
			if agent != testcase.agent {
				t.Error("parsed agent ID did not match expected")
			}
		})
	}
}
