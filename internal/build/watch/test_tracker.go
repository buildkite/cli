package watch

import (
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// TestTracker tracks which test executions have already been reported,
// so that each test change is only surfaced once across polling iterations.
type TestTracker struct {
	seenExecutions map[string]bool // keyed by execution ID
}

// NewTestTracker creates a new TestTracker.
func NewTestTracker() *TestTracker {
	return &TestTracker{
		seenExecutions: make(map[string]bool),
	}
}

// Update processes a list of build tests and returns only those with
// at least one execution that has not been seen before.
func (t *TestTracker) Update(tests []buildkite.BuildTest) []buildkite.BuildTest {
	var newTestChanges []buildkite.BuildTest
	for _, test := range tests {
		if test.Executions == nil || len(test.Executions) == 0 {
			continue
		}

		hasNewExecution := false
		for _, execution := range test.Executions {
			if !t.seenExecutions[execution.ID] {
				t.seenExecutions[execution.ID] = true
				hasNewExecution = true
			}
		}
		if hasNewExecution {
			newTestChanges = append(newTestChanges, test)
		}

	}
	return newTestChanges
}
