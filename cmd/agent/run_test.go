package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

func TestRunAgentRevokesToken(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake agent")
	}

	t.Run("when the agent fails", func(t *testing.T) {
		t.Parallel()

		f, revoked := newTokenServer(t)
		dir := t.TempDir()
		agent := writeFakeAgent(t, dir, "exit 1")

		err := runAgent(context.Background(), f, "test-org", "cluster-1", agent, dir, "default", make(chan os.Signal, 1))
		if err == nil {
			t.Fatal("expected an error when the agent fails")
		}

		if got := revoked(); got != "token-1" {
			t.Errorf("revoked tokens = %q, want %q", got, "token-1")
		}
	})

	t.Run("when bk is stopped with SIGTERM", func(t *testing.T) {
		t.Parallel()

		f, revoked := newTokenServer(t)
		dir := t.TempDir()
		sigCh, errCh := startAgent(t, f, dir)

		// The terminal delivers Ctrl+C to the agent itself, so bk must not
		// pass it on. SIGTERM is usually sent to bk alone, so bk must.
		sigCh <- os.Interrupt
		// Give the agent time to record a signal if bk wrongly passed one on.
		time.Sleep(200 * time.Millisecond)
		sigCh <- syscall.SIGTERM
		waitForRunAgent(t, errCh)

		if got := readAgentLog(t, dir); got != "started\nTERM\n" {
			t.Errorf("agent log = %q, want %q", got, "started\nTERM\n")
		}
		if got := revoked(); got != "token-1" {
			t.Errorf("revoked tokens = %q, want %q", got, "token-1")
		}
	})

	t.Run("when the terminal is closed", func(t *testing.T) {
		t.Parallel()

		f, revoked := newTokenServer(t)
		dir := t.TempDir()
		sigCh, errCh := startAgent(t, f, dir)

		// Closing the terminal sends bk SIGHUP twice. A second SIGTERM would
		// make the agent cancel its job instead of finishing it.
		sigCh <- syscall.SIGHUP
		waitForAgentLog(t, dir, "started\nTERM\n")
		sigCh <- syscall.SIGHUP
		waitForRunAgent(t, errCh)

		if got := readAgentLog(t, dir); got != "started\nTERM\n" {
			t.Errorf("agent log = %q, want %q", got, "started\nTERM\n")
		}
		if got := revoked(); got != "token-1" {
			t.Errorf("revoked tokens = %q, want %q", got, "token-1")
		}
	})
}

// startAgent runs a fake agent through runAgent until it receives SIGTERM, and
// returns the channel to send bk's signals on and runAgent's result.
func startAgent(t *testing.T, f *factory.Factory, dir string) (chan<- os.Signal, <-chan error) {
	t.Helper()

	// Run for up to 10 seconds, until SIGTERM, then stay up briefly to record
	// any further signals. Waiting on a background sleep lets the shell run
	// traps straight away; shells drop a SIGINT that arrives while a
	// foreground child runs.
	agent := writeFakeAgent(t, dir, `for i in $(seq 100); do [ -n "$stopping" ] && break; sleep 0.1 & wait $!; done
sleep 0.5 & wait $!`)

	sigCh := make(chan os.Signal, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- runAgent(context.Background(), f, "test-org", "cluster-1", agent, dir, "default", sigCh)
	}()

	waitForAgentLog(t, dir, "started\n")
	return sigCh, errCh
}

func waitForRunAgent(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runAgent() error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runAgent did not return after the agent was stopped")
	}
}

// newTokenServer fakes the cluster token endpoints. It returns a factory that
// uses it and a function that lists the revoked token IDs.
func newTokenServer(t *testing.T) (*factory.Factory, func() string) {
	t.Helper()

	var mu sync.Mutex
	var revoked []string

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const tokens = "/v2/organizations/test-org/clusters/cluster-1/tokens"
		switch {
		case r.Method == http.MethodPost && r.URL.Path == tokens:
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id": "token-1", "token": "secret-value"}`)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, tokens+"/"):
			mu.Lock()
			revoked = append(revoked, strings.TrimPrefix(r.URL.Path, tokens+"/"))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
	if err != nil {
		t.Fatal(err)
	}

	return &factory.Factory{RestAPIClient: client}, func() string {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(revoked, ",")
	}
}

// writeFakeAgent writes a script that stands in for buildkite-agent. It checks
// it was given the created token, logs that it started and any signals it
// receives, then runs body. SIGTERM sets $stopping.
func writeFakeAgent(t *testing.T, dir, body string) string {
	t.Helper()

	script := fmt.Sprintf(`#!/bin/sh
log=%q
trap 'echo INT >> "$log"' INT
trap 'echo TERM >> "$log"; stopping=1' TERM
grep -q 'token="secret-value"' "$3" || exit 3
echo started >> "$log"
%s
`, filepath.Join(dir, "agent.log"), body)

	path := filepath.Join(dir, "fake-agent")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitForAgentLog(t *testing.T, dir, want string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for readAgentLog(t, dir) != want {
		if time.Now().After(deadline) {
			t.Fatalf("agent log = %q, want %q", readAgentLog(t, dir), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readAgentLog(t *testing.T, dir string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dir, "agent.log"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}
