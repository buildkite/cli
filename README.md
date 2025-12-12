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

### Configure

```sh
bk configure
```

## Feedback

We'd love to hear any feedback and questions you might have. Please [file an issue on GitHub](https://github.com/buildkite/cli/issues) and let us know!

## Development

Developed using Go 1.20+ with modules.

```bash
git clone git@github.com:buildkite/cli.git
cd cli/
go generate
go run main.go --help
```
