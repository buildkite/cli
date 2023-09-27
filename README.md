# bk - The Buildkite CLI

[![Latest release](https://img.shields.io/github/release/buildkite/cli.svg)](https://github.com/buildkite/cli/releases/latest)

> [!NOTE]  
> The `3.x` (default) branch is under current active development. If you'd like to use the most recent released version of the Buildkite CLI, please refer to the `main` [branch](https://github.com/buildkite/cli/tree/main) and [releases](https://github.com/buildkite/cli/releases) page for details and installation instructions.

A command line interface for [Buildkite](https://buildkite.com/).

## 💬 Feedback

We'd love to hear any feedback and questions you might have. Please [file an issue on GitHub](https://github.com/buildkite/cli/issues) and let us know!

## 🔨 Development

Developed using Golang 1.20+ with modules.

```bash
export GO111MODULE=on
git clone git@github.com:buildkite/cli.git
cd cli/
go run cmd/bk/main.go --help
```
