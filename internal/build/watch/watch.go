package watch

import (
	"context"
	"fmt"
	"time"

	buildstate "github.com/buildkite/cli/v3/internal/build/state"
	buildkite "github.com/buildkite/go-buildkite/v5"
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

// WatchOpt configures optional WatchBuild behavior.
type WatchOpt func(*watchConfig)

type watchConfig struct {
	includeRetriedJobs bool
}

// WithRetriedJobs includes retried (superseded) jobs in each poll so the
// tracker can correlate original failures with their retry outcomes.
func WithRetriedJobs() WatchOpt {
	return func(c *watchConfig) {
		c.includeRetriedJobs = true
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

	var (
		consecutiveErrors int
		lastBuild         buildkite.Build
	)

	for {
		if err := ctx.Err(); err != nil {
			return lastBuild, err
		}

		reqCtx, cancel := context.WithTimeout(ctx, DefaultRequestTimeout)
		getOpts := &buildkite.BuildGetOptions{
			BuildsListOptions: buildkite.BuildsListOptions{
				IncludeRetriedJobs: cfg.includeRetriedJobs,
				// Watchers render job progress but never use the pipeline
				// payload, so exclude it to keep polling responses small.
				ExcludePipeline: true,
			},
		}
		b, _, err := client.Builds.Get(reqCtx, org, pipeline, fmt.Sprint(buildNumber), getOpts)
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
