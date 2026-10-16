package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"

	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/charmbracelet/x/ansi"
)

func TestWatchTimeoutParsing(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  time.Duration
		bad   bool
	}{
		{value: "20m", want: 20 * time.Minute},
		{value: "0"},
		{value: "-1s", bad: true},
		{value: "invalid", bad: true},
	} {
		t.Run(tt.value, func(t *testing.T) {
			var cmd WatchCmd
			_, err := kong.Must(&cmd).Parse([]string{"--timeout=" + tt.value})
			if (err != nil) != tt.bad {
				t.Fatalf("parse error = %v, want error %v", err, tt.bad)
			}
			if !tt.bad && cmd.Timeout != tt.want {
				t.Fatalf("timeout = %s, want %s", cmd.Timeout, tt.want)
			}
		})
	}
}

func TestWatchTimeout(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		state   string
		stall   bool
		timeout bool
	}{
		{name: "polling wait", args: []string{"429", "--timeout=100ms"}, state: "running", timeout: true},
		{name: "poll request", args: []string{"429", "--timeout=100ms"}, stall: true, timeout: true},
		{name: "build resolution", args: []string{"--timeout=100ms"}, stall: true, timeout: true},
		{name: "completed before deadline", args: []string{"429", "--timeout=5s"}, state: "passed"},
		{name: "default unlimited", args: []string{"429"}, state: "passed"},
		{name: "explicit unlimited", args: []string{"429", "--timeout=0"}, state: "passed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
					http.Error(w, "read-only watch", http.StatusMethodNotAllowed)
					return
				}
				if tt.stall {
					select {
					case <-r.Context().Done():
					case <-time.After(3 * time.Second):
						t.Error("request was not canceled by watch timeout")
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(buildkite.Build{Number: 429, State: tt.state})
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")

			var cmd WatchCmd
			args := append([]string{"--pipeline=widgets", "--branch=feature", "--interval=10"}, tt.args...)
			ctx, err := kong.Must(&cmd).Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			err = cmd.Run(ctx, cli.Globals{NoInput: true, Quiet: true})
			if tt.timeout {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("watch error = %v, want deadline exceeded", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Fatalf("watch took %s; should not wait for the polling interval or request timeout", elapsed)
			}
		})
	}
}

func TestWatchStatus(t *testing.T) {
	finished := &buildkite.Timestamp{Time: time.Now()}
	script := func(state string) buildkite.Job { return buildkite.Job{Type: "script", State: state} }

	tests := []struct {
		name  string
		build buildkite.Build
		want  ansi.ProgramStatus
	}{
		{
			name: "running counts finished script jobs, ignoring retried and non-script jobs",
			build: buildkite.Build{Number: 7, State: "running", Jobs: []buildkite.Job{
				script("passed"),
				script("skipped"),
				script("running"),
				script("waiting"),
				{Type: "script", State: "failed", Retried: true},
				{Type: "waiter", State: "passed"},
				{Type: "manual", State: "passed"},
			}},
			want: ansi.ProgramStatus{State: ansi.ProgramStateWorking, Progress: 50, HasProgress: true, Message: "app #7 running"},
		},
		{
			name:  "no script jobs yet is indeterminate",
			build: buildkite.Build{Number: 7, State: "scheduled"},
			want:  ansi.ProgramStatus{State: ansi.ProgramStateWorking, Message: "app #7 scheduled"},
		},
		{
			name:  "failing is still working",
			build: buildkite.Build{Number: 7, State: "failing", Jobs: []buildkite.Job{script("failed"), script("running"), script("running")}},
			want:  ansi.ProgramStatus{State: ansi.ProgramStateWorking, Progress: 33, HasProgress: true, Message: "app #7 failing"},
		},
		{
			name:  "passed",
			build: buildkite.Build{Number: 7, State: "passed", FinishedAt: finished},
			want:  ansi.ProgramStatus{State: ansi.ProgramStateDone, Message: "app #7 passed"},
		},
		{
			name:  "blocked on a block step finishes the watch",
			build: buildkite.Build{Number: 7, State: "blocked", FinishedAt: finished},
			want:  ansi.ProgramStatus{State: ansi.ProgramStateDone, Message: "app #7 blocked"},
		},
		{
			name:  "failed",
			build: buildkite.Build{Number: 7, State: "failed", FinishedAt: finished},
			want:  ansi.ProgramStatus{State: ansi.ProgramStateError, Message: "app #7 failed"},
		},
		{
			name:  "canceled without finished time",
			build: buildkite.Build{Number: 7, State: "canceled"},
			want:  ansi.ProgramStatus{State: ansi.ProgramStateError, Message: "app #7 canceled"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := watchStatus("app", tt.build); got != tt.want {
				t.Errorf("watchStatus() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWatchStoppedStatus(t *testing.T) {
	expired, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-expired.Done()

	// WatchBuild wraps the last poll error, which may itself be a request timeout.
	pollTimeout := fmt.Errorf("fetching build status (10 consecutive errors): %w", context.DeadlineExceeded)

	if got := watchStoppedStatus(context.Background(), pollTimeout); got.State != ansi.ProgramStateError || got.Message != pollTimeout.Error() {
		t.Errorf("poll timeouts with a live watch = %+v, want error with message", got)
	}
	if got := watchStoppedStatus(expired, context.DeadlineExceeded); got.State != ansi.ProgramStateIdle {
		t.Errorf("watch --timeout = %+v, want idle", got)
	}
}

func TestWatchFilterBroken(t *testing.T) {
	got := watchJobs([]buildkite.Job{
		{State: "passed"},
		{State: "broken"},
		{State: "skipped"},
		{State: "failed"},
		{State: "waiting_failed"},
	})
	want := []buildkite.Job{
		{State: "passed"},
		{State: "skipped"},
		{State: "failed"},
		{State: "waiting_failed"},
	}

	if !slices.EqualFunc(got, want, func(a, b buildkite.Job) bool {
		return a.State == b.State
	}) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
