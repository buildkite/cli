package watch

import (
	"context"
	"fmt"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

const (
	// DefaultMaxConsecutiveErrors is the number of consecutive polling failures
	// before the watch loop aborts.
	DefaultMaxConsecutiveErrors = 10

	// DefaultRequestTimeout is the per-request timeout for each polling call.
	DefaultRequestTimeout = 30 * time.Second
)

// StatusFunc is called on each successful poll with the latest build state.
type StatusFunc func(b buildkite.Build)

func isTerminalBuildState(state string) bool {
	switch state {
	case "passed", "failed", "failing", "blocked", "canceled", "canceling", "skipped", "not_run":
		return true
	default:
		return false
	}
}

// WatchBuild polls a build until it reaches a terminal state (FinishedAt != nil).
// It calls onStatus after each successful poll so callers can render progress.
func WatchBuild(
	ctx context.Context,
	client *buildkite.Client,
	org, pipeline string,
	buildNumber int,
	interval time.Duration,
	onStatus StatusFunc,
) (buildkite.Build, error) {
	consecutiveErrors := 0

	for {
		if err := ctx.Err(); err != nil {
			return buildkite.Build{}, err
		}

		reqCtx, cancel := context.WithTimeout(ctx, DefaultRequestTimeout)
		b, _, err := client.Builds.Get(reqCtx, org, pipeline, fmt.Sprint(buildNumber), nil)
		cancel()

		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= DefaultMaxConsecutiveErrors {
				return buildkite.Build{}, fmt.Errorf("fetching build status (%d consecutive errors): %w", consecutiveErrors, err)
			}
		} else {
			consecutiveErrors = 0
			onStatus(b)

			if b.FinishedAt != nil || isTerminalBuildState(b.State) {
				return b, nil
			}
		}

		select {
		case <-ctx.Done():
			return buildkite.Build{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}
