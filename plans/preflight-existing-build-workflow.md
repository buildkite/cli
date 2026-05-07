# Plan: Preflight Existing Build Workflow

## Context

Preflight currently creates a synthetic branch, runs CI against that branch, and then cleans it up. That works well for testing uncommitted local changes, but it is wasteful when a developer has already committed and pushed their changes: CI runs once for the preflight branch, then the developer still needs to push their real branch and wait for CI again.

The goal is to support a workflow where a developer can commit, push, and ask `bk preflight` to monitor the Buildkite build that already belongs to that commit.

## Current Behaviour

Preflight currently:

1. Resolves the user's current branch and commit as source metadata.
2. Creates a new synthetic commit from the working tree.
3. Pushes that commit to `bk/preflight/<uuid>`.
4. Creates a Buildkite build against that synthetic branch and commit with `PREFLIGHT=true`.
5. Watches the build, renders failures and summaries, then cleans up the temporary branch.

```diagram
Current:
╭──────────────╮
│ Local branch │
╰──────┬───────╯
       │ snapshot
       ▼
╭──────────────────────╮
│ bk/preflight/<uuid>  │
╰──────┬───────────────╯
       │ create build with PREFLIGHT=true
       ▼
╭──────────────────────╮
│ Preflight CI build   │
╰──────────────────────╯
```

Key implementation points:

- `cmd/preflight/preflight.go` defines `RunCmd`, creates the preflight build, and watches it.
- `internal/preflight/snapshot.go` creates and pushes the synthetic `bk/preflight/<uuid>` commit.
- `internal/preflight/git.go` resolves the source branch and commit.
- `internal/build/watch/watch.go` already provides the generic build polling loop used by preflight.

## Option 1: Add `bk preflight monitor` for the Current Pushed Commit

Add a new subcommand:

```sh
git commit -am "..."
git push
bk preflight monitor --pipeline org/pipeline
```

This command would not create a snapshot and would not create a new Buildkite build. Instead, it would find the Buildkite build that already exists for the user's current branch and `HEAD` commit, then reuse the existing preflight watcher UI and summary output.

### Implementation Shape

- Add `MonitorCmd` under the existing preflight command group.
- Reuse existing setup logic for API client, experiment gate, repository root, signal context, and pipeline resolution.
- Resolve the current branch and commit with the existing source context helper.
- Query `Builds.ListByPipeline` with:
  - `Branch: []string{currentBranch}`
  - `Commit: currentHEAD`
  - `PerPage: 1`
- Poll for a short window because webhook-created builds may appear a few seconds after `git push`.
- Once found, call the same watch/render/summary path currently used after preflight build creation.
- Add useful flags:
  - `--commit <sha>` for explicit commit monitoring.
  - `--branch <branch>` for detached `HEAD` or non-current branch use.
  - `--wait-for-build 2m` to control how long to wait for webhook-created CI.
  - `--build <number>` as an escape hatch to monitor a known build directly.

### Pros

- Directly solves the wasteful “preflight passed, now push real branch and wait again” loop.
- Avoids duplicate CI builds.
- Keeps existing snapshot-based preflight behaviour intact.
- Builds run exactly as normal branch CI, so branch filters, PR behaviour, and commit statuses match the real workflow.
- Can reuse existing Buildkite list/watch patterns.

### Cons and Caveats

- The existing build will not have `PREFLIGHT=true`, because environment variables cannot be added after the build is created.
- Any pipeline logic that depends on `PREFLIGHT` will not run in this mode.
- Requires the commit to be pushed and CI to be triggered by the normal webhook or API path.
- Need to confirm the preflight summary endpoint works for non-`PREFLIGHT` builds; otherwise fall back to regular build and test summary data.

### Recommendation

This should be the first implementation. It cleanly supports the desired workflow:

```sh
git commit
git push
bk preflight monitor
```

## Option 2: Add a Real Branch Preflight Build Mode

Add a mode like:

```sh
bk preflight run --source=head
# or
bk preflight run --real-branch
```

Instead of snapshotting to `bk/preflight/<uuid>`, this would create a Buildkite build using:

- `Commit: HEAD`
- `Branch: currentBranch`
- `Env: PREFLIGHT=true`

Preflight would still create a build, but the build would target the real branch and real commit rather than a synthetic branch.

```diagram
Option 2:
╭──────────────╮
│ Local branch │
╰──────┬───────╯
       │ HEAD commit
       ▼
╭────────────────────────────╮
│ Buildkite build on branch  │
│ with PREFLIGHT=true        │
╰────────────────────────────╯
```

### Implementation Shape

- Add a flag to `RunCmd`, for example `--source=head` or `--real-branch`.
- If real-branch mode is enabled:
  - Require a named branch.
  - Require `HEAD` to be pushed, or optionally push it.
  - Skip snapshot creation.
  - Create the build using the current branch and current commit instead of the synthetic branch and commit.
  - Skip preflight branch cleanup.

### Pros

- Preserves `PREFLIGHT=true`.
- The tested commit is the real branch commit.
- Avoids the second post-preflight push/wait cycle.

### Cons and Caveats

- If pushing the branch also triggers CI automatically, this can still create duplicate builds: one webhook build and one API-created preflight build.
- Unlike the current snapshot workflow, this cannot safely include uncommitted changes unless the command also commits them.
- Adds behavioural complexity inside `preflight run`.

### Best Fit

Use this if `PREFLIGHT=true` is important and teams are comfortable with preflight explicitly creating a build on the real branch.

## Option 3: Keep Snapshot Mode, but Add Publish or Promote on Pass

Keep the current snapshot workflow, but add a flag or separate command:

```sh
bk preflight run --publish-on-pass
```

or:

```sh
bk preflight promote <preflight-id>
```

After the synthetic preflight build passes, the CLI would fast-forward the original branch to the already-tested snapshot commit and push it.

```diagram
Option 3:
╭──────────────╮
│ Local changes│
╰──────┬───────╯
       │ snapshot
       ▼
╭──────────────────────╮
│ bk/preflight/<uuid>  │
╰──────┬───────────────╯
       │ CI passes
       ▼
╭──────────────────────╮
│ Push same commit to  │
│ original branch      │
╰──────────────────────╯
```

### Safety Requirements

- Only allow fast-forward publish.
- Verify the source branch still points to the original `PREFLIGHT_SOURCE_COMMIT`.
- Verify the remote branch has not moved unexpectedly.
- Require confirmation unless `--yes` is provided.
- Do not amend or rewrite the snapshot commit after CI passes, because that would change the SHA and invalidate the CI result.

### Pros

- Preserves the current “test my dirty worktree” experience.
- The exact commit that passed CI becomes the real branch commit.
- Avoids running CI again after publish, assuming commit statuses/checks are commit-based.

### Cons and Caveats

- The build still ran on `bk/preflight/<uuid>`, not the real branch, so branch-specific pipeline logic may differ.
- The resulting commit message is currently `Preflight snapshot`, unless preflight gains a `--message` flag before snapshot creation.
- Higher risk because the CLI would push to the user's real branch.
- More edge cases around branch movement and user intent.

### Best Fit

Useful as an advanced follow-up, but it should not be the first fix.

## Suggested Implementation Sequence

1. Extract shared build watching logic.
   - Move the existing post-build-create watcher and summary code from `RunCmd.Run` into a helper that accepts an existing `buildkite.Build`.
   - This allows both snapshot preflight and monitor mode to share output, exit policies, test summaries, and JSON/text rendering.
2. Implement Option 1: `bk preflight monitor`.
   - Add `MonitorCmd`.
   - Resolve branch and commit.
   - Poll `Builds.ListByPipeline` by branch and commit until found or timeout.
   - Watch the found build with the shared watcher.
3. Add tests.
   - Finds a build by current branch and `HEAD` commit.
   - Waits until a webhook-created build appears.
   - Fails clearly when the commit is not pushed or no build exists.
   - Allows explicit `--commit` and `--branch`.
   - Reuses existing exit policy behaviour.
4. Document the new workflow.
   - `bk preflight run`: snapshot local working tree.
   - `bk preflight monitor`: monitor CI for an already-pushed commit.
5. Reassess whether Option 2 or Option 3 is still needed.
   - If users need `PREFLIGHT=true` on real branch builds, implement Option 2.
   - If users want dirty-worktree preflight to become the real branch commit, consider Option 3 with strong safety checks.

## Recommended Direction

Ship Option 1 first:

```sh
git commit
git push
bk preflight monitor
```

Keep the current snapshot-based `bk preflight run` workflow for uncommitted or experimental local changes. This gives users both modes without making either workflow surprising.
