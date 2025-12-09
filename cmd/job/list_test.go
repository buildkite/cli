package job

import (
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestFilterJobs(t *testing.T) {
	now := time.Now()
	jobs := []buildkite.Job{
		{
			ID:              "job-1",
			State:           "running",
			AgentQueryRules: []string{"queue=test-queue"},
			StartedAt:       &buildkite.Timestamp{Time: now.Add(-5 * time.Minute)},
			FinishedAt:      &buildkite.Timestamp{Time: now.Add(-4 * time.Minute)}, // 1 minute
		},
		{
			ID:              "job-2",
			State:           "passed",
			AgentQueryRules: []string{"queue=other-queue"},
			StartedAt:       &buildkite.Timestamp{Time: now.Add(-30 * time.Minute)},
			FinishedAt:      &buildkite.Timestamp{Time: now.Add(-10 * time.Minute)}, // 20 minutes
		},
	}

	opts := jobListOptions{duration: ">10m"}
	filtered, err := applyClientSideFilters(jobs, opts)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 job >= 10m, got %d", len(filtered))
	}

	opts = jobListOptions{queue: "test-queue"}
	filtered, err = applyClientSideFilters(jobs, opts)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 job with 'test-queue', got %d", len(filtered))
	}
}
