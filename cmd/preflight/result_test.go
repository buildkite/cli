package preflight

import (
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
	}{
		{name: "clean pass", result: Result{kind: resultCompletedPass, buildState: "passed"}, wantCode: bkErrors.ExitCodeSuccess},
		{name: "completed failure", result: Result{kind: resultCompletedFailure, buildState: "failed"}, wantCode: bkErrors.ExitCodePreflightCompletedFailure, wantErr: true},
		{name: "active failure", result: Result{kind: resultActiveFailure, buildState: "running"}, wantCode: bkErrors.ExitCodePreflightActiveFailure, wantErr: true},
		{name: "unknown state", result: Result{kind: resultUnknown, buildState: "mystery"}, wantCode: bkErrors.ExitCodePreflightUnknown, wantErr: true},
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
		})
	}
}
