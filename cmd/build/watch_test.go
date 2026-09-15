package build

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	buildkite "github.com/buildkite/go-buildkite/v5"
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
