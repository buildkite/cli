// Package programstatus reports what bk is doing to the terminal using the
// Program Status Protocol (OSC 7501), so terminals can show whether bk is
// working, waiting on the user, or finished.
//
// Reports go to stderr and are only sent when stderr is a terminal outside CI.
// Set BUILDKITE_PROGRAM_STATUS=false to disable them or =true to always send
// them.
//
// See: https://www.superlogical.com/rex/docs/build/program-status
package programstatus

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-isatty"
)

const app = "bk"

var std = newReporter(defaultWriter())

// Report sends s as the status of bk, tagged with the bk app name.
func Report(s ansi.ProgramStatus) {
	std.report(s)
}

// Finish reports how long-running work ended, leaving a record the user can
// come back to: done with msg if err is nil, idle if the user canceled it, and
// error otherwise.
func Finish(err error, msg string) {
	Report(outcome(err, msg))
}

func outcome(err error, msg string) ansi.ProgramStatus {
	switch {
	case err == nil:
		return ansi.ProgramStatus{State: ansi.ProgramStateDone, Message: msg}
	case errors.Is(err, context.Canceled):
		return ansi.ProgramStatus{State: ansi.ProgramStateIdle}
	default:
		return ansi.ProgramStatus{State: ansi.ProgramStateError, Message: err.Error()}
	}
}

// Block reports that bk is blocked on the user. Call the returned function once
// the user has answered to restore the status reported before blocking.
func Block(kind ansi.ProgramStatusKind, msg string) (unblock func()) {
	return std.block(kind, msg)
}

type reporter struct {
	mu   sync.Mutex
	w    io.Writer
	last ansi.ProgramStatus
}

func newReporter(w io.Writer) *reporter {
	return &reporter{w: w}
}

func (r *reporter) report(s ansi.ProgramStatus) {
	if r.w == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = s
	r.write(s)
}

func (r *reporter) block(kind ansi.ProgramStatusKind, msg string) func() {
	if r.w == nil {
		return func() {}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.write(ansi.ProgramStatus{State: ansi.ProgramStateBlocked, Kind: kind, Message: msg})
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		// bk carries on after the user answers, so it is working even if it
		// had not reported anything before blocking.
		last := r.last
		if last.State == "" {
			last.State = ansi.ProgramStateWorking
		}
		r.write(last)
	}
}

func (r *reporter) write(s ansi.ProgramStatus) {
	s.App = app
	if seq := ansi.SetProgramStatus(s); seq != "" {
		_, _ = io.WriteString(r.w, seq)
	}
}

func defaultWriter() io.Writer {
	tty := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	if !enabled(os.Getenv, tty) {
		return nil
	}
	return os.Stderr
}

func enabled(getenv func(string) string, stderrTTY bool) bool {
	if on, err := strconv.ParseBool(getenv("BUILDKITE_PROGRAM_STATUS")); err == nil {
		return on
	}
	// Buildkite agents run jobs in a PTY, so a terminal check alone would put
	// these sequences in job logs.
	if getenv("CI") != "" || getenv("BUILDKITE") != "" {
		return false
	}
	return stderrTTY && getenv("TERM") != "dumb"
}
