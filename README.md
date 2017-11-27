# Buildkite Command-line Interface

A cli for interacting with Buildkite.com to make it easier to create and manage
pipelines and builds. Integrates seamlessly with Github / Gitlab / Bitbucket and provides a

## This is still (mostly) imaginary.🤔🦄🦑

## Usage

```bash
## set up your credentials
bk configure

# creates a .buildkite/pipeline.yml with queue=default and no-op step
bk init

# creates a bk pipeline for the current project, sets up webhooks in github/bitbucket
# this is inferred from your current directory git remote information
bk pipeline create .

# trigger a build via the cli
bk build .
```

## Design

### Secret Storage

`bk`` needs several sets of credentials to operate (aws, buildkite, and github/gitlab/bithucket), all of which need to be stored securely on your local machine. We use 99design's [keyring](https://github.com/99designs/keyring) to store the credentials in your operating system's native secure store. On macOS this is Keychain.

