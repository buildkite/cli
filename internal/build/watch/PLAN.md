# Job Tracking & Rendering Plan

## Goal

Track job state changes across polls, separated from rendering.
Provide two rendering modes: live (TTY) and plain (non-TTY).

## Architecture

```
WatchBuild (poll loop)
    │
    ▼
JobTracker.Update(build) → BuildStatus (pure data)
    │
    ├── Live renderer (Screen/Region)
    └── Plain renderer (fmt.Printf)
```

State tracking lives in `internal/build/watch/`.
Rendering lives in `cmd/preflight/`.

## File Layout

- `internal/build/watch/tracker.go` — `JobTracker`, internal types, `Update()` method
- `internal/build/watch/tracker_test.go` — unit tests for state transitions
- `cmd/preflight/preflight.go` — rendering logic in `onStatus` callbacks

## Data Structures

### Internal (unexported, inside JobTracker)

```go
type trackedJob struct {
    Job       buildkite.Job
    PrevState string  // state from previous poll, "" if first seen
    Reported  bool    // true once surfaced to caller as failed
}

type trackedGroup struct {
    Name    string
    Order   int            // insertion order for stable rendering
    Jobs    []*trackedJob  // pointers into tracker.jobs
    Flushed bool           // true once group completion surfaced to caller
}
```

### Exported

```go
type FailedJob struct {
    Name       string
    ID         string
    State      string        // "failed", "timed_out"
    ExitStatus *int
    Duration   time.Duration
    SoftFailed bool
    GroupName  string        // non-empty if part of a parallel group (phase 2)
}

type ParallelGroup struct {  // phase 2
    Name       string
    Total      int
    Passed     int
    Failed     int
    Running    int
    Waiting    int
    FailedJobs []FailedJob
}

type BuildStatus struct {
    // Events
    NewlyFailed     []FailedJob
    CompletedGroups []ParallelGroup  // phase 3

    // Snapshots
    Running      []buildkite.Job
    TotalRunning int
    Summary      JobSummary
    Groups       []ParallelGroup  // phase 2: active parallel groups

    Build buildkite.Build
}

type JobTracker struct {
    jobs   map[string]*trackedJob
    groups map[string]*trackedGroup  // phase 2: keyed by StepKey
    order  int                       // phase 2
}

const MaxRunning = 10
```

### Helpers

```go
func jobDisplayName(j buildkite.Job) string
    j.Name → j.Label → j.Type + " step"

func jobDuration(j buildkite.Job) time.Duration
    StartedAt to FinishedAt (or now), truncated to second

func isTerminalState(state string) bool
    passed, failed, canceled, timed_out, skipped, broken, not_run

func isFailedJob(j buildkite.Job) bool
    state in {failed, timed_out} OR j.SoftFailed

func isActiveState(state string) bool
    running, canceling, timing_out
```

---

## Phase 1: Basic Job Tracking

Track individual job state transitions. No parallel group awareness.
All parallel jobs treated as individual jobs.

### tracker.go (phase 1)

```go
type JobTracker struct {
    jobs map[string]*trackedJob
}
```

### Update() — phase 1

```
1. For each job in b.Jobs:
   a. Skip if job.Type != "script" or job.State == "broken"
   b. Upsert into t.jobs[job.ID]:
      - New: trackedJob{Job: job, PrevState: ""}
      - Existing: PrevState = old State, update Job
   c. If isFailedJob(job) AND !isFailedJob(prev) AND !tj.Reported:
      → append to NewlyFailed, set Reported = true
   d. If isActiveState(job.State): collect into Running

2. Build Summary from all tracked jobs
3. Cap Running to MaxRunning, set TotalRunning
4. Return BuildStatus{NewlyFailed, Running, TotalRunning, Summary, Build}
```

### BuildStatus — phase 1

```go
type BuildStatus struct {
    NewlyFailed  []FailedJob
    Running      []buildkite.Job
    TotalRunning int
    Summary      JobSummary
    Build        buildkite.Build
}
```

### Rendering — phase 1

**Live (TTY):**
- `failedRegion.AppendLine()` for each NewlyFailed (exit status, duration, ID)
- `jobsRegion.SetLines()` with: up to 10 running jobs, summary line, spinner

**Plain (non-TTY):**
- Print each NewlyFailed once: `✗ name  state  id`
- Print summary line when changed
- After build: print full failed job recap

### Tests — phase 1

1. First poll — failures reported as NewlyFailed
2. Same data second poll — no NewlyFailed
3. running → failed transition — appears in NewlyFailed
4. SoftFailed flag — appears in NewlyFailed
5. Running capped at MaxRunning
6. Skips non-script and broken jobs
7. New job appears mid-build (pipeline upload)
8. Summary counts are correct

---

## Phase 2: Parallel Group Tracking

Group parallel jobs by StepKey. Show active groups in the jobs region.
Failed parallel sub-jobs still appear individually in NewlyFailed (same as phase 1).

### tracker.go changes

```go
type JobTracker struct {
    jobs   map[string]*trackedJob
    groups map[string]*trackedGroup  // keyed by StepKey (fallback to display name)
    order  int
}
```

### Update() — phase 2 additions

```
After step 1a, before classification:
   e. Determine group key:
      key = j.StepKey (fallback to jobDisplayName if empty)
   f. If parallel (ParallelGroupTotal != nil && *ParallelGroupTotal > 1):
      - Upsert into t.groups[key]
      - Add trackedJob pointer to group.Jobs
      - Reset group counters each poll (Total/Passed/Failed/Running/Waiting)
      - Increment counters based on current state

After all jobs:
   g. Build status.Groups from active (not fully done) groups, sorted by Order
   h. Parallel running jobs counted in TotalRunning but NOT in Running slice
      (they are represented by their group in status.Groups instead)
```

### BuildStatus — phase 2 additions

```go
Groups []ParallelGroup  // active parallel groups, stable insertion order
```

### Rendering — phase 2 additions

**Live (TTY) jobsRegion:**
- Non-parallel running jobs: `● name  running  duration`
- Active groups: `● name  3 running, 2 passed  (5)`
- Summary line + spinner (unchanged)

### Tests — phase 2

1. Parallel jobs grouped by StepKey
2. Fallback to display name when StepKey empty
3. Group counters reset each poll
4. Parallel running jobs excluded from Running slice, included in TotalRunning
5. Groups sorted by insertion order
6. Failed parallel sub-job still appears in NewlyFailed individually
7. Mixed parallel and non-parallel jobs

---

## Phase 3: Group Completion Promotion

When a parallel group becomes fully done and has failures, promote it
to CompletedGroups so the renderer can show it in the failed region.

### Update() — phase 3 additions

```
After building groups (phase 2 step g):
   i. For each group:
      - done = (Passed + Failed + Canceled + Skipped) == Total
      - If done AND !group.Flushed:
        - If group has failures:
          → Build ParallelGroup snapshot with FailedJobs
          → Append to status.CompletedGroups
        - Set group.Flushed = true
      - If done: exclude from status.Groups (no longer active)
```

### BuildStatus — phase 3 additions

```go
CompletedGroups []ParallelGroup  // groups that just finished, with their FailedJobs
```

### Rendering — phase 3 additions

**Live (TTY):**
- For each CompletedGroups with failures:
  - `failedRegion.AppendLine()` group header: `✗ name  2/5 failed`
  - `failedRegion.AppendLine()` each failed sub-job indented

**Plain (non-TTY):**
- Print group completion line when it appears in CompletedGroups

### Tests — phase 3

1. Group completes all passed — Flushed set, not in CompletedGroups
2. Group completes with failures — appears in CompletedGroups with FailedJobs
3. Completed group not re-reported next poll (Flushed)
4. Completed group removed from active Groups list
5. Group with mix of failed and soft-failed sub-jobs

---

## What Doesn't Change

- `WatchBuild()` — poll loop, consecutive error handling, terminal state detection
- `JobSummary` / `Summarize()` — reused by tracker internally
- `Screen` / `Region` — unchanged
- `cmd/build/watch.go` — keeps using `Summarize()` directly
