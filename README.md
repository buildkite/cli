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
To target an organization without changing your selection, use a qualified pipeline
or URL with `build list` or `job list`:

```sh
bk build list --pipeline other-org/my-pipeline
bk job list --pipeline https://buildkite.com/other-org/my-pipeline --build 123
```

The CLI does not fall back to another organization's stored credentials. For
automation or a token with access to multiple organizations, supply
`BUILDKITE_API_TOKEN` explicitly. This environment token takes precedence over
stored credentials and is not replaced by stored OAuth credentials if a request
fails authentication.

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
