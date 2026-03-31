package watch

import (
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// TestTracker tracks which test failure executions have already been reported,
// so that each failure is only surfaced once across polling iterations.
type TestTracker struct {
	seen map[string]bool // keyed by latest_fail execution ID
}

// NewTestTracker creates a new TestTracker.
func NewTestTracker() *TestTracker {
	return &TestTracker{
		seen: make(map[string]bool),
	}
}

// Update processes a list of build tests and returns only those with
// a LatestFail execution that has not been seen before.
func (t *TestTracker) Update(tests []buildkite.BuildTest) []buildkite.BuildTest {
	var newFailures []buildkite.BuildTest
	for _, test := range tests {
		if test.LatestFail == nil {
			continue
		}
		if t.seen[test.LatestFail.ID] {
			continue
		}
		t.seen[test.LatestFail.ID] = true
		newFailures = append(newFailures, test)
	}
	return newFailures
}
