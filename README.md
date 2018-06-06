# Buildkite Command-line Interface

A cli for interacting with Buildkite.com to make it easier to create and manage
pipelines and builds. Integrates seamlessly with Github / Gitlab / Bitbucket.

## Status

This is still (mostly) imaginary.🤔🦄🦑

 * [x] Configure command
   * [x] Store Buildkite GraphQL token
   * [x] OAuth with github.com and store the access token
 * [x] Init command
   * [x] Creates Buildkite pipeline if missing
   * [x] Adds Buildkite webhook to Github repository settings
   * [x] Adds a generic .buildkite/pipeline.yml to the local repository
 * [x] Build command
   * [x] Create a build on Buildkite, shows a link

## Usage

```bash
## set up your credentials
bk configure

# creates a .buildkite/pipeline.yml with queue=default and no-op step
# also creates a bk pipeline for the current project, sets up webhooks in github/bitbucket
bk init

# trigger a build via the cli
bk build
```

## Design

### Secret Storage

`bk` needs several sets of credentials to operate (aws, buildkite, and github/gitlab/bithucket), all of which need to be stored securely on your local machine. We use 99design's [keyring](https://github.com/99designs/keyring) to store the credentials in your operating system's native secure store. On macOS this is Keychain.

