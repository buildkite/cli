# PRD: Enhanced `bk job log` Command

## Context

The Buildkite MCP server provides three powerful log tools (`read_logs`, `search_logs`, `tail_logs`) backed by the `buildkite-logs` Go library, which downloads raw logs from the REST API, converts them to Parquet format for efficient columnar querying, and caches them locally. The current CLI's `bk job log` is minimal — it fetches the entire log via REST API and pipes it to a pager with no search, pagination, tailing, or follow capabilities. This PRD designs the enhancement of `bk job log` to replicate MCP server capabilities in a CLI-native way.

**Why now:** CI/CD debugging is the #1 CLI use case. Users currently copy-paste job UUIDs, wait for full log downloads, and manually grep through output. The `buildkite-logs` library already solves the hard problems (Parquet conversion, caching, efficient search). The CLI just needs to wire it up.

---

## Goals

1. **Parity with MCP server log tools** — read with seek/limit, regex search with context, tail last N lines
2. **Live follow mode** — `tail -f` equivalent for running jobs, with 2-second polling
3. **Interactive job picker** — eliminate the need to copy-paste job UUIDs
4. **Industry-standard UX** — grep-familiar flags (-g, -C, -A, -B, -v, -i), matches kubectl/Railway/Heroku conventions
5. **Backward compatible** — existing `bk job log JOB_ID -b 123` works identically

---

## Command Interface

```
bk job log [JOB_ID] [-b BUILD] [-p PIPELINE] [flags]
```

JOB_ID becomes **optional** — when omitted, an interactive job picker is presented.

### Reading Flags
| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--seek` | | int | -1 | Start reading from row N (0-based) |
| `--limit` | | int | 0 | Maximum number of lines to output |
| `--tail` | `-n` | int | 0 | Show last N lines |
| `--follow` | `-f` | bool | false | Follow log output for running jobs (2s poll) |

### Search Flags
| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--grep` | `-g` | string | "" | Regex pattern to search for |
| `--context` | `-C` | int | 0 | Lines of context around each match |
| `--after-context` | `-A` | int | 0 | Lines after each match |
| `--before-context` | `-B` | int | 0 | Lines before each match |
| `--ignore-case` | `-i` | bool | true | Case-insensitive search (negate with `--no-ignore-case`) |
| `--invert-match` | `-v` | bool | false | Show non-matching lines |

### Display Flags
| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--no-timestamps` | | bool | false | Strip `bk;t=\d+` timestamp markers (existing) |
| `--no-color` | | bool | false | Strip ANSI codes from log output |
| `--line-numbers` | | bool | false | Prefix each line with its row number |

### Flag Constraints
- `--tail` and `--seek` are mutually exclusive
- `--follow` cannot combine with `--grep` or `--seek`
- `--grep` is required for `-C`, `-A`, `-B`, `-v` flags

### Usage Examples
```bash
# Full log (existing behavior, now powered by buildkite-logs)
bk job log 019004-e199-453b -p my-pipeline -b 123

# Interactive job picker (omit job ID)
bk job log -p my-pipeline -b 123

# Last 50 lines
bk job log JOB_ID -b 123 -n 50

# Follow a running job's output
bk job log JOB_ID -b 123 -f

# Search for errors with 3 lines of context
bk job log JOB_ID -b 123 -g "error|failed|panic" -C 3

# Case-sensitive search, inverted
bk job log JOB_ID -b 123 -g "SUCCESS" --no-ignore-case -v

# Paginated read (rows 100-200)
bk job log JOB_ID -b 123 --seek 100 --limit 100

# With line numbers, no color (for piping)
bk job log JOB_ID -b 123 --line-numbers --no-color
```

---

## Architecture

### Dependency: `buildkite-logs` v0.8.0

The same library the MCP server uses. It provides:
- **`Client`** — downloads raw logs via REST API, converts to Parquet, caches in blob storage (`~/.bklog`)
- **`ParquetReader`** — efficient columnar queries: `ReadEntriesIter()`, `SeekToRow()`, `SearchEntriesIter()`, `GetFileInfo()`
- **`ParquetLogEntry`** — `RowNumber`, `Timestamp`, `Content`, `Group`, `Flags`, `CleanContent(stripANSI)`
- **`SearchResult`** — `Match`, `BeforeContext`, `AfterContext`
- **`SearchOptions`** — `Pattern`, `CaseSensitive`, `InvertMatch`, `Context`, `BeforeContext`, `AfterContext`

The library accepts `*buildkite.Client` (from `go-buildkite/v4`) which the CLI already creates via Factory. Integration is trivial.

### Client Initialization

**New file: `internal/logs/client.go`**

```go
func NewLogsClient(ctx context.Context, restClient *buildkite.Client) (*buildkitelogs.Client, error) {
    storageURL := os.Getenv("BKLOG_CACHE_URL") // Same env var as MCP server
    return buildkitelogs.NewClient(ctx, restClient, storageURL)
}
```

- Default storage: `~/.bklog` (local filesystem, auto-created)
- Override via `BKLOG_CACHE_URL` env var (supports `file://`, `s3://`, `gcs://`)
- Client created per command invocation, closed via `defer`
- NOT added to Factory — it's command-specific

### Mode Dispatch in Run()

```
Run()
├── 1. Factory + validation (existing)
├── 2. Pipeline/build resolution (existing AggregateResolver)
├── 3. Job resolution (new: interactive picker if JobID empty)
├── 4. Flag validation (mutual exclusivity checks)
├── 5. Create buildkite-logs client
├── 6. Mode dispatch:
│   ├── --grep set    → searchMode()
│   ├── --tail > 0    → tailMode()
│   ├── --follow      → followMode()
│   └── default       → readMode() (handles --seek, --limit, or full read)
└── 7. Output to pager (or stdout for --follow)
```

### Mode Implementations

#### readMode
1. Create `ParquetReader` via `client.NewReader(org, pipeline, build, job, 30s TTL, false)`
2. If `--seek >= 0`: use `reader.SeekToRow(seek)`, else `reader.ReadEntriesIter()`
3. Iterate entries, apply `--limit` if set
4. Write each entry via `writeEntry()` to pager

#### tailMode
1. Create `ParquetReader`
2. `GetFileInfo()` for total row count
3. Calculate `startRow = max(totalRows - tail, 0)`
4. `SeekToRow(startRow)`, iterate to end
5. Write each entry to pager

#### searchMode
1. Validate regex pattern early with `regexp.Compile()`
2. Create `ParquetReader`
3. Build `SearchOptions` from flags
4. Iterate `SearchEntriesIter(opts)`, apply `--limit` if set
5. Write each `SearchResult` via `writeSearchResult()` to pager
6. Search results formatted grep-style: context lines, match line, `--` separator between groups

#### followMode
1. **No pager** — writes directly to stdout
2. Track `lastSeenRow` (starts at 0, or at `totalRows - tail` if `--tail` also provided)
3. Poll loop every 2 seconds:
   a. Create reader with `TTL: 0, forceRefresh: true` to bypass cache
   b. `GetFileInfo()` to check current row count
   c. If new rows: `SeekToRow(lastSeenRow)`, write new entries, update `lastSeenRow`
   d. Close reader (cleanup temp files)
   e. Check job state via REST API — exit if terminal (passed, failed, canceled, etc.)
   f. Wait 2s or exit on Ctrl-C / context cancellation
4. Clean exit with signal handling (`SIGINT`, `SIGTERM`)

### Output Formatting

#### writeEntry(writer, entry)
```
1. Get content from entry.Content (preserves ANSI by default)
2. If --no-color OR piped (not TTY): strip ANSI via library's CleanContent(true)
3. If --no-timestamps: strip bk;t=\d+\x07 patterns (existing regex)
4. TrimRight trailing newlines
5. If --line-numbers: prefix with row number (right-aligned, 6 chars)
6. Write to writer with trailing newline
```

#### writeSearchResult(writer, result)
```
1. Write before-context entries (dimmed if color enabled)
2. Write match entry (normal/highlighted)
3. Write after-context entries (dimmed if color enabled)
4. Write "--" separator between match groups (grep convention)
```

#### ANSI Auto-detection
- **TTY output or pager**: Pass ANSI codes through. Pager is `less -R` which handles raw ANSI.
- **Piped/redirected**: Strip ANSI codes automatically.
- **`--no-color` flag**: Force stripping regardless.
- **`NO_COLOR` env var**: Already handled by existing `output.ColorEnabled()`.

### Interactive Job Picker

When `JobID` is empty and terminal is interactive (`!NoInput && isTTY`):

1. Fetch build via REST API: `Builds.Get(org, pipeline, buildNumber)`
2. Filter to `type == "script"` jobs (skip wait/trigger/block steps)
3. If only 1 job: auto-select it
4. If multiple: present numbered list via existing `io.PromptForOne()`:
   ```
   Select a job:
   1. Run tests (passed) - 0190046e-e199-453b-a302-a21a4d649d31
   2. Deploy staging (failed) - 0190046e-e199-453b-a302-a21a4d649d32
   3. Integration tests (running) - 0190046e-e199-453b-a302-a21a4d649d33
   ```
5. Extract job UUID from selection

---

## Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `go.mod` | Modify | Add `github.com/buildkite/buildkite-logs v0.8.0` |
| `go.sum` | Auto | `go mod tidy` |
| `internal/logs/client.go` | **New** | `NewLogsClient()` helper |
| `cmd/job/log.go` | **Rewrite** | Enhanced LogCmd struct, Run() flow, all mode functions, output formatting |
| `cmd/job/log_test.go` | **New** | Unit tests for flags, formatting, validation |
| `main.go` | No change | Already references `job.LogCmd` |

### Key Reference Files
- `buildkite-mcp-server/pkg/buildkite/joblogs.go` — Reference implementation for all three modes
- `buildkite-logs@v0.8.0/client.go` — `NewClient()`, `NewReader()` API surface
- `buildkite-logs@v0.8.0/query.go` — `ParquetReader` methods
- `pkg/cmd/factory/factory.go` — How `RestAPIClient` is created (line 176)
- `internal/io/pager.go` — Pager creation pattern
- `internal/io/prompt.go` — Interactive picker pattern
- `pkg/output/color.go` — `ColorEnabled()` for TTY/NO_COLOR detection
- `cmd/build/watch.go` + `internal/build/watch/watch.go` — Polling pattern reference for follow mode

---

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Missing API token | Factory fails at creation (existing) |
| Can't create `~/.bklog` cache dir | Clear error suggesting `BKLOG_CACHE_URL` override |
| API 404 (wrong job ID) | "Job not found. Verify the job UUID and build number." |
| API 401 (expired token) | "Authentication failed. Run `bk auth login` to re-authenticate." |
| `ErrLogTooLarge` (>10MB) | "Log exceeds 10MB. Use `--tail N` or `--seek/--limit` to read a portion." |
| Invalid regex in `--grep` | Validate early before creating reader. "Invalid regex pattern: ..." |
| Empty log (0 rows) | "No log output for this job." (exit 0) |
| `--follow` on terminal job | Print existing log, then "Job already finished (state: passed)." (exit 0) |
| Seek beyond end of file | "Row N is beyond log end (total: M rows). Use --tail or a smaller --seek value." |
| Network error during follow | Retry silently up to 10 consecutive failures (matches `bk build watch` pattern), then error |

---

## Edge Cases

1. **Very large logs**: `ErrLogTooLarge` caught and user directed to `--tail`/`--seek`
2. **Running jobs with no output yet**: RowCount=0. Follow mode keeps polling. Other modes: "No log output yet."
3. **Retried jobs**: Library handles `RetrySource` internally
4. **Cancelled jobs mid-follow**: Terminal state detected, follow exits cleanly
5. **Non-TTY + --follow**: Works fine, streams to stdout. No spinner.
6. **WSL cross-filesystem**: Default blob storage in `~/.bklog` avoids cross-device temp file issues
7. **Job ID format**: Validate UUID format early. GraphQL-style IDs should be rejected with guidance.

---

## Testing Strategy

### Unit Tests (`cmd/job/log_test.go`)
1. **Flag validation**: `--tail` + `--seek` conflict, `--follow` + `--grep` conflict, `--grep` required for context flags
2. **Output formatting**: `writeEntry` with line numbers, ANSI stripping, timestamp stripping
3. **Search result formatting**: Context lines, separators
4. **Job picker filtering**: Only `type == "script"` jobs, single-job auto-select
5. **stripTimestamps**: Existing regex behavior preserved

### Manual Integration Testing
```bash
# Against a real pipeline:
bk job log -b LATEST_BUILD                    # test job picker
bk job log JOB -b BUILD -n 20                 # test tail
bk job log JOB -b BUILD -g "error" -C 2       # test search
bk job log JOB -b BUILD --seek 0 --limit 10   # test paginated read
bk job log JOB -b BUILD -f                    # test follow on running job
bk job log JOB -b BUILD | head -5             # test piped (no pager, no color)
bk job log JOB -b BUILD --line-numbers        # test line numbers
```

---

## Implementation Sequence

1. `go.mod` — add `buildkite-logs v0.8.0`, run `go mod tidy`
2. `internal/logs/client.go` — `NewLogsClient()` helper
3. `cmd/job/log.go` — rewrite in stages:
   a. Update `LogCmd` struct with all flags, update `Help()`
   b. Refactor `Run()` with job picker + mode dispatch skeleton
   c. Implement `readMode()` first (closest to existing, validates library integration)
   d. Implement `tailMode()`
   e. Implement `searchMode()` with `writeSearchResult()`
   f. Implement `followMode()` (most complex, do last)
   g. Implement `writeEntry()` with ANSI auto-detection
4. `cmd/job/log_test.go` — unit tests
5. Manual integration testing
