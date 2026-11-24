package pipeline

import (
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

func TestCopyKeepsBranchBuildFilters(t *testing.T) {
	t.Parallel()

	source := &buildkite.Pipeline{
		SkipQueuedBranchBuilds:          true,
		SkipQueuedBranchBuildsFilter:    "!main",
		CancelRunningBranchBuilds:       true,
		CancelRunningBranchBuildsFilter: "feature/*",
	}

	c := CopyCmd{}
	got := c.buildCreatePipeline(source, "copy", false, "")

	if !got.SkipQueuedBranchBuilds || got.SkipQueuedBranchBuildsFilter != "!main" {
		t.Errorf("skip queued builds = %v %q, want true %q", got.SkipQueuedBranchBuilds, got.SkipQueuedBranchBuildsFilter, "!main")
	}
	if !got.CancelRunningBranchBuilds || got.CancelRunningBranchBuildsFilter != "feature/*" {
		t.Errorf("cancel running builds = %v %q, want true %q", got.CancelRunningBranchBuilds, got.CancelRunningBranchBuildsFilter, "feature/*")
	}
}
