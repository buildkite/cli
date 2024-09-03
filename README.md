# bk - The Buildkite CLI

[![Latest Release](https://img.shields.io/github/v/release/buildkite/cli?include_prereleases&sort=semver&display_name=release&logo=buildkite)](https://github.com/buildkite/cli/releases)

A command line interface for [Buildkite](https://buildkite.com/).

Use `bk` to interact with your Buildkite organization without leaving the terminal 🙌.

> [!NOTE]  
> The `3.x` (default) branch is under current active development. If you'd like to use the most recent released version of the Buildkite CLI, please refer to the `main` [branch](https://github.com/buildkite/cli/tree/main) and [releases](https://github.com/buildkite/cli/releases) page for details and installation instructions.

## Installing

`bk` is available as a downloadable binary from the [releases page](https://github.com/buildkite/cli/releases).

## Usage

```
Work with Buildkite from the command line.

Usage:
  bk [command]

Examples:
$ bk build view


Available Commands:
  agent       Manage agents
  api         Interact with the Buildkite API
  build       Manage pipeline builds
  cluster     Manage organization clusters
  completion  Generate the autocompletion script for the specified shell
  configure   Configure Buildkite API token
  help        Help about any command
  init        Initialize a pipeline.yaml file
  job         Manage jobs within a build
  package     Manage packages
  pipeline    Manage pipelines
  use         Select an organization

Flags:
  -h, --help   help for bk

Use "bk [command] --help" for more information about a command.
```

### Configure

You'll need to run `bk configure` first to set up your organization and API token.

## 💬 Feedback

We'd love to hear any feedback and questions you might have. Please [file an issue on GitHub](https://github.com/buildkite/cli/issues) and let us know!

## 🔨 Development

Developed using Golang 1.20+ with modules.

```bash
git clone git@github.com:buildkite/cli.git
cd cli/
export BUILDKITE_GRAPHQL_TOKEN="<token>"
go generate
go run cmd/bk/main.go --help
```
