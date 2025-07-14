# Buildkite CLI v3.0 Upgrade Guide

## Breaking Changes

1. **Shell completions:** If you use tab completion, run `bk install-completions` once to re-enable it
2. **Exit codes:** Scripts expecting exit code `1` for flag errors should expect `2`

## Deprecations (still work, but update when convenient)

- `--envFile` → `--env-file`
- `BK_ACCESS_TOKEN` → `BUILDKITE_API_TOKEN`
- `--json` → `--output json` (applies to build new, pipeline view, and other commands)
