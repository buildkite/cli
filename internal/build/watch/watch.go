package watch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

// TestStatusFunc is called with newly-seen test changes on each poll.
// Returning an error aborts the watch loop.
type TestStatusFunc func(newTestChanges []buildkite.BuildTest) error

// WatchOpt configures optional WatchBuild behavior.
type WatchOpt func(*watchConfig)

type watchConfig struct {
	onTestStatus TestStatusFunc
}

// WithTestTracking enables polling BuildTests.List for failed tests on each
// iteration, calling onTestStatus with any newly-seen test changes.
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
	testPollingEnabled := false
	if cfg.onTestStatus != nil {
		testTracker = NewTestTracker()
		testPollingEnabled = true
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

			if testPollingEnabled && b.ID != "" {
				enabled, err := pollTestFailures(ctx, client, org, b.ID, testTracker, cfg.onTestStatus)
				if err != nil {
					return b, err
				}
				testPollingEnabled = enabled
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

func pollTestFailures(ctx context.Context, client *buildkite.Client, org, buildID string, tracker *TestTracker, onTestStatus TestStatusFunc) (bool, error) {
	opts := &buildkite.BuildTestsListOptions{
		ListOptions: buildkite.ListOptions{Page: 1, PerPage: 100},
		Result:      "failed",
		State:       "enabled",
		Include:     "executions",
	}

	var newTestChanges []buildkite.BuildTest
	for {
		reqCtx, cancel := context.WithTimeout(ctx, DefaultRequestTimeout)
		tests, resp, err := client.BuildTests.List(reqCtx, org, buildID, opts)
		cancel()
		if err != nil {
			if isPermanentTestPollingError(err) {
				return false, nil
			}

			// Test data may not be available yet; don't treat as fatal.
			break
		}

		newTestChanges = append(newTestChanges, tracker.Update(tests)...)
		if resp == nil || resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	if len(newTestChanges) > 0 {
		return true, onTestStatus(newTestChanges)
	}

	return true, nil
}

func isPermanentTestPollingError(err error) bool {
	var apiErr *buildkite.ErrorResponse
	if !errors.As(err, &apiErr) || apiErr.Response == nil {
		return false
	}

	switch apiErr.Response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return true
	default:
		return false
	}
}
