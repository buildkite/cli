# Contributing

We welcome contributions from the community to make Buildkite CLI, `bk`, project even better.

## Getting Started

To get started with contributing, please follow these steps:

1. Fork the repository.
2. Create a feature branch with a nice name (`git checkout -b cli-new-feature`) for your changes.
3. Install [mise](https://mise.jdx.dev/) and run `mise install`.
4. Install the local git hooks with `mise run hooks`.
5. Write your code.
6. Run the local checks before opening a pull request.
   * Format the code with `mise run format`.
   * Lint with `mise run lint`.
   * Make sure the tests pass with `mise run test`.
   * Run `mise run generate` after GraphQL changes. If you need to refresh `schema.graphql`, set `BUILDKITE_GRAPHQL_TOKEN` first.
7. Commit your changes and push them to your forked repository.
8. Submit a pull request with a detailed description of your changes and links to any relevant issues.

The team maintaining this codebase will review your PR and start a CI build for it. For security reasons, we don't automatically run CI against forked repos, and a human will review your PR prior to its CI running.

## Testing

There is a continuous integration pipeline on Buildkite:

https://buildkite.com/buildkite/buildkite-cli

### Certificate trust

Build with Go `1.27.3` or later (or `1.26.10` on the Go 1.26 branch) to include the [CVE-2026-111943 mitigation](https://go.dev/issue/82345), which rejects certificate chains containing eight distrusted intermediate public keys. This does not remove emSign or AAA Certificate Services roots from trust stores. The CLI uses the operating system's certificate trust store, not an embedded CA bundle. Broader root distrust must be applied to that store separately.

Published container images use `cgr.dev/chainguard/static:latest` via GoReleaser; the separate Dockerfile installs Alpine's `ca-certificates` package. Rebuild and republish images to pick up updated CA bundles, and update the host trust store for standalone binaries. Rebuild and release binaries to pick up Go security fixes; updating the installed Go toolchain alone does not update an existing `bk`.

## Releasing

**Merging a reviewed release PR into `main` authorizes publication.** There is no release unblock, workflow dispatch, manual tagging, or draft-publish step.

### Prepare and review a release

1. Start from current `main`. Compare changes since the latest `v3.*` tag.
2. Choose the next version using SemVer: patch for fixes, security updates,
   dependencies, and internal changes; minor for new commands, flags, or other
   compatible capabilities. Use the highest change classification in the release.
   Breaking changes need a major-release plan, including the Go module path and
   Homebrew formula. This flow accepts stable v3 versions only, not prereleases.
3. Change the root `VERSION` file to the bare version, for example `3.70.0`.
   It must be newer than the existing version and released tags. Do not change
   `cmd/version.Version`: GoReleaser still sets the binary version from the tag.
4. Open a PR to `main` titled `release: v3.70.0` with the `release` label.
   Put the complete release notes in the PR body, not in an attached file or
   review comment. Do not include an internal release checklist in that body.
5. Review the notes for user-visible changes, upgrade instructions, security
   fixes, correct PR links, and changes merged since the notes were drafted.
   Update the body before merging. The publisher uses that body, with no
   generated-notes fallback. Leave it unchanged during publication and retries.
6. Wait for CI and approval. Version-bump PRs validate their title, label, notes,
   version ordering, and GoReleaser configuration. PR builds only make snapshots.
   Rerun validation after changing the title, labels, or body, since metadata
   edits do not necessarily start another Buildkite build.

### Follow publication

After merge, the [main pipeline](https://buildkite.com/buildkite/buildkite-cli) runs lint, tests, vulnerability checks, release validation, and a snapshot build. Only after all pass does it tag the exact release PR merge commit. That tag automatically starts the existing [release pipeline](https://buildkite.com/buildkite/buildkite-cli-release). It publishes [GitHub releases](https://github.com/buildkite/cli/releases), the `bk@3` formula in `buildkite/homebrew-buildkite`, and `buildkite/cli` on Docker Hub through GoReleaser. The following matrix uploads Debian and RPM packages to `cli-deb` and `cli-rpm` in Buildkite Package Registries. APK files remain GitHub assets; this does not add an Alpine registry channel.

Check both builds, the published notes, and all distribution channels. A GitHub release alone does not mean the package-upload matrix succeeded. Ordinary merges keep `VERSION` unchanged and skip tagging when its remote tag already exists. The initial `VERSION` is the already released `3.69.0`, so introducing this process does not itself release anything.

## Reporting Issues

If you encounter any issues or have suggestions for improvements, please open an issue on the GitHub repository. Provide as much detail as possible, including steps to reproduce the issue.

## Contact

If we're really dragging our feet on reviewing a PR, please feel free to ping us through GitHub or Slack, or get in touch with [support@buildkite.com](mailto:support@buildkite.com), and they can bug us to get things done :) 

Happy contributing!
