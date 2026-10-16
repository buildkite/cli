package programstatus

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestEnabled(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		tty  bool
		want bool
	}{
		{"terminal", nil, true, true},
		{"not a terminal", nil, false, false},
		{"dumb terminal", map[string]string{"TERM": "dumb"}, true, false},
		{"CI", map[string]string{"CI": "true"}, true, false},
		{"Buildkite job", map[string]string{"BUILDKITE": "true"}, true, false},
		{"disabled", map[string]string{"BUILDKITE_PROGRAM_STATUS": "false"}, true, false},
		{"forced in CI without a terminal", map[string]string{"BUILDKITE_PROGRAM_STATUS": "1", "CI": "true"}, false, true},
		{"unrecognised value falls back to detection", map[string]string{"BUILDKITE_PROGRAM_STATUS": "auto", "CI": "true"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string { return tt.env[k] }
			if got := enabled(getenv, tt.tty); got != tt.want {
				t.Errorf("enabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutcome(t *testing.T) {
	failed := errors.New("build is failed")
	tests := []struct {
		name string
		err  error
		want ansi.ProgramStatus
	}{
		{"success", nil, ansi.ProgramStatus{State: ansi.ProgramStateDone, Message: "Downloaded"}},
		{"user canceled", fmt.Errorf("user aborted: %w", context.Canceled), ansi.ProgramStatus{State: ansi.ProgramStateIdle}},
		{"request timeout is a failure", fmt.Errorf("download: %w", context.DeadlineExceeded), ansi.ProgramStatus{State: ansi.ProgramStateError, Message: "download: context deadline exceeded"}},
		{"failure", failed, ansi.ProgramStatus{State: ansi.ProgramStateError, Message: "build is failed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outcome(tt.err, "Downloaded"); got != tt.want {
				t.Errorf("outcome() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBlockRestoresPreviousStatus(t *testing.T) {
	var out strings.Builder
	r := newReporter(&out)

	r.report(ansi.ProgramStatus{State: ansi.ProgramStateWorking, Progress: 40, HasProgress: true, Message: "Loading"})
	unblock := r.block(ansi.ProgramStatusKindPermission, "Continue?")
	unblock()

	want := []map[string]string{
		{"state": "working", "progress": "40", "app": "bk", "msg": "Loading"},
		{"state": "blocked", "kind": "permission", "app": "bk", "msg": "Continue?"},
		{"state": "working", "progress": "40", "app": "bk", "msg": "Loading"},
	}
	assertReports(t, out.String(), want)
}

func TestBlockWithoutPreviousStatusRestoresWorking(t *testing.T) {
	var out strings.Builder
	r := newReporter(&out)

	r.block(ansi.ProgramStatusKindQuestion, "Select a pipeline")()

	want := []map[string]string{
		{"state": "blocked", "kind": "question", "app": "bk", "msg": "Select a pipeline"},
		{"state": "working", "app": "bk"},
	}
	assertReports(t, out.String(), want)
}

// assertReports decodes OSC 7501 reports into key/value pairs, with msg decoded.
func assertReports(t *testing.T, out string, want []map[string]string) {
	t.Helper()
	seqs := strings.SplitAfter(out, "\x07")
	if seqs[len(seqs)-1] != "" {
		t.Fatalf("output does not end with a terminated report: %q", out)
	}
	seqs = seqs[:len(seqs)-1]
	if len(seqs) != len(want) {
		t.Fatalf("got %d reports, want %d: %q", len(seqs), len(want), out)
	}
	for i, seq := range seqs {
		body, ok := strings.CutPrefix(seq, "\x1b]7501;")
		if !ok {
			t.Fatalf("report %d is not OSC 7501: %q", i, seq)
		}
		got := map[string]string{}
		for pair := range strings.SplitSeq(strings.TrimSuffix(body, "\x07"), ":") {
			k, v, _ := strings.Cut(pair, "=")
			if k == "msg" {
				decoded, err := base64.StdEncoding.DecodeString(v)
				if err != nil {
					t.Fatalf("report %d msg is not base64: %q", i, v)
				}
				v = string(decoded)
			}
			got[k] = v
		}
		if !maps.Equal(got, want[i]) {
			t.Errorf("report %d = %v, want %v", i, got, want[i])
		}
	}
}
