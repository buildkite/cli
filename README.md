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
CI. The module itself remains compatible with Go `1.26.0` as declared in
`go.mod`.
