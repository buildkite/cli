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
	stopStates   map[buildstate.State]struct{}
}

// StopReason describes why WatchBuild exited before the build reached a
// terminal state.
type StopReason string

const (
	StopReasonStateReached StopReason = "state_reached"
)

// StopError indicates WatchBuild exited because a configured stop condition
// was met rather than because the build reached a terminal state.
type StopError struct {
	Reason StopReason
	State  buildstate.State
}

func (e *StopError) Error() string {
	return fmt.Sprintf("watch stopped early: %s (%s)", e.Reason, e.State)
}

// WithTestTracking enables polling BuildTests.List for failed tests on each
// iteration, calling onTestStatus with any newly-seen test changes.
func WithTestTracking(fn TestStatusFunc) WatchOpt {
	return func(c *watchConfig) {
		c.onTestStatus = fn
	}
}

// WithStopStates exits the watch loop when the build enters any of the given
// states, in addition to the default terminal-state behavior.
func WithStopStates(states ...buildstate.State) WatchOpt {
	return func(c *watchConfig) {
		if c.stopStates == nil {
			c.stopStates = make(map[buildstate.State]struct{}, len(states))
		}
		for _, state := range states {
			c.stopStates[state] = struct{}{}
		}
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

			if stopErr := shouldStopWatching(cfg, b); stopErr != nil {
				return b, stopErr
			}
		}

		select {
		case <-ctx.Done():
			return lastBuild, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func shouldStopWatching(cfg *watchConfig, build buildkite.Build) error {
	if build.FinishedAt != nil {
		return nil
	}

	state := buildstate.State(build.State)
	if buildstate.IsTerminal(state) {
		return nil
	}

	if _, stop := cfg.stopStates[state]; stop {
		return &StopError{
			Reason: StopReasonStateReached,
			State:  state,
		}
	}

	return nil
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
