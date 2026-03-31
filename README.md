# bk - The Buildkite CLI

[![Latest Release](https://img.shields.io/github/v/release/buildkite/cli?include_prereleases&sort=semver&display_name=release&logo=buildkite)](https://github.com/buildkite/cli/releases)

A command line interface for [Buildkite](https://buildkite.com/).

Full documentation is available at [buildkite.com/docs/platform/cli](https://buildkite.com/docs/platform/cli).

## Quick Start

### Install

```sh
brew tap buildkite/buildkite && brew install buildkite/buildkite/bk
```

Or install a specific release from the [releases page](https://github.com/buildkite/cli/releases):

```sh
VERSION=3.33.0
ASSET="bk_${VERSION}_macOS_arm64.zip"

curl -L -o "/tmp/${ASSET}" \
  "https://github.com/buildkite/cli/releases/download/v${VERSION}/${ASSET}"
curl -L -o "/tmp/bk_${VERSION}_checksums.txt" \
  "https://github.com/buildkite/cli/releases/download/v${VERSION}/bk_${VERSION}_checksums.txt"

grep "${ASSET}" "/tmp/bk_${VERSION}_checksums.txt" | shasum -a 256 -c
unzip -oq "/tmp/${ASSET}" -d /tmp
install -m 0755 "/tmp/bk_${VERSION}_macOS_arm64/bk" "$HOME/bin/bk"

bk version
```

If you're not on Apple Silicon, replace `bk_${VERSION}_macOS_arm64.zip` with the asset for your platform, such as `bk_${VERSION}_macOS_amd64.zip`, `bk_${VERSION}_linux_arm64.tar.gz`, or `bk_${VERSION}_linux_amd64.tar.gz`.

To opt into `preflight` after installing `v3.33.0`:

```sh
bk config set experiments preflight
bk preflight --pipeline buildkite/buildkite-cli
```

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
mise run hooks
mise run format
mise run lint
mise run test
mise run generate
go run main.go --help
```

`mise.toml` pins Go `1.26.1` to match the current release build image. The
module itself remains compatible with Go `1.25.0` as declared in `go.mod`.
