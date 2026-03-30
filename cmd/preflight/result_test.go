package preflight

import (
	"strings"
	"testing"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestResult(t *testing.T) {
	tests := []struct {
		name       string
		build      buildkite.Build
		failedJobs []buildkite.Job
		want       Result
	}{
		{
			name:  "clean pass",
			build: buildkite.Build{State: "passed"},
			want:  Result{kind: resultCompletedPass, buildState: "passed"},
		},
		{
			name:       "passed with failed jobs exits clean",
			build:      buildkite.Build{State: "passed"},
			failedJobs: []buildkite.Job{{ID: "job-1", State: "failed"}},
			want:       Result{kind: resultCompletedPass, buildState: "passed"},
		},
		{
			name:  "terminal failed build is completed failure",
			build: buildkite.Build{State: "failed"},
			want:  Result{kind: resultCompletedFailure, buildState: "failed"},
		},
		{
			name:       "active build with failures is active failure",
			build:      buildkite.Build{State: "running"},
			failedJobs: []buildkite.Job{{ID: "job-1", State: "failed"}},
			want:       Result{kind: resultActiveFailure, buildState: "running"},
		},
		{
			name:  "running build is incomplete",
			build: buildkite.Build{State: "running"},
			want:  Result{kind: resultIncomplete, buildState: "running"},
		},
		{
			name:  "blocked build is incomplete",
			build: buildkite.Build{State: "blocked"},
			want:  Result{kind: resultIncomplete, buildState: "blocked"},
		},
		{
			name:  "canceling build is incomplete",
			build: buildkite.Build{State: "canceling"},
			want:  Result{kind: resultIncomplete, buildState: "canceling"},
		},
		{
			name:  "unknown state is unknown result",
			build: buildkite.Build{State: "mystery"},
			want:  Result{kind: resultUnknown, buildState: "mystery"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewResult(tt.build, tt.failedJobs); got != tt.want {
				t.Fatalf("NewResult() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResultError(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		wantCode int
		wantErr  bool
		wantText string
	}{
		{name: "clean pass", result: Result{kind: resultCompletedPass, buildState: "passed"}, wantCode: bkErrors.ExitCodeSuccess},
		{name: "completed failure", result: Result{kind: resultCompletedFailure, buildState: "failed"}, wantCode: bkErrors.ExitCodePreflightCompletedFailure, wantErr: true},
		{name: "active failure", result: Result{kind: resultActiveFailure, buildState: "running"}, wantCode: bkErrors.ExitCodePreflightActiveFailure, wantErr: true},
		{name: "incomplete", result: Result{kind: resultIncomplete, buildState: "blocked"}, wantCode: bkErrors.ExitCodePreflightIncomplete, wantErr: true, wantText: `preflight build is blocked`},
		{name: "unknown state", result: Result{kind: resultUnknown, buildState: "passing"}, wantCode: bkErrors.ExitCodePreflightUnknown, wantErr: true, wantText: `preflight build is passing`},
		{name: "unknown result kind", result: Result{kind: resultKind(99), buildState: "passed"}, wantCode: bkErrors.ExitCodeInternalError, wantErr: true, wantText: "unknown preflight result type 99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Error()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Result.Error() err presence = %v, wantErr %v", err != nil, tt.wantErr)
			}
			if err == nil {
				return
			}
			if code := bkErrors.GetExitCodeForError(err); code != tt.wantCode {
				t.Fatalf("Result.Error() exit code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantText != "" && !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("Result.Error() text = %q, want substring %q", err.Error(), tt.wantText)
			}
		})
	}
}
