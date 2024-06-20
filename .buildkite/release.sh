#!/bin/env bash

#
# This script is used to build a release of the CLI and publish it to multiple registries on Buildkite
#

# NOTE: do not exit on non-zero returns codes
set -uo pipefail

export GORELEASER_KEY=$(buildkite-agent secret get goreleaser_key)

if [[ $? -ne 0 ]]; then
    echo "Failed to retrieve GoReleaser Pro key"
    exit 1
fi

if ! goreleaser "$@"; then
    echo "Failed to build a release"
    exit 1
fi
