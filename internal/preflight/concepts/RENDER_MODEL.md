# Preflight Render Model

This note captures what the `bubblespike` demo is doing and how `bk preflight` can match that layout with a smaller rendering model.

## Observations From `bubblespike`

- The visual model is simpler than the current `Screen` and `Region` API.
- In TTY mode there are only two behaviors:
  - append new lines to scrollback
  - redraw one live summary area at the bottom
- In text mode the same logical events are emitted linearly.
- In JSON mode the same logical events are emitted structurally.
- The source file [`../bubblespike.go`](file:///Users/matthewborden/bubblespike.go) is the best reference. The compiled demo at `/tmp/bubblespike.1vacvg` appears older: it supports `-json`, but not the `-text` flag that exists in source.

The important point is that the shared model is not "multiple mutable regions". The shared model is "append events" plus "replace the current summary state".

## What Preflight Does Today

Current preflight rendering is spread across mode-specific methods in [cmd/preflight/render.go](file:///Users/matthewborden/cli/cmd/preflight/render.go) and a region manager in [internal/preflight/screen.go](file:///Users/matthewborden/cli/internal/preflight/screen.go).

That has a few mismatches with the demo layout:

- The renderer interface is phase-specific: `appendSnapshotLine`, `setSnapshot`, `renderStatus`, `renderTestFailures`, `renderFinalFailures`.
- TTY rendering is built around five mutable regions: snapshot, title, failed, summary, and result.
- `watch.BuildStatus` still carries `Running` and `TotalRunning` for a separate running-jobs section.
- Tests in [cmd/preflight/render_test.go](file:///Users/matthewborden/cli/cmd/preflight/render_test.go) are asserting the old multi-region shape.

That is more structure than the `bubblespike` layout needs.

## Target Model

Use a single `Event` entity.

```go
type EventType string

const (
    EventStatus      EventType = "status"
    EventJobFailure  EventType = "job_failure"
    EventTestFailure EventType = "test_failure"
)

type Event struct {
    Type EventType
    Time time.Time

    PreflightID string

    Operation   string
    Pipeline    string
    BuildNumber int
    BuildURL    string
    BuildState  string

    Jobs watch.JobSummary

    Job  *buildkite.Job
    Test *buildkite.BuildTest
}
```

Why this is enough:

- `Event` is the only thing producers emit.
- the latest `status` event is the live summary/footer state.
- every `job_failure` event is appended to scrollback once.
- every `test_failure` event is appended to scrollback once.
- `PreflightID` ties every event back to the same run.
- `Time` gives text and JSON mode a stable timestamp and lets TTY optionally prefix appended lines.

This is intentionally not optimized for rendering. It is just the smallest complete data model.

## How Each Phase Maps To The Model

Before the build exists:

- `Event{Type: EventStatus, Time: now, PreflightID: id, Operation: "Creating snapshot of working tree..."}`
- `Event{Type: EventStatus, Time: now, PreflightID: id, Operation: "Creating build on buildkite/cli..."}`

After the build exists:

- `Event{Type: EventStatus, Time: now, PreflightID: id, Pipeline: "buildkite/cli", BuildNumber: n, BuildURL: url, BuildState: b.State, Jobs: status.Summary}`

During watch:

- each poll emits a fresh `status` event with the latest counts
- each newly failed job emits one `job_failure` event
- each newly failed test emits one `test_failure` event

At completion:

- emit one final `status` event with the terminal `BuildState`

This matches the `bubblespike` layout: the stream contains durable failure events plus a replaceable current status.

## Rendering Implications

The renderer should consume `Event` values and project them differently by mode.

### TTY

- `status` replaces the live summary area at the bottom
- `job_failure` appends formatted job failure lines to scrollback
- `test_failure` appends formatted test failure lines to scrollback

TTY still only needs two visible behaviors:

- append to scrollback
- rewrite one footer block

That means `Screen` and `Region` are still unnecessary.

### Text

- `status` prints a timestamped status line when the visible status changes
- `job_failure` and `test_failure` print timestamped lines immediately

### JSON

- emit one JSON object per `Event`
- keep field names close to the `Event` struct so JSON is just a serialization of the data model

## Implications For Current Types

With this model, preflight no longer needs a separate running-jobs panel.

That means these fields in [internal/build/watch/tracker.go](file:///Users/matthewborden/cli/internal/build/watch/tracker.go) likely become unnecessary for preflight:

- `BuildStatus.Running`
- `BuildStatus.TotalRunning`

`BuildStatus.NewlyFailed` and `BuildStatus.Summary` are still useful because they map directly to emitted `Event` values.

The command in [cmd/preflight/preflight.go](file:///Users/matthewborden/cli/cmd/preflight/preflight.go) can own `PreflightID`, `Pipeline`, and `BuildURL`, then attach them to each event as it emits them.

## Migration Plan

Each step is a self-contained, shippable change. Steps 1–4 are independent of each other and can land in any order. Step 5 integrates them. Step 6 cleans up.

### Step 1 — Add `Event` type

Add `Event` and `EventType` in a new file `cmd/preflight/event.go`. Pure addition, no callers, no behavior change.

```go
type EventType string

const (
    EventStatus      EventType = "status"
    EventJobFailure  EventType = "job_failure"
    EventTestFailure EventType = "test_failure"
)

type Event struct {
    Type        EventType
    Time        time.Time
    PreflightID string
    Operation   string
    Pipeline    string
    BuildNumber int
    BuildURL    string
    BuildState  string
    Jobs        watch.JobSummary
    Job         *buildkite.Job
    Test        *buildkite.BuildTest
}
```

Tests: compiles, existing tests still pass.

### Step 2 — Add `plainRenderer.Render(Event)`

Add a `Render(Event)` method to `plainRenderer` in [cmd/preflight/render.go](file:///Users/matthewborden/cli/cmd/preflight/render.go). Not yet called by `preflight.go`. The method switches on `Event.Type`:

- `EventStatus` with `Operation` set → print timestamped operation line
- `EventStatus` with `BuildState` set → print timestamped build status + summary (deduplicated against `lastLine`)
- `EventJobFailure` → print formatted job failure via `plainJobPresenter`
- `EventTestFailure` → print formatted test failure

Add a `Close()` method (no-op for plain).

Tests: unit test `plainRenderer.Render` with each event type.

### Step 3 — Add bubbletea dependency

```sh
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/bubbles/spinner
go get github.com/charmbracelet/lipgloss
```

No code changes. Just `go.mod` / `go.sum`.

### Step 4 — Add bubbletea TTY renderer

Create `cmd/preflight/tty.go` with a bubbletea model similar to [`bubblespike.go`](file:///Users/matthewborden/bubblespike.go). New file, not wired into `preflight.go` yet.

The model receives `Event` values via `p.Send()`:

- `EventStatus` → store as latest status, `View()` re-renders the footer (spinner + build state + job summary)
- `EventJobFailure` / `EventTestFailure` → return `tea.Printf(formatted)` to append above the footer

Public API:

```go
type ttyEventRenderer struct {
    program *tea.Program
}

func newTTYEventRenderer(pipeline string, buildNumber int) *ttyEventRenderer
func (r *ttyEventRenderer) Render(e Event)  // calls r.program.Send(e)
func (r *ttyEventRenderer) Close()          // calls r.program.Quit() and r.program.Wait()
```

`View()` renders:
- a separator line
- spinner + status text (operation or "Watching build #N (state)")
- job summary counts (when watching)

This is the same layout as `bubblespike.go` lines 377–389.

Tests: not easily unit-testable (bubbletea owns the terminal). Verify via manual `go run` or a small integration test.

### Step 5 — Wire up: switch `preflight.go` to emit events

Change the `renderer` interface to:

```go
type renderer interface {
    Render(Event)
    Close()
}
```

Update `newRenderer()` to return `*ttyEventRenderer` (step 4) for TTY, `*plainRenderer` (step 2) for non-TTY.

Change [cmd/preflight/preflight.go](file:///Users/matthewborden/cli/cmd/preflight/preflight.go) to emit `Event` values instead of calling the old methods:

- Snapshot phase: `renderer.Render(Event{Type: EventStatus, Operation: "Creating snapshot..."})`
- Build creation: `renderer.Render(Event{Type: EventStatus, Operation: "Creating build..."})`
- Watch loop: emit `EventStatus` for each poll, `EventJobFailure` for each newly failed job
- Completion: `renderer.Render(Event{Type: EventStatus, BuildState: finalState})` then `renderer.Close()`

Remove the old `ttyRenderer` struct, old renderer methods, and `renderFinalFailures` / `finalResultLines`.

Update [cmd/preflight/render_test.go](file:///Users/matthewborden/cli/cmd/preflight/render_test.go) to assert against `Render(Event)` instead of the old methods.

### Step 6 — Delete `Screen` and `Region`

Remove [internal/preflight/screen.go](file:///Users/matthewborden/cli/internal/preflight/screen.go) and [internal/preflight/screen_test.go](file:///Users/matthewborden/cli/internal/preflight/screen_test.go). No remaining callers after step 5.

## Recommendation

Treat the renderer as a projection of a single event stream, not as a screen layout engine.

That keeps all three output modes aligned:

- TTY: latest `status` event as the footer, failures appended to scrollback
- text: serialized `Event` values
- JSON: structured `Event` values

Once preflight is shaped that way, removing `Screen` and `Region` becomes a simplification instead of a feature loss.
