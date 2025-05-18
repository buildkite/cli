# bk - The Buildkite CLI

[![Latest Release](https://img.shields.io/github/v/release/buildkite/cli?include_prereleases&sort=semver&display_name=release&logo=buildkite)](https://github.com/buildkite/cli/releases)

A command line interface for [Buildkite](https://buildkite.com/).

Use `bk` to interact with your Buildkite organization without leaving the terminal 🙌.

![bk cli](./images/demo.gif)

## Installing

### Using the binary

`bk` is available as a downloadable binary from the [releases page](https://github.com/buildkite/cli/releases).

### Using Brew

```sh
brew tap buildkite/buildkite && brew install buildkite/buildkite/bk
```

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
  user        Invite users to the organization

Flags:
  -h, --help   help for bk

Use "bk [command] --help" for more information about a command.
```

### Configure

You'll need to run `bk configure` first to set up your organization and API token.

### Shell Prompt Integration

Want to display your current Buildkite organization in your shell prompt? Check out our [Shell Prompt Integration Guide](/docs/shell-prompt-integration.md) for detailed instructions for Zsh, Bash, and Powerlevel10k.

## 💬 Feedback

We'd love to hear any feedback and questions you might have. Please [file an issue on GitHub](https://github.com/buildkite/cli/issues) and let us know!

## LLM Documentation

We provide a comprehensive [LLM-optimized documentation file](https://github.com/buildkite/cli/releases/latest/download/llms.txt) for AI language models to better understand the CLI.

## 🔨 Development

Developed using Golang 1.20+ with modules.

```bash
git clone git@github.com:buildkite/cli.git
cd cli/
export BUILDKITE_GRAPHQL_TOKEN="<token>"
go generate
go run cmd/bk/main.go --help
```
