Buildkite Command-line Interface
================================

A cli for interacting with Buildkite.com to make it easier to create and manage
pipelines and builds. Integrates seamlessly with AWS / Github / Gitlab / Bitbucket.

Status
------

**This is still imaginary.🤔🦄🦑**

Examples
--------

```bash
# creates a .buildkite/pipeline.yml with queue=default and no-op step
bk project init --stack=buildkite .

# creates a bk pipeline for the current project, sets up webhooks in github/bitbucket
# this is inferred from your current directory git remote information
bk pipeline create .

# creates an elastic stack called "buildkite". Will required aws credentials
bk aws stack create

# uploads a deploy key to the secrets bucket and adds it to the github/bitbucket project as a deploy key. Will
# require both aws and github credentials
bk aws stack link .

# trigger a build via the cli
bk build .
```

Commands
--------

* [ ] `bk aws stack create`
* [ ] `bk aws stack link`
* [ ] `bk aws stack inspect`
* [ ] `bk pipeline create`
* [ ] `bk pipeline inspect`
* [ ] `bk build`


Supports the following integrations:

* [ ] Creates webhooks on github.com when a pipeline is created
* [ ] Generates deploy keys and uploads them to github.com
* [ ] Generates deploy keys and uploads privkey to aws s3
