#!/usr/bin/env bash
set -euo pipefail
go mod tidy

if ! git diff --quiet; then
  echo "The Go module dependency setup is not clean. Please run 'go mod tidy' and commit any resulting changes."
  exit 1
fi
