# bk - The Buildkite CLI

[![Latest Release](https://img.shields.io/github/v/release/buildkite/cli?include_prereleases&sort=semver&display_name=release&logo=buildkite)](https://github.com/buildkite/cli/releases)

A command line interface for [Buildkite](https://buildkite.com/).

## Install

On macOS, install with [Homebrew](https://brew.sh/):

```sh
brew install buildkite
```

> [!TIP]
> The Homebrew core formula installs the command as `buildkite`. To use `bk` with
> the examples below, run `alias bk=buildkite`. Add that line to your shell
> configuration (`~/.zshrc` for Zsh or `~/.bashrc` for Bash) to keep it across sessions.

Alternatively, install from Buildkite's tap, which provides `bk` directly:

```sh
brew install buildkite/buildkite/bk@3
```

For Linux, Windows, and other installation methods, see the
[installation guide](https://buildkite.com/docs/platform/cli/installation), or
download a binary from the [releases page](https://github.com/buildkite/cli/releases).

## Authenticate

Sign in through your browser:

```sh
bk auth login
```

Stored credential refresh, login replacement, and logout coordinate across `bk`
processes using private locks in the OS account's persistent home directory,
independent of `HOME` or repository configuration. On Linux, these operations
require a local `/etc/passwd` entry and a writable home outside temporary
directories. Environments without one can use `BUILDKITE_API_TOKEN` instead.

## Documentation

See the [official documentation](https://buildkite.com/docs/platform/cli) for
configuration, usage guides, and the command reference.
