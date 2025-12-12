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

func TestBuildListOptions_MetaData(t *testing.T) {
	cmd := &ListCmd{
		MetaData: map[string]string{
			"env":    "production",
			"deploy": "true",
		},
	}

	opts, err := cmd.buildListOptions()
	if err != nil {
		t.Fatalf("buildListOptions failed: %v", err)
	}

	if len(opts.MetaData.MetaData) != 2 {
		t.Errorf("Expected 2 meta-data filters, got %d", len(opts.MetaData.MetaData))
	}

	if opts.MetaData.MetaData["env"] != "production" {
		t.Errorf("Expected env=production, got env=%s", opts.MetaData.MetaData["env"])
	}

	if opts.MetaData.MetaData["deploy"] != "true" {
		t.Errorf("Expected deploy=true, got deploy=%s", opts.MetaData.MetaData["deploy"])
	}
}

func TestBuildListOptions_EmptyMetaData(t *testing.T) {
	cmd := &ListCmd{}

	opts, err := cmd.buildListOptions()
	if err != nil {
		t.Fatalf("buildListOptions failed: %v", err)
	}

	if len(opts.MetaData.MetaData) != 0 {
		t.Errorf("Expected empty meta-data, got %d entries", len(opts.MetaData.MetaData))
	}
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
