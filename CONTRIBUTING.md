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

## Releasing

Builds on `main` include a block step to "Create a release". The step takes a tag name, then takes care of tagging the built commit.

New tags trigger the release pipeline:

https://buildkite.com/buildkite/buildkite-cli-release

This will prepare a new draft release on GitHub:

https://github.com/buildkite/cli/releases

To release, edit the draft and _Publish release_.

## Reporting Issues

If you encounter any issues or have suggestions for improvements, please open an issue on the GitHub repository. Provide as much detail as possible, including steps to reproduce the issue.

## Contact

If we're really dragging our feet on reviewing a PR, please feel free to ping us through GitHub or Slack, or get in touch with [support@buildkite.com](mailto:support@buildkite.com), and they can bug us to get things done :) 

Happy contributing!
