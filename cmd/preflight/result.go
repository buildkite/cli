package preflight

import (
	"fmt"

	buildstate "github.com/buildkite/cli/v3/internal/build/state"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type resultKind int

const (
	resultCompletedPass resultKind = iota
	resultCompletedFailure
	resultIncompleteFailure
	resultIncomplete
	resultUnknown
)

type Result struct {
	kind       resultKind
	buildState string
}

func NewResult(build buildkite.Build) Result {
	state := buildstate.State(build.State)

	if state == buildstate.Passed {
		return Result{kind: resultCompletedPass, buildState: build.State}
	}

	if buildstate.IsTerminal(state) {
		return Result{kind: resultCompletedFailure, buildState: build.State}
	}

	if state == buildstate.Failing {
		return Result{kind: resultIncompleteFailure, buildState: build.State}
	}

	if buildstate.IsIncomplete(state) {
		return Result{kind: resultIncomplete, buildState: build.State}
	}

	return Result{kind: resultUnknown, buildState: build.State}
}

func (r Result) Error() error {
	switch r.kind {
	case resultCompletedPass:
		return nil
	case resultCompletedFailure:
		return bkErrors.NewPreflightCompletedFailureError(nil, fmt.Sprintf("build is %s", r.buildState))
	case resultIncompleteFailure:
		return bkErrors.NewPreflightIncompleteFailureError(nil, fmt.Sprintf("build is %s", r.buildState))
	case resultIncomplete:
		return bkErrors.NewPreflightIncompleteError(nil, fmt.Sprintf("build is %s", r.buildState))
	case resultUnknown:
		return bkErrors.NewPreflightUnknownError(nil,
			fmt.Sprintf("build is %s", r.buildState),
		)
	default:
		return bkErrors.NewInternalError(nil,
			fmt.Sprintf("build is %d, unable to coerce to preflight result", r.kind),
			"This is likely a bug",
			"Report to Buildkite",
		)
	}
}
