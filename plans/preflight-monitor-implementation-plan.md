# Plan: Implement `bk preflight monitor`

## Goal

Add a small `bk preflight monitor` command that finds and watches the Buildkite build already created for the current pushed branch and `HEAD` commit.

Primary workflow:

```sh
git commit -am "..."
git push
bk preflight monitor --pipeline org/pipeline
```

This command should avoid duplicate CI. It must not create a snapshot branch, create a new build, push anything, or set `PREFLIGHT=true`.

## V1 Scope

V1 should do one thing well:

> Find the Buildkite build for the current Git branch and current `HEAD` commit, wait briefly if it has not appeared yet, then watch it with preflight-style output.

Keep the command intentionally narrow. Defer broader build lookup features until users need them.

## Non-goals

- Do not set or emulate `PREFLIGHT=true` on existing builds.
- Do not create a fallback build if no matching build is found.
- Do not support arbitrary `--branch` or `--commit` lookup in V1.
- Do not support direct `--build <number>` lookup in V1; `bk build watch` already covers direct build watching.
- Do not add `--no-watch` in V1; a command named `monitor` should monitor.
- Do not change existing `bk preflight` / `bk preflight run` snapshot behaviour.
- Do not implement publish/promote behaviour from preflight snapshot branches.

## User-facing Behaviour

### Command

```sh
bk preflight monitor --pipeline org/pipeline
```

### V1 flags

| Flag | Purpose |
| --- | --- |
| `--pipeline`, `-p` | Same pipeline selector as `bk preflight run`. |
| `--wait-for-build` | How long to wait for the webhook-created build to appear. Default: `2m`. |
| `--interval` | Polling interval while looking for and watching the build. |
| `--exit-on` | Reuse existing preflight exit policy behaviour. |
| `--text` / `--json` | Reuse existing preflight renderer modes. |

Do not include `--branch`, `--commit`, `--build`, `--no-watch`, or `--await-test-results` in the first version.

### Expected flow

```diagram
╭──────────────╮
│ Local branch │
╰──────┬───────╯
       │ git push
       ▼
╭──────────────────────╮
│ Buildkite webhook CI │
╰──────┬───────────────╯
       │ existing build for current branch + HEAD
       ▼
╭──────────────────────╮
│ bk preflight monitor │
╰──────┬───────────────╯
       │ watch existing build
       ▼
╭──────────────────────╮
│ Preflight-style      │
│ result and summary   │
╰──────────────────────╯
```

## Technical Approach

### 1. Add `MonitorCmd`

Add a new command in the `cmd/preflight` package, likely:

```text
cmd/preflight/monitor.go
```

Register it under `PreflightCmd` in `main.go`:

```go
PreflightCmd struct {
    Run     preflight.RunCmd     `cmd:"" default:"withargs" help:"Run a build against a snapshot of the local working tree (experimental)"`
    Monitor preflight.MonitorCmd `cmd:"" help:"Monitor the existing build for the current branch and commit (experimental)"`
    Cleanup preflight.CleanupCmd `cmd:"" help:"Clean up completed preflight branches (experimental)"`
}
```

Keep `RunCmd` as the default command so `bk preflight` behaviour does not change.

### 2. Reuse existing setup

Use the existing preflight `setup()` helper for:

- factory creation,
- experiment gate,
- repository root resolution,
- signal-aware context,
- pipeline resolution,
- rate-limit transport.

This keeps `monitor` aligned with `run` without adding new setup paths.

### 3. Resolve current branch and `HEAD`

Use `internal/preflight.ResolveSourceContext`.

Validation rules:

- If the branch is empty, fail clearly. V1 does not support detached `HEAD`.
- If the commit cannot be resolved, fail clearly.
- Do not require a clean working tree. The command only monitors the pushed `HEAD` commit.

Suggested detached `HEAD` error:

```text
could not determine branch for build lookup

Detached HEAD cannot be matched to a Buildkite branch automatically.
Check out a branch before running `bk preflight monitor`.
```

### 4. Find the existing build

Add a small lookup helper that polls `Builds.ListByPipeline` by branch and commit.

Query shape:

```go
opts := &buildkite.BuildsListOptions{
    Branch: []string{source.Branch},
    Commit: source.Commit,
    ListOptions: buildkite.ListOptions{PerPage: 1},
}
```

Polling behaviour:

- Poll until a matching build is found.
- Stop when `--wait-for-build` expires.
- Stop when the command context is canceled.

If no build is found, return a user-facing error with suggestions:

```text
No Buildkite build found for branch my-branch at abc123.

Suggestions:
- Ensure the commit has been pushed.
- Ensure the selected pipeline runs for this branch.
- Try again in a few seconds if the push just happened.
```

Use a normal validation/user error for this case. Preflight result exit codes should only apply after a build has been found and watched.

### 5. Extract the smallest shared watch helper

`RunCmd.Run` currently mixes snapshot/build creation with build watching and result rendering. Extract only the generic watching portion needed by both `run` and `monitor`.

The shared helper should:

- watch a known `buildkite.Build`,
- render build status events,
- render newly failed jobs,
- render retry-passed jobs,
- render the final summary,
- return the same result error mapping as current preflight.

The shared helper should not:

- create builds,
- clean up branches,
- cancel builds,
- know whether the build came from snapshot preflight or webhook CI.

Keep cleanup and cancellation decisions in the callers:

- `RunCmd` keeps cleaning up preflight branches.
- `RunCmd` keeps canceling preflight-created builds when exiting early on `build-failing`.
- `MonitorCmd` never cleans up branches.
- `MonitorCmd` never cancels the existing CI build.

### 6. Watch the found build

After lookup succeeds:

1. Render an operation event showing the found build number and URL.
2. Pass the build into the shared watch helper.
3. Return the helper's result.

Use existing event types where possible. Prefer `EventOperation` for lookup/found messages and avoid adding new JSON event types in V1.

## Test Plan

Add focused tests in `cmd/preflight/monitor_test.go`.

V1 test coverage:

1. Finds a build for the current branch and `HEAD` commit.
   - Assert the API request includes `branch[]` and `commit`.
2. Waits for a delayed webhook build.
   - First list response is empty; second returns a build.
3. Times out clearly when no build appears.
   - Assert the error mentions push/pipeline suggestions.
4. Watches a found build to success.
   - Assert the command exits successfully.
5. Exits early on `build-failing` without canceling the build.
   - Assert no cancel API request is made.

Existing `RunCmd` tests should continue covering snapshot-specific behaviour:

- snapshot branch creation,
- `PREFLIGHT=true` on preflight-created builds,
- branch cleanup,
- cancellation of preflight-created builds on early failure.

## Documentation Updates

Update preflight help text to describe the two workflows:

```text
bk preflight run
  Snapshot local working tree changes to a temporary preflight branch and run CI.

bk preflight monitor
  Monitor the existing CI build for the current pushed branch and commit.
```

Add examples:

```sh
# Test uncommitted or local-only changes
bk preflight run --pipeline org/pipeline

# Commit, push, then monitor the real CI build
git commit -am "..."
git push
bk preflight monitor --pipeline org/pipeline
```

Document the important limitation:

```text
`bk preflight monitor` watches an existing build. It does not set `PREFLIGHT=true` because the build environment is fixed when the build is created.
```

## Implementation Checklist

- [ ] Add `MonitorCmd` and register it under `PreflightCmd`.
- [ ] Reuse existing preflight setup.
- [ ] Resolve current branch and `HEAD` commit.
- [ ] Fail clearly for detached `HEAD`.
- [ ] Implement build lookup by current branch and commit.
- [ ] Implement wait loop for delayed webhook builds.
- [ ] Extract the smallest shared watch/render helper from `RunCmd.Run`.
- [ ] Update `RunCmd.Run` to use the helper without changing behaviour.
- [ ] Wire `MonitorCmd` into the helper without cleanup or cancellation.
- [ ] Add the five focused monitor tests.
- [ ] Update help text and examples.
- [ ] Run `mise run format`.
- [ ] Run targeted tests for `cmd/preflight` and `internal/preflight`.
