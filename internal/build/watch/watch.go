package watch

import (
	"context"
	"fmt"
	"time"

	buildstate "github.com/buildkite/cli/v3/internal/build/state"
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
// Returning an error aborts the watch loop and propagates that error to the caller.
type StatusFunc func(b buildkite.Build) error

// TestStatusFunc is called with newly-seen failed test executions on each poll.
// Returning an error aborts the watch loop.
type TestStatusFunc func(newFailures []buildkite.BuildTest) error

// WatchOpt configures optional WatchBuild behavior.
type WatchOpt func(*watchConfig)

type watchConfig struct {
	onTestStatus TestStatusFunc
}

// WithTestTracking enables polling BuildTests.List for failed tests on each
// iteration, calling onTestStatus with any newly-seen failures.
func WithTestTracking(fn TestStatusFunc) WatchOpt {
	return func(c *watchConfig) {
		c.onTestStatus = fn
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
	opts ...WatchOpt,
) (buildkite.Build, error) {
	cfg := &watchConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	var testTracker *TestTracker
	if cfg.onTestStatus != nil {
		testTracker = NewTestTracker()
	}

	var (
		consecutiveErrors int
		lastBuild         buildkite.Build
	)

	for {
		if err := ctx.Err(); err != nil {
			return lastBuild, err
		}

		reqCtx, cancel := context.WithTimeout(ctx, DefaultRequestTimeout)
		b, _, err := client.Builds.Get(reqCtx, org, pipeline, fmt.Sprint(buildNumber), nil)
		cancel()

		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= DefaultMaxConsecutiveErrors {
				return lastBuild, fmt.Errorf("fetching build status (%d consecutive errors): %w", consecutiveErrors, err)
			}
		} else {
			consecutiveErrors = 0
			lastBuild = b
			if onStatus != nil {
				if err := onStatus(b); err != nil {
					return b, err
				}
			}

			if testTracker != nil && b.ID != "" {
				if err := pollTestFailures(ctx, client, org, b.ID, testTracker, cfg.onTestStatus); err != nil {
					return b, err
				}
			}

			if b.FinishedAt != nil || buildstate.IsTerminal(buildstate.State(b.State)) {
				return b, nil
			}
		}

		select {
		case <-ctx.Done():
			return lastBuild, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func pollTestFailures(ctx context.Context, client *buildkite.Client, org, buildID string, tracker *TestTracker, onTestStatus TestStatusFunc) error {
	reqCtx, cancel := context.WithTimeout(ctx, DefaultRequestTimeout)
	defer cancel()

	tests, _, err := client.BuildTests.List(reqCtx, org, buildID, &buildkite.BuildTestsListOptions{
		Result:  "^failed",
		State:   "enabled",
		Include: "latest_fail",
	})
	if err != nil {
		// Test data may not be available yet; don't treat as fatal.
		return nil
	}

	newFailures := tracker.Update(tests)
	if len(newFailures) > 0 {
		return onTestStatus(newFailures)
	}
	return nil
}
