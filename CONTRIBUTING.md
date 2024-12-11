# Contributing

We welcome contributions from the community to make Buildkite CLI, `bk`, project even better.

## Getting Started

To get started with contributing, please follow these steps:

1. Fork the repository 
2. Create a feature branch with a nice name (`git checkout -b cli-new-feature`) for your changes
3. Write your code
    * We use `golangci-lint` and would be good to use the same in order to pass a PR merge. You can use `docker-compose -f .buildkite/docker-compose.yaml run lint` for that. 
    * Make sure the tests are passing by running go test ./...
5. Commit your changes and push them to your forked repository.
7. Submit a pull request with a detailed description of your changes and linked to any relevant issues.

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
