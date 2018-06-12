package git

import "testing"

func TestMatchRemotes(t *testing.T) {
	for _, tc := range []struct {
		From  string
		To    string
		Match bool
	}{
		{`git@github.com:buildkite/llamas.git`, `https://github.com/buildkite/agent.git`, false},
		{`git@github.com:buildkite/agent.git`, `https://github.com/buildkite/agent`, true},
		{`git@github.com:buildkite/agent.git`, `https://github.com/buildkite/agent.git`, true},
	} {
		t.Run("", func(t *testing.T) {
			if tc.Match && !MatchRemotes(tc.From, tc.To) {
				t.Errorf("Expected %q to match %q", tc.From, tc.To)
			} else if !tc.Match && MatchRemotes(tc.From, tc.To) {
				t.Errorf("Expected %q to NOT match %q", tc.From, tc.To)
			}
		})
	}
}
