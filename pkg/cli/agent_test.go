package cli

import (
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/afero"
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

			conf := config.New(afero.NewMemMapFs(), nil)
			conf.SelectOrganization("testing", true)
			org, agent := parseAgentArg(testcase.url, conf)

			if org != testcase.org {
				t.Error("parsed organization slug did not match expected")
			}
			if agent != testcase.agent {
				t.Error("parsed agent ID did not match expected")
			}
		})
	}
}

func TestAgentListCmdHelp(t *testing.T) {
	cmd := &AgentListCmd{}
	help := cmd.Help()

	// Check that help includes the interactive flag example
	if help == "" {
		t.Error("expected help text to be non-empty")
	}

	// Check for key examples
	expectedPhrases := []string{
		"bk agent list",
		"interactive TUI",
		"--queue=deploy",
		"--hostname=ci-server-01",
		"--output json",
	}

	for _, phrase := range expectedPhrases {
		if !contains(help, phrase) {
			t.Errorf("expected help to contain '%s'", phrase)
		}
	}
}

func TestBuildWatchCmdHelp(t *testing.T) {
	cmd := &BuildWatchCmd{}
	help := cmd.Help()

	// Check that help includes the interactive flag example
	if help == "" {
		t.Error("expected help text to be non-empty")
	}

	// Check for key examples
	expectedPhrases := []string{
		"bk build watch 42",
		"live refresh",
		"--output json",
	}

	for _, phrase := range expectedPhrases {
		if !contains(help, phrase) {
			t.Errorf("expected help to contain '%s'", phrase)
		}
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
