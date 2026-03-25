package watch

import (
	"fmt"
	"strings"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

// JobSummary aggregates job counts by high-level state.
type JobSummary struct {
	Passed    int
	Failed    int
	Running   int
	Scheduled int
	Blocked   int
	Skipped   int
	Waiting   int
}

// String returns a human-readable summary of non-zero job counts,
// e.g. "3 passed, 1 failed, 2 running".
func (s JobSummary) String() string {
	type entry struct {
		count int
		label string
	}
	entries := []entry{
		{s.Passed, "passed"},
		{s.Failed, "failed"},
		{s.Running, "running"},
		{s.Scheduled, "scheduled"},
		{s.Blocked, "blocked"},
		{s.Skipped, "skipped"},
		{s.Waiting, "waiting"},
	}
	var parts []string
	for _, e := range entries {
		if e.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", e.count, e.label))
		}
	}
	return strings.Join(parts, ", ")
}

// Summarize counts script jobs from a build into high-level state buckets.
func Summarize(b buildkite.Build) JobSummary {
	var s JobSummary
	for _, j := range b.Jobs {
		if j.Type != "script" {
			continue
		}
		switch j.State {
		case "running", "canceling", "timing_out":
			s.Running++
		case "passed":
			s.Passed++
		case "failed", "timed_out", "canceled", "expired":
			s.Failed++
		case "skipped", "broken":
			s.Skipped++
		case "blocked", "blocked_failed":
			s.Blocked++
		case "scheduled", "assigned", "accepted", "reserved":
			s.Scheduled++
		case "waiting", "waiting_failed",
			"pending", "limited", "limiting",
			"platform_limited", "platform_limiting":
			s.Waiting++
		}
	}
	return s
}
