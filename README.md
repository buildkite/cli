# bk - The Buildkite CLI

[![Latest release](https://img.shields.io/github/release/buildkite/cli.svg)](https://github.com/buildkite/cli/releases/latest)

A command line interface for [Buildkite](https://buildkite.com/).

## 💬 Pre-Release Feedback

This is currently a pre-release, and we'd love to hear any feedback and questions you might have. Please [file an issue on GitHub](https://github.com/buildkite/cli/issues) and let us know 💖

## ⬇️ Installation

On macOS, you can install with [Homebrew](https://brew.sh):

```
brew tap buildkite/cli
brew install bk
````

On all other platforms, download a binary from the [latest GitHub releases](https://github.com/buildkite/cli/releases/latest).

## 📄 Usage

```bash
# Sets up your credentials (stored in your operating system's native secure storage, using https://github.com/99designs/keyring)
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

## 🔨 Development

Developed using Golang 1.11+ with modules.

```
export GO111MODULE=on
git clone git@github.com:buildkite/cli.git
cd cli/
go run ./cmd/bk --help
```
