# bk - The Buildkite CLI

A command line interface for [Buildkite](https://buildkite.com/).

## Status

This is experimental! 🦄🦑

For any questions, issues of feedback please [file an issue](https://github.com/buildkite/cli/issues) 💖

## Usage

```bash
# Sets up your credentials
bk configure

# Creates a .buildkite/pipeline.yml with queue=default and no-op step
# Also creates a bk pipeline for the current project, and sets up webhooks in GitHub/Bitbucket
bk init

# Triggers a build via the cli
bk create build

# Opens the current pipeline in your browse
bk browse

# Lists pipelines that you have access to
bk list pipelines

# Runs a build entirely locally for development
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
