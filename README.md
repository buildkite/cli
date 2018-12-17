# bk - The Buildkite CLI

A command line interface for [Buildkite](https://buildkite.com/).

## Status

This is experimental! 🦄🦑 For any questions, issues of feedback please [file an issue](https://github.com/buildkite/cli/issues) 💖

## Installation

On macOS you can install with [Homebrew](https://brew.sh):

```
brew install buildkite/cli/bk
````

On all other platforms, download a binary from the [latest GitHub releases](https://github.com/buildkite/cli/releases/latest).

## Usage

```bash
# Sets up your credentials
bk configure

# Opens the current pipeline in your browser
bk browse

# List the pipelines that you have access to
bk pipelines list

# Triggers a build for the current directory's commit and branch
bk build create

# Runs the current directory's pipeline steps locally (requires the buildkite-agent to be installed)
bk local run

# Sets up your current git project directory for Buildkite, creating a .buildkite/pipeline.yml file, a pipeline in Buildkite, and setting up the webhooks on GitHub or Bitbucket
bk init
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
