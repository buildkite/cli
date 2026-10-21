# bk - The Buildkite CLI

[![Latest Release](https://img.shields.io/github/v/release/buildkite/cli?include_prereleases&sort=semver&display_name=release&logo=buildkite)](https://github.com/buildkite/cli/releases)

A command line interface for [Buildkite](https://buildkite.com/).

Full documentation is available at [buildkite.com/docs/platform/cli](https://buildkite.com/docs/platform/cli).

## Quick Start

### Install

```sh
brew tap buildkite/buildkite && brew install buildkite/buildkite/bk
```

Or download a binary from the [releases page](https://github.com/buildkite/cli/releases).

To update a standalone release-binary install later, run:

```sh
bk update
```

If `bk` is managed by Homebrew or mise, `bk update` will tell you how to update
it with that tool instead.

### Authenticate

```sh
bk auth login
```

Stored credentials are selected per organization. When targeting another
organization, authenticate it explicitly:

```sh
bk auth login --org other-org
```

Logging in also selects that organization in your user-wide configuration.
Repository configuration and `BUILDKITE_ORGANIZATION_SLUG` still take precedence.
Use the global `--org` flag to select an organization for one invocation without
changing either configuration file. It overrides `BUILDKITE_ORGANIZATION_SLUG`
and repository/user defaults, and selects that organization's stored credentials:

```sh
bk build list --org other-org
bk build view 123 --pipeline my-pipeline --org other-org
bk --org other-org job list
bk agent list --org other-org
```

The flag is available to all commands, including cluster, queue, secret, team,
cache registry, repository connection, package, and API commands. Authentication
and configuration commands retain their existing credential/selection changes.
Pipeline selections discovered from the repository are not cached when using
`--org` or `BUILDKITE_ORGANIZATION_SLUG`, so a temporary organization cannot
overwrite the saved pipeline defaults. Existing cached pipeline names are only
used when the effective organization matches the saved organization; otherwise
provide `--pipeline` or let repository discovery find a pipeline in the target org.

A qualified pipeline or build target (including a URL) selects its organization
for the invocation in the same way as `--org`, including that organization's
stored credentials. Omit `--org` when the target already specifies the desired
organization; when both are given, or two targets name different organizations,
the conflict produces an error before execution:

```sh
bk build list --pipeline other-org/my-pipeline
bk build view 123 --pipeline other-org/my-pipeline
bk build view https://buildkite.com/other-org/my-pipeline/builds/123
bk job list --pipeline https://buildkite.com/other-org/my-pipeline --build 123
```

For `pipeline copy`, `--org` selects the source organization; `--target org/name`
can still specify a different destination organization. A qualified source such
as `bk pipeline copy other-org/their-pipeline --target my-copy` reads from
`other-org` but still creates the copy in the selected organization. `bk browse`
builds its URL from the target without calling the API, so it does not require
credentials for the target's organization.

The CLI does not fall back to another organization's stored credentials. For
automation or a token with access to multiple organizations, supply
`BUILDKITE_API_TOKEN` explicitly. This environment token takes precedence over
stored credentials and is not replaced by stored OAuth credentials if a request
fails authentication.

### Debug HTTP requests

Use `--debug` to log HTTP API requests and responses to stderr. Logged request
URLs omit userinfo, query strings and fragments. Common credential headers
(`Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`)
and selected token/password/private-key body fields are redacted; cluster-secret
create/update request bodies are omitted.

This is not comprehensive sanitisation. Arbitrary bodies, URL paths and other
headers can still contain sensitive data. Review debug logs before sharing or
retaining them; do not treat them as safe telemetry.

### Create a pipeline

From a Git checkout, `bk pipeline create` infers the repository URL and default
name from `origin` (or uses the checkout directory name if no origin exists).
An explicit name and `--repository` override those defaults. When used
interactively without a name or repository argument, it offers a recommended
local starter and alternatives if no pipeline file exists:

Guided creation defaults to `Pipeline created: <URL>` on stdout, while file
status and starter prompts go to stderr. Format flags and explicitly configured
output preferences take precedence; named and scripted output defaults are
unchanged.

```sh
bk pipeline create
bk pipeline create "My Pipeline" --repository git@github.com:org/repo.git
bk pipeline create --no-input --template go
bk pipeline create --dry-run --json
```

Named invocations, redirected input and `--no-input` do not add prompts or local
files unless `--template` is supplied. Dry-run resolves the same name and URL
but never writes local files or creates a pipeline/webhook; it still performs
API reads and requires the existing authentication configuration.

`bk init` uses the same starter selection without creating a remote pipeline.
Without interactive input it uses the project recommendation; `--template`
selects `go`, `node`, `pipenv`, `ruby` or `empty` explicitly. Both commands search
the checkout root, including from subdirectories and linked worktrees, and leave
existing pipeline files unchanged.

Starters are bundled adaptations of Buildkite's
[Go](https://github.com/buildkite/golang-example),
[Node](https://github.com/buildkite/nodejs-example),
[Pipenv](https://github.com/buildkite/python-pipenv-example) and
[Ruby](https://github.com/buildkite/ruby-rbenv-example) examples. Go runs test/vet;
Node requires a real test script and adapts installation to npm, pnpm, Yarn or
Bun lockfiles. Pipenv/pytest and Bundler/RSpec recommendations require dependency
evidence and lockfiles. Agents must have the matching runtime and package manager
installed; these starters do not provision agents, Docker, hooks or caches. The
empty starter contains `steps: []` and needs editing.

Review generated files, then **commit and push them before building**. Remote
builds cannot see local starters; creating a pipeline does not publish them.

### Compare builds

```sh
bk build compare 123 --pipeline my-pipeline --json
bk build compare acme/my-pipeline/123 --baseline 120 --text
bk build compare https://buildkite.com/acme/my-pipeline/builds/123 --yaml
# Inspect newly failing target jobs without interpreting every unchanged row:
bk build compare acme/my-pipeline/123 --json |
  jq '.steps[] | select(.change == "newly_failing") | .target | {id, web_url, retries_count}'
```

Requires `read_builds`. The required target accepts the usual build selectors;
plain numbers use `--pipeline` or configuration/repository inference. JSON/YAML
never prompts for pipeline selection; specify `--pipeline` when ambiguous.
Configured/environment `no_input` and `quiet` settings are preserved alongside
the global CLI flags, including when selecting another organization's credentials.
`--baseline` is a different positive build number in the **same pipeline**. It
may be newer, failed or on another branch (cross-branch comparisons warn).
Without it, search at most five pages/500 candidates in API newest-first order
for a lower-numbered, strictly earlier-created build on the exact target branch
that is **currently passed**. It need not have passed before the target started.
Missing branch/creation time or no eligible baseline is an error: supply
`--baseline`. Missing/denied builds, failed pages, and inventories exceeding ten
pages/1000 jobs per build also fail with no partial output, not false additions.

Only final attempts (`retried=false`, including running attempts) are compared.
Identity includes step key, type, matrix and parallel index **and total**, keeping
absent coordinates distinct from zero. Keyed renames/group moves still match.
Unkeyed jobs use an exact name plus group, type, matrix and parallel coordinates,
reported as `match_method: name_fallback` with a heuristic warning. Keys never
match names. Duplicate identities on either side leave **all affected jobs on
both sides unmatched**; unnamed unkeyed jobs also remain unmatched. Renames of
unkeyed jobs and changed coordinates appear as added/removed, not proven changes
to the same step. Unfinished builds warn: added/removed is snapshot presence only.

JSON/YAML contains `organization`, `pipeline`, `target`/`baseline` metadata
(ID, number, state, branch, commit, URL, creation time), `baseline_selection`,
`candidates_scanned`, final-attempt `target_jobs`/`baseline_jobs`, `change_counts`,
`steps` and `warnings`. Every comparison row is returned, with no output truncation.
Empty inventories produce `steps: []` and `change_counts: {}`. Counts count rows,
so an ambiguous identity can contribute separate target and baseline rows.
Each row includes identity fields, optional `match_method`/`unmatched_reason`,
and available `target`/`baseline` attempts with IDs, names, states, `soft_failed`,
exit status/signal reason, retry counts, URLs and valid timing metrics.
If a job URL is missing but its step ID is available, its URL opens that step in
the corresponding build instead. Existing job URLs are preserved.

Outcome precedence and display order are:

| Change | Meaning |
| --- | --- |
| `newly_failing` | Passed → failed/timed_out/expired, with unchanged soft-failure flag |
| `recovered` | Failed/timed_out/expired → passed, with unchanged soft-failure flag |
| `still_failing` | Both states are failed/timed_out/expired, with unchanged soft-failure flag |
| `state_changed` | Soft-failure flag changed (takes precedence over the above), or another state change |
| `retries_changed` | Same outcome/state but a different final-attempt retry count |
| `added` / `removed` | Unique identity present only in target / baseline |
| `unmatched` | Missing or ambiguous identity; no pair inferred |
| `unchanged` | Same outcome/state and retry count; timings may still differ |

One-sided and unmatched rows take precedence over outcome classification. Two
soft-failed attempts can be `still_failing`; cancellations, broken, waiting_failed
and skipped states are not classified as failures. Within each category, execution
deltas sort descending (missing treated as zero), with API order retained for ties.
`execution_seconds` is finished minus started; `scheduling_seconds` is started
minus scheduled. Uploaded jobs can inherit the build's creation time as their
scheduled time, so this interval can include earlier upstream waiting; it is not
isolated scheduler/agent delay. Missing or reversed timestamps omit the metric;
zero is valid. `execution_delta_seconds` is target minus baseline when both are
valid, so positive means slower. These are final-attempt durations, not all-retry
costs or build wall-clock deltas.

Still-failing jobs can have different causes. Comparison does not prove causality
or retry safety. No logs are fetched; follow job URLs/IDs for evidence and use
`bk job --help` for drill-down commands. Text output uses untruncated per-step
blocks; JSON/YAML is preferable for filtering and automated analysis.

### List a build's annotations

```sh
bk build annotations 123 --pipeline my-pipeline --json
bk build annotations acme/my-pipeline/123 --text
bk build annotations https://buildkite.com/acme/my-pipeline/builds/123 --yaml
bk build annotations acme/my-pipeline/123 --json | jq '.annotations[].body_html'
```

The build argument is required. A plain build number uses `--pipeline` (a slug
or `org/pipeline`) or the pipeline inferred from configuration or the repository.
JSON/YAML never prompts for pipeline selection; specify `--pipeline` when
inference is ambiguous. Qualified selectors use the target organization's
credentials. Requires the `read_builds` token scope.

All pages of both build- and job-scoped annotations are fetched. JSON/YAML
includes `organization`, `pipeline`, `build_number`, and `annotations` (an empty
array when there are none). Annotation fields use REST names, including `id`,
`context`, `style`, `scope`, `job_id` when present, `priority`, `body_html`,
`created_at`, and `updated_at`. Bodies are rendered HTML, not original Markdown;
text output also preserves the HTML. `--output/-o json|yaml|text` and the standard
`--json`, `--yaml`, `--text` flags are supported. Text output reports
`No annotations found.` for an empty build. Errors, including a failed later
page, return a nonzero exit status with no partial results on stdout.

### List a build's failed tests

```sh
bk build failed-tests 123 --pipeline my-pipeline --json
bk build failed-tests acme/my-pipeline/123 --text
bk build failed-tests https://buildkite.com/acme/my-pipeline/builds/123 --yaml
```

The build argument is required. A plain build number uses `--pipeline` or the
pipeline inferred from configuration or the repository. JSON/YAML never prompts
for pipeline selection; specify `--pipeline` when inference is ambiguous.
The command fetches all pages of tests with **at least one failed execution**,
including tests that passed on retry, from every Test Engine suite accessible to
your token. It does not restrict results to flaky labels or require the build to
be failed.

JSON/YAML output includes `organization`, `pipeline`, `build_number`, `build_id`,
`failed_tests` (an empty array when there are no matches), and `note`. Each test
includes its identity, source location, links, current labels, and execution
metrics. Individual execution failure messages and backtraces are not included.
Requires `read_builds` and `read_suites` token scopes and uploaded Test Engine data
associated with the build. The API aggregation window is bounded by retention and
ends at most 24 hours after build creation. An empty result does **not** prove
that no tests failed, particularly when data is missing or inaccessible.

Current test labels do not establish that flakiness caused a failure.

### Retry a build's failed jobs

```sh
bk build retry-failed-jobs 123 --pipeline my-pipeline
bk build retry-failed-jobs acme/my-pipeline/123 --states failed,soft_failed --yes --json
bk build retry-failed-jobs https://buildkite.com/acme/my-pipeline/builds/123 --yes --no-input --text
```

The required build argument accepts the same number, qualified selector, and URL
forms as `failed-tests`. JSON/YAML never prompts for pipeline selection; specify
`--pipeline` when inference is ambiguous. Confirmation is required unless `--yes`
is supplied. `--no-input` without `--yes` fails without retrying. Confirmation
prompts go to stderr, leaving structured stdout parseable.
Malformed selectors and conflicting explicit build/pipeline targets are rejected.

This calls the public build-wide REST `retry_failed_jobs` operation and requires
`write_builds`. It is not limited to infrastructure failures and does not check
whether jobs are safe to rerun. Retries can repeat side effects,
including deployments. By default (including `--states ''`), all retryable failed
jobs are retried. `--states` accepts a comma-separated list of `canceled`,
`expired`, `failed`, `soft_failed`, and `timed_out`.

The API returns **202 Accepted**, asynchronously queuing retries in the existing
build. JSON/YAML includes `organization`, `pipeline`, `build_number`, `web_url`,
`retried_jobs_count`, and `note`. The count is jobs queued for retry, including
zero when none were queued, not completed or successful retries. The command
does not wait; use `bk build view` or `bk build watch` to follow the build.
If the request was accepted but its count could not be read, the command fails
without a receipt and warns you to check the build before resubmitting.

### List a build's dynamic step uploads

```sh
bk build step-uploads 123 --pipeline my-pipeline --json
bk build step-uploads acme/my-pipeline/123 --text
bk build step-uploads https://buildkite.com/acme/my-pipeline/builds/123 --yaml
```

The build argument is required and uses the same selectors and prompt-safe
pipeline inference as `failed-tests`. Requires the `read_builds` token scope.
All cursor pages are fetched, newest first. JSON output includes `organization`,
`pipeline`, `build_number`, `step_uploads` (an empty array when none exist), and
`note`. Uploads include their UUID, state (`pending`, `processing`, `applied`,
`rejected`, or `failed`), source job, replace flag, timestamps, created job count,
and any rejection type and outcome message. An unavailable `created_jobs_count`
is omitted in JSON and shown as `-` in text; an applied upload that created no
jobs has a count of `0`.

Rejection/failure messages are available without fetching individual uploads.
They exclude uploaded configuration and internal failure details; inspect the
source job's pipeline upload command output for further diagnostics. Uploaded
definitions are not fetched. The API only retains uploads within the build's
maximum lifetime, up to 30 days after creation. Older builds return a `410 Gone`
error, not an empty successful result. Other API errors also fail the command
without emitting partial results.

### Inspect flaky test failures

List failed tests that Test Engine currently labels flaky for a failed job:

```sh
bk job flaky-tests <job-uuid> --text
bk job flaky-tests <job-uuid> --json
```

The command fetches all pages and includes the job UUID, build UUID, test details,
and execution metrics in structured output (JSON or YAML). It requires
`read_builds` and `read_suites` token scopes and Test Engine executions tagged
with the matching `build.job_id`. Missing or overridden collector job metadata
can prevent matches.

Current flaky labels do not prove that flakiness caused the job failure. An empty
result does **not** mean “not flaky”: Test Engine data may be missing or incomplete.

### Find failures across triggered builds

```sh
bk build view acme/my-pipeline/123 --recursive --json
```

`--recursive` returns current hard-failed (`failed`) and timed-out (`timed_out`)
command jobs from the selected build and its descendants. It follows every
available child reference, including passed, asynchronous, and soft-failed
triggers, without filtering by build state. Superseded retry attempts and the
children of superseded trigger attempts are excluded. Soft-failed command jobs,
canceled/expired jobs, configuration-excluded (`broken`) jobs, and jobs stopped
by failed dependencies are not reported as command failures. Running jobs that
have promised failure are not terminal failures and are also excluded.

JSON/YAML output contains `root_build`, `complete`, `failures`, and `issues`.
Each failure includes its qualified `build` (`org/pipeline/number`), job `id`,
`name`, `state`, `web_url`, `log_url`, `raw_log_url`, and `trigger_path`. The path
lists ancestor trigger jobs as `org/pipeline/number#job-id`, from the root;
root-build failures have an empty path. `--text` provides the same information
in a readable form.

This is one CLI invocation, not one API request: the CLI fetches each reachable
build once using the existing REST API. It does not download logs, artifacts,
or annotations. Build access requires `read_builds` and permission to read each
pipeline; following log pointers separately requires `read_build_logs`.

Inaccessible children, invalid child references, missing/not-yet-created child
builds (except skipped or configuration-excluded triggers), and request failures
produce partial output with `complete: false`, explanatory `issues`, and a
nonzero exit status. Failures from accessible siblings are still returned.
`complete` describes traversal of the observed current attempts, not an atomic
snapshot or a guarantee that running builds have finished. Builds created by
scripts/API calls without a trigger-job link cannot be discovered this way.
`--recursive` cannot be combined with `--summary`, `--web`, or `--job-states`.

### Migrate GitHub Actions secrets

Use `bk secret migrate github-actions` to move repository Actions secrets into
Buildkite without exposing their values to the local machine. See the
[GitHub Actions secrets migration guide](docs/secret-migration-github-actions.md).

## Feedback

We'd love to hear any feedback and questions you might have. Please [file an issue on GitHub](https://github.com/buildkite/cli/issues) and let us know!

## Development

This repository uses [mise](https://mise.jdx.dev/) to pin Go and the main
local development tools.

```bash
git clone git@github.com:buildkite/cli.git
cd cli/
mise install
mise run build
mise run install
mise run install:global
mise run hooks
mise run format
mise run lint
mise run test
mise run generate
go run main.go --help
```

`mise.toml` pins the shared toolchain, including the release helpers used in
CI. The module itself remains compatible with Go `1.26.10` as declared in
`go.mod`.

### Certificate trust

Build with Go `1.27.3` or later (or `1.26.10` on the Go 1.26 branch) to include
the [CVE-2026-111943 mitigation](https://go.dev/issue/82345), which rejects
certificate chains containing eight distrusted intermediate public keys.
This does not remove emSign or AAA Certificate Services roots from trust stores.
The CLI uses the operating system's certificate trust store, not an embedded CA
bundle. Broader root distrust must be applied to that store separately.

Published container images use `cgr.dev/chainguard/static:latest` via GoReleaser;
the separate Dockerfile installs Alpine's `ca-certificates` package. Rebuild and
republish images to pick up updated CA bundles, and update the host trust store
for standalone binaries. Rebuild and release binaries to pick up Go security
fixes; updating the installed Go toolchain alone does not update an existing `bk`.
