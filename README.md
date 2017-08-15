Buildkite Command-line Interface
================================

A cli for interacting with Buildkite.com to make it easier to create and manage
pipelines and builds. Integrates seamlessly with AWS / Github / Gitlab / Bitbucket.

Status
------

This is still imaginary. 

Examples
--------

```bash
# creates an elastic stack called "buildkite"
buildkite linux-aws-stack create

# creates a .buildkite/pipeline.yml with queue=default and no-op step
buildkite project init --stack=buildkite .

# creates a bk pipeline for the current project, sets up webhooks in github/bitbucket
buildkite pipeline create  . 

# uploads a deploy key to the secrets bucket and adds it to the github/bitbucket project as a deploy key
buildkite linux-aws-stack link . 

# trigger a build
git commit -a -m "Added a basic buildkite pipeline"
git push
```

Commands
--------

* [ ] `buildkite linux-aws-stack create`
* [ ] `buildkite linux-aws-stack link` 
* [ ] `buildkite pipeline create`
* [ ] `buildkite project init`
* [ ] `buildkite project build`

Supports the following integrations:

* [ ] Creates webhooks on github.com when a pipeline is created
* [ ] Generates deploy keys and uploads them to github.com
* [ ] Generates deploy keys and uploads privkey to aws s3
