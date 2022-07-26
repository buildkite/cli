#!/usr/bin/env bash
set -euo pipefail
bash -c "$(curl -sL https://raw.githubusercontent.com/buildkite/agent/master/install.sh)"
export PATH="/root/.buildkite-agent/bin:$PATH"
go run ./cmd/bk run .buildkite/local.yml
