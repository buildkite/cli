# Migrate GitHub Actions secrets

GitHub's API lists repository secret names but cannot export their values.
`bk secret migrate github-actions` prepares and dispatches a reviewable,
one-use workflow that sends selected values directly from GitHub Actions to
Buildkite. Secret values do not pass through `bk`, command arguments, local
files, workflow outputs, or artifacts.

## Prerequisites

Authenticate `gh` for the source GitHub repository and `bk` for the destination
Buildkite organization:

```sh
gh auth login
bk auth login
```

The Buildkite API token needs `read_pipelines`, `read_secrets_details`, and
`write_secrets`. Its user also needs `manage_cluster` permission for the
destination cluster. Only github.com repository Actions secrets are supported;
GitHub Enterprise Server, organization, environment, Dependabot, and Codespaces
secrets are not.

## Prepare the workflow

From the source GitHub repository, select secrets and write the workflow:

```sh
bk secret migrate github-actions prepare \
  --match 'DEPLOY_*' \
  --secret API_KEY \
  --output .github/workflows/migrate-buildkite-secrets.yml
```

Without `--secret` or `--match`, `prepare` lists the repository's Actions
secret names and prompts for a selection. It discovers Buildkite pipelines
associated with the repository and prompts if more than one matches. For a
non-interactive run, use `--no-input`, provide `--secret` or `--match`, and
provide `--pipeline` when repository discovery is ambiguous:

```sh
bk --no-input secret migrate github-actions prepare \
  --organization acme \
  --pipeline deploy \
  --cluster 11111111-2222-4333-8444-555555555555 \
  --match 'DEPLOY_*' \
  --output .github/workflows/migrate-buildkite-secrets.yml
```

`--cluster` asserts the selected pipeline's cluster; it cannot redirect the
migration to a different cluster. The default access policy is restricted to
the selected pipeline. Use `--policy-file` for a different non-empty
[Buildkite secret access policy](https://buildkite.com/docs/pipelines/security/secrets/buildkite-secrets/access-policies).

Each workflow can migrate at most 40 secrets. `GITHUB_TOKEN` cannot be migrated.
Destination keys are create-only: preparation fails if any selected name already
exists, and the migration backend never overwrites an existing value.

Review the generated static secret allowlist and access policy, then commit and
merge the unchanged file into the repository's default branch. `bk` never
commits or pushes it and refuses to replace an existing workflow file.

## Dispatch the migration

Run the committed workflow:

```sh
bk secret migrate github-actions run \
  --workflow .github/workflows/migrate-buildkite-secrets.yml
```

Before creating authority, `run`:

1. Re-renders the workflow and requires an exact deterministic match.
2. Resolves the current default-branch commit.
3. Reads the committed workflow at that exact commit and requires it to match
   the local file. Windows CRLF checkout conversion is accepted.
4. Creates a short-lived, one-use Buildkite grant bound to the organization,
   cluster, policy, static secret names, immutable GitHub repository IDs,
   workflow path and commit, default-branch ref, and `workflow_dispatch` event.
5. Dispatches that exact workflow and branch through `gh`.

The command prints the exact Actions run URL when `gh` returns one, otherwise
the Actions workflow URL, plus the grant ID. A successful command
means GitHub accepted the dispatch, **not** that migration completed. Open the
printed Actions URL and verify the run succeeds. If dispatch fails, the grant
expires unused; rerun the command to create a replacement. There is no
`--watch` option because an immutable run ID is not returned by every supported
`gh`/GitHub dispatch path, so polling could attach to an unrelated concurrent
run.

After the run succeeds, remove the migration workflow from the default branch.

## Security properties

The generated workflow contains one literal `${{ secrets.NAME }}` reference
for each reviewed name. It validates every value before requesting GitHub OIDC,
uses the grant's migration URL as the exact OIDC audience, disables redirects,
and sends one in-memory HTTPS batch. Values must be nonblank and smaller than
32 KiB. The backend validates the exact batch and atomically creates all visible
secret records while consuming the grant.

Anyone who can modify and run the workflow on the default branch can read its
selected GitHub secrets. Commit binding limits the additional Buildkite
authority to the reviewed workflow revision; removing the workflow after the
migration removes that GitHub-side path.

## Workflows prepared by `buildkite-gha`

`buildkite-gha` no longer includes `migrate-secrets`; this CLI is the only
migration tool. Workflows that `buildkite-gha migrate-secrets` v0.90.0 or later
prepared are byte-compatible, so run them with
`bk secret migrate github-actions run` without regenerating or recommitting
them.
