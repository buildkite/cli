#!/usr/bin/env bash

#
# This script is used to build a release of the CLI and publish it to multiple registries on Buildkite
#

set -euo pipefail

args=("$@")
if [[ " $* " != *" --snapshot "* ]]; then
    notes=$(mktemp)
    trap 'rm -f "$notes"' EXIT
    .buildkite/release-pr.sh notes "$notes"
    args+=(--release-notes "$notes")
fi

# check if DOCKERHUB_USER and DOCKERHUB_PASSWORD are set if not skip docker login
if [[ -z "${DOCKERHUB_USER:-}" || -z "${DOCKERHUB_PASSWORD:-}" ]]; then
    echo "Skipping Docker login as DOCKERHUB_USER or DOCKERHUB_PASSWORD is not set"
else
    echo "--- :key: :docker: Login to Docker Hub using ko"
    if ! echo "${DOCKERHUB_PASSWORD}" | ko login index.docker.io --username "${DOCKERHUB_USER}" --password-stdin; then
        echo "Docker login failed"
        exit 1
    fi
fi

if ! goreleaser "${args[@]}"; then
    echo "Failed to build a release"
    exit 1
fi
