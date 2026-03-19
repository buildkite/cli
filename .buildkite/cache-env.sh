#!/usr/bin/env bash

set -euo pipefail

checkout_path="${BUILDKITE_BUILD_CHECKOUT_PATH:-$(pwd)}"
cache_root="${checkout_path}/.buildkite/cache-volume"

mkdir -p \
  "${cache_root}/go/build" \
  "${cache_root}/go/pkg/mod" \
  "${cache_root}/golangci-lint"

export GOCACHE="${cache_root}/go/build"
export GOMODCACHE="${cache_root}/go/pkg/mod"
export GOLANGCI_LINT_CACHE="${cache_root}/golangci-lint"
