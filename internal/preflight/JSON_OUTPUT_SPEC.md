# Preflight JSON Output Spec

This document defines the `v1` JSON output contract for `bk preflight`.

## Goals

- Support machine-readable output for `bk preflight`.
- Use the shared output package for JSON output.
- Keep the existing plain non-TTY renderer available behind `--text`.
- Keep the schema focused on actionable build, job, test, and lint information.

## Output Modes

- `bk preflight --json` emits a single JSON document using the shared output package.
- `bk preflight --text` emits the existing plain text output used for non-TTY stdout today.
- Interactive TTY rendering remains unchanged unless `--json` is explicitly requested.
- `--json` and `--text` are mutually exclusive.

## JSON Format

- `--json` uses the shared output package in [pkg/output/output.go](file:///Users/matthewborden/cli/pkg/output/output.go).
- Output is a single standard JSON document.
- Output is pretty-printed with indentation, consistent with the rest of the CLI.
- Output is terminated by a newline.
- Output contains no ANSI escape sequences or other human-oriented formatting.
- JSON output represents the final known preflight state, not a stream of intermediate updates.

## Top-Level Schema

The final JSON document has this shape:

```json
{
  "status": "failed",
  "duration_ms": 23400,
  "build_url": "https://buildkite.com/org/pipeline/builds/123",
  "jobs": {
    "failures": [
      {
        "id": "019d-job-id",
        "name": "Unit Tests",
        "state": "failed",
        "soft_failed": false,
        "exit_status": 1,
        "duration_ms": 91000,
        "web_url": "https://buildkite.com/org/pipeline/builds/123#job-019d-job-id"
      }
    ]
  },
  "tests": {
    "failures": [
      {
        "id": "54c5b928-2e76-8592-bb5e-329c0c66ce45",
        "name": "doesn't pass at all",
        "location": "./spec/flaky_spec.rb:10",
        "labels": [],
        "executions_count": 2,
        "reliability": 0,
        "message": "Failure/Error: expect(1 + 1).to eq(3)",
        "details": [
          "expected: 3",
          "got: 2",
          "(compared using ==)"
        ],
        "backtrace": [
          "./spec/flaky_spec.rb:11:in 'block (2 levels) in <top (required)>'"
        ]
      }
    ]
  },
  "lint": {
    "errors": 0,
    "warnings": 1
  }
}
```

## Top-Level Fields

### `status`

- Type: string
- Required: yes
- Meaning: the overall final preflight state.

Supported values:

- `preparing`
- `running`
- `passed`
- `failed`
- `canceled`
- `skipped`
- `not_run`
- `error`

### `duration_ms`

- Type: integer
- Required: yes
- Meaning: wall-clock elapsed time in milliseconds for the command.

### `build_url`

- Type: string
- Required: when the build has been created
- Meaning: Buildkite web URL for the preflight build.

If the build has not been created yet, this field is omitted.

### `jobs`

- Type: object
- Required: when build data is available
- Meaning: actionable job failures only.

`jobs` intentionally does not include summary counts such as `passed`, `failed`, `running`, or `skipped`.

### `tests`

- Type: object
- Required: when test failure data is available
- Meaning: actionable test failures only.

`tests` intentionally does not include summary counts such as `passed`, `failed`, or `skipped`.

### `lint`

- Type: object
- Required: when lint data is available
- Meaning: lint summary counts.

## `jobs` Schema

```json
{
  "failures": [
    {
      "id": "019d-job-id",
      "name": "Unit Tests",
      "state": "failed",
      "soft_failed": false,
      "exit_status": 1,
      "duration_ms": 91000,
      "web_url": "https://buildkite.com/org/pipeline/builds/123#job-019d-job-id"
    }
  ]
}
```

### `jobs.failures`

- Type: array
- Required: yes when `jobs` is present
- Empty value: `[]`

### `jobs.failures[*]`

- `id`: Buildkite job ID.
- `name`: human-readable job name.
- `state`: terminal job state, for example `failed`, `timed_out`, `canceled`, or `expired`.
- `soft_failed`: whether the job is marked soft-failed.
- `exit_status`: numeric exit code when available.
- `duration_ms`: elapsed runtime in milliseconds when available.
- `web_url`: Buildkite job URL when available.

## `tests` Schema

```json
{
  "failures": [
    {
      "id": "54c5b928-2e76-8592-bb5e-329c0c66ce45",
      "name": "doesn't pass at all",
      "location": "./spec/flaky_spec.rb:10",
      "labels": [],
      "executions_count": 2,
      "reliability": 0,
      "message": "Failure/Error: expect(1 + 1).to eq(3)",
      "details": [
        "expected: 3",
        "got: 2",
        "(compared using ==)"
      ],
      "backtrace": [
        "./spec/flaky_spec.rb:11:in 'block (2 levels) in <top (required)>'"
      ]
    }
  ]
}
```

### `tests.failures`

- Type: array
- Required: yes when `tests` is present
- Empty value: `[]`

### `tests.failures[*]`

- `id`: test identifier from the upstream tests API.
- `name`: human-readable test name.
- `location`: literal `location` value from the upstream tests API. This value is not parsed or normalized.
- `labels`: labels from the upstream tests API.
- `executions_count`: execution count from the upstream tests API.
- `reliability`: reliability value from the upstream tests API.
- `message`: top-level failure message.
- `details`: optional list of expanded failure detail lines.
- `backtrace`: optional list of backtrace lines.

## `lint` Schema

```json
{
  "errors": 0,
  "warnings": 1
}
```

### `lint.errors`

- Type: integer
- Required: yes when `lint` is present

### `lint.warnings`

- Type: integer
- Required: yes when `lint` is present

## Normalization Rules

### Jobs

- Only failed or soft-failed jobs are included in `jobs.failures`.
- Non-failing jobs are not emitted in JSON output.
- `duration_ms` is derived from job timestamps when available.

### Tests

- Test failures are normalized from the upstream tests API response.
- The nested `latest_fail` wrapper is flattened into the fields on each failure object.
- `location` uses the exact upstream string value.
- `details` is flattened from the upstream expanded failure details.
- `backtrace` is flattened from the upstream expanded failure backtrace entries.

### Omitted Upstream Test Fields

The following upstream test fields are intentionally excluded from the `v1` preflight JSON contract:

- `scope`
- `state`
- `latest_fail.id`
- `latest_fail.timestamp`
- `latest_fail.duration`

## Presence Rules

- Unknown data is omitted rather than populated with placeholder values.
- `build_url` is omitted until the build exists.
- `jobs` is omitted until build job data is available.
- `tests` is omitted until test failure data is available.
- `lint` is omitted until lint data is available.

## Non-Goals For `v1`

- Snapshot commit, ref, and file list details.
- Job or test summary counts.
- Emitting raw upstream API payloads unchanged.
- Streaming JSON updates during execution.
