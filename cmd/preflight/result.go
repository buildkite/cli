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
	resultActiveFailure
	resultUnknown
)

type Result struct {
	kind       resultKind
	buildState string
}

func NewResult(build buildkite.Build, hardFailedJobs []buildkite.Job) Result {
	state := buildstate.State(build.State)

	if state == buildstate.Passed {
		return Result{kind: resultCompletedPass, buildState: build.State}
	}

	if buildstate.IsTerminal(state) {
		return Result{kind: resultCompletedFailure, buildState: build.State}
	}

	if state == buildstate.Failing || len(hardFailedJobs) > 0 {
		return Result{kind: resultActiveFailure, buildState: build.State}
	}

	return Result{kind: resultUnknown, buildState: build.State}
}

func (r Result) Error() error {
	switch r.kind {
	case resultCompletedFailure:
		return bkErrors.NewPreflightCompletedFailureError(nil, fmt.Sprintf("preflight build %s", r.buildState))
	case resultActiveFailure:
		return bkErrors.NewPreflightIncompleteFailureError(nil, "preflight build has active failures")
	case resultUnknown:
		return bkErrors.NewPreflightUnknownError(nil,
			fmt.Sprintf("unknown preflight build state %q", r.buildState),
			"This is likely a bug",
			"Report to Buildkite",
		)
	default:
		return nil
	}
}
