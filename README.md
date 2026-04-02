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

### Authenticate

```sh
bk auth login
```

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

- `mise run build` builds `bk` into `dist/bk`, stamped with `git describe`
- `mise run install` installs `bk` into `$(go env GOBIN)` or `$(go env GOPATH)/bin`
- `mise run install:global` installs `bk` into `~/bin/bk`

`mise.toml` pins Go `1.26.1` to match the current release build image. The
module itself remains compatible with Go `1.25.0` as declared in `go.mod`.
