package watch

import (
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// JobSummary aggregates job counts by high-level state.
type JobSummary struct {
	Passed    int
	Failed    int
	Canceled  int
	Running   int
	Scheduled int
	Blocked   int
	Skipped   int
	Waiting   int
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
		case "failed", "timed_out":
			s.Failed++
		case "canceled":
			s.Canceled++
		case "skipped", "broken":
			s.Skipped++
		case "blocked", "blocked_failed":
			s.Blocked++
		case "scheduled", "assigned", "accepted", "reserved":
			s.Scheduled++
		case "waiting", "waiting_failed",
			"pending", "limited", "limiting",
			"platform_limited", "platform_limiting",
			"expired":
			s.Waiting++
		}
	}
	return s
}
