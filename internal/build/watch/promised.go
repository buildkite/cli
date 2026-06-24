package watch

import (
	"context"
	"fmt"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

// FetchPromisedHardFailures returns the set of job UUIDs that the server
// classifies as failing *and* are still running — i.e. running jobs that have
// declared a hard-failing promised exit status (an early failure declaration).
//
// It queries the build-scoped jobs index with state=failed. When
// PROMISED_EXIT_STATUS_FOR_FAILING is enabled server-side, that filter folds
// running script jobs with a hard-failing promised exit status into the failed
// results (soft-fails excluded), so the server — not the client — owns the
// precise "will fail" classification. We keep only the still-running jobs;
// finished failures are surfaced through the normal failed path.
//
// Best-effort by design: until the server change ships (or for orgs without the
// flag), state=failed returns only finished jobs, so the running-job filter
// yields an empty set and this is a no-op. Callers should treat an error as
// "no promised failures this poll" rather than fatal.
func FetchPromisedHardFailures(ctx context.Context, client *buildkite.Client, org, pipeline string, buildNumber int) (map[string]bool, error) {
	result := make(map[string]bool)
	opts := &buildkite.JobsListOptions{
		State:   []string{"failed"},
		PerPage: 100,
	}

	for {
		reqCtx, cancel := context.WithTimeout(ctx, DefaultRequestTimeout)
		list, _, err := client.Jobs.ListByBuild(reqCtx, org, pipeline, fmt.Sprint(buildNumber), opts)
		cancel()
		if err != nil {
			return nil, err
		}

		for _, j := range list.Items {
			if isActiveState(j.State) {
				result[j.ID] = true
			}
		}

		if list.Links.Next == "" {
			break
		}
		next, err := list.Links.Next.ToOptions()
		if err != nil {
			return result, err
		}
		opts = next
	}

	return result, nil
}
