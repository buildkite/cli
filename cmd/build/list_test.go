package build

import (
	"testing"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

type buildListOptions struct {
	duration string
	message  string
}

func applyClientSideFilters(builds []buildkite.Build, opts buildListOptions) ([]buildkite.Build, error) {
	cmd := &ListCmd{
		Duration: opts.duration,
		Message:  opts.message,
	}
	return cmd.applyClientSideFilters(builds)
}

func TestFilterBuilds(t *testing.T) {
	now := time.Now()
	builds := []buildkite.Build{
		{
			Number:     1,
			Message:    "Fast build",
			StartedAt:  &buildkite.Timestamp{Time: now.Add(-5 * time.Minute)},
			FinishedAt: &buildkite.Timestamp{Time: now.Add(-4 * time.Minute)}, // 1 minute
		},
		{
			Number:     2,
			Message:    "Long build",
			StartedAt:  &buildkite.Timestamp{Time: now.Add(-30 * time.Minute)},
			FinishedAt: &buildkite.Timestamp{Time: now.Add(-10 * time.Minute)}, // 20 minutes
		},
	}

	opts := buildListOptions{duration: "10m"}
	filtered, err := applyClientSideFilters(builds, opts)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 build >= 10m, got %d", len(filtered))
	}

	opts = buildListOptions{message: "Fast"}
	filtered, err = applyClientSideFilters(builds, opts)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 build with 'Fast', got %d", len(filtered))
	}
}
