package build

import (
	"slices"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

func TestWatchJobs(t *testing.T) {
	got := watchJobs([]buildkite.Job{
		{State: "passed"},
		{State: "broken"},
		{State: "skipped"},
		{State: "failed"},
		{State: "waiting_failed"},
	})
	want := []buildkite.Job{
		{State: "passed"},
		{State: "skipped"},
		{State: "failed"},
		{State: "waiting_failed"},
	}

	if !slices.EqualFunc(got, want, func(a, b buildkite.Job) bool {
		return a.State == b.State
	}) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}