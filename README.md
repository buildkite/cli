# Buildkite Command-line Interface

A cli for interacting with Buildkite.com to make it easier to create and manage pipelines and builds.

## Status

This is experimental! 🦄🦑 At this stage support for the `bk` cli is via [issues on this repository](https://github.com/buildkite/cli/issues), but we'd love feedback!

## Usage

```bash
## set up your credentials
bk configure

# creates a .buildkite/pipeline.yml with queue=default and no-op step
# also creates a bk pipeline for the current project, sets up webhooks in github/bitbucket
bk init

# trigger a build via the cli
bk create build

# list pipelines that you have access to
bk list pipelines

# run a build entirely locally for development
bk run local
```

## Development

Developed using Golang 1.11+ with modules.

```
export GO111MODULE=on
git clone git@github.com:buildkite/cli.git
cd cli/
go run ./cmd/bk --help
```

## Design

### Secret Storage

`bk` needs several sets of credentials to operate (aws, buildkite, and github/gitlab/bithucket), all of which need to be stored securely on your local machine. We use 99design's [keyring](https://github.com/99designs/keyring) to store the credentials in your operating system's native secure store. On macOS this is Keychain.

