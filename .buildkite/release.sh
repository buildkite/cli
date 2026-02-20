#!/usr/bin/env bash

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

# check if DOCKERHUB_USER and DOCKERHUB_PASSWORD are set if not skip docker login
if [[ -z "${DOCKERHUB_USER:-}" || -z "${DOCKERHUB_PASSWORD:-}" ]]; then
    echo "Skipping Docker login as DOCKERHUB_USER or DOCKERHUB_PASSWORD is not set"
else
    echo "--- :key: :docker: Login to Docker Hub using ko"
    echo "${DOCKERHUB_PASSWORD}" | ko login index.docker.io --username "${DOCKERHUB_USER}" --password-stdin
    if [[ $? -ne 0 ]]; then
        echo "Docker login failed"
        exit 1
    fi
fi

if ! goreleaser "$@"; then
    echo "Failed to build a release"
    exit 1
fi
