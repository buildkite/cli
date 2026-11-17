#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "Error: $*" >&2
  exit 1
}

version=$(cat VERSION)
[[ "$version" =~ ^3\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || fail "VERSION must be a bare v3 major.minor.patch version"
tag="v${version}"
commit=$(git rev-parse HEAD)
export GH_TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-${MISE_GITHUB_TOKEN:-}}}"

remote_tags() {
  # A network error must not be mistaken for a missing tag.
  tags=$(git ls-remote --tags origin 'refs/tags/v3.*')
  latest=$(printf '%s\n' "$tags" | awk '$2 ~ /^refs\/tags\/v3\.[0-9]+\.[0-9]+$/ {sub("refs/tags/v", "", $2); print $2}' | sort -t. -k1,1n -k2,2n -k3,3n | tail -1)
  [[ -n "$latest" ]] || fail "No released v3 version found"
}

require_new_version() {
  local previous="${1:-$latest}"
  [[ "$version" != "$previous" && $(printf '%s\n' "$version" "$previous" | sort -t. -k1,1n -k2,2n -k3,3n | tail -1) == "$version" ]] || fail "VERSION must be newer than ${previous}"
}

release_notes() {
  local endpoint="$1"
  gh api "$endpoint" --jq "
    if type == \"array\" then
      map(select(.merge_commit_sha == \"$commit\" and .merged_at != null))
    else [.] end |
    map(select(.base.ref == \"main\" and .base.repo.full_name == \"buildkite/cli\"
      and .title == \"release: $tag\"
      and any(.labels[]; .name == \"release\")
      and ((.body // \"\") | test(\"\\\\S\")))) |
    if length == 1 then .[0].body
    else error(\"Expected one release PR to main titled release: $tag, labelled release, with nonempty notes\") end"
}

case "${1:-}" in
  check)
    if goreleaser check; then
      :
    else
      status=$?
      # GoReleaser returns 2 for valid configs with deprecated properties.
      # Keep the existing Homebrew formula channel (brews), not a cask.
      [[ "$status" == 2 ]] || exit "$status"
      echo "GoReleaser config is valid with deprecations (the Homebrew formula uses brews)."
    fi
    if [[ "${BUILDKITE_PULL_REQUEST:-false}" != false ]]; then
      git fetch origin main
      base=$(git merge-base HEAD FETCH_HEAD)
      remote_tags
      previous=$(git show "${base}:VERSION" 2>/dev/null || printf '%s' "$latest")
      if [[ "$version" != "$previous" ]]; then
        require_new_version
        require_new_version "$previous"
        release_notes "repos/buildkite/cli/pulls/${BUILDKITE_PULL_REQUEST}" >/dev/null
      fi
    fi
    ;;
  tag)
    [[ "${BUILDKITE_PIPELINE_SLUG:-}" == buildkite-cli && "${BUILDKITE_BRANCH:-}" == main && "${BUILDKITE_PULL_REQUEST:-false}" == false && -z "${BUILDKITE_TAG:-}" ]] || fail "Only non-PR main builds in buildkite-cli can tag releases"
    [[ "${BUILDKITE_COMMIT:-}" == "$commit" ]] || fail "Checkout does not match the build commit"
    for step in lint test vulncheck build release_check; do
      [[ $(buildkite-agent step get state --step "$step") == passed ]] || fail "Prerequisite $step has not passed"
    done
    remote_tags
    if printf '%s\n' "$tags" | awk '{print $2}' | grep -Fxq "refs/tags/${tag}"; then
      echo "${tag} already exists; not publishing again. Retry failed jobs in its original release build to recover."
      exit 0
    fi
    require_new_version
    release_notes "repos/buildkite/cli/commits/${commit}/pulls" >/dev/null
    gh auth setup-git
    git push origin "${commit}:refs/tags/${tag}"
    ;;
  notes)
    [[ "${BUILDKITE_PIPELINE_SLUG:-}" == buildkite-cli-release && "${BUILDKITE_PULL_REQUEST:-false}" == false && "${BUILDKITE_TAG:-}" == "$tag" ]] || fail "Only version-tag builds in buildkite-cli-release can publish"
    [[ "${BUILDKITE_COMMIT:-}" == "$commit" && $(git rev-parse "${tag}^{commit}") == "$commit" ]] || fail "Tag does not match the build commit"
    release_notes "repos/buildkite/cli/commits/${commit}/pulls" > "${2:?Pass a release notes file}"
    ;;
  *)
    fail "Usage: $0 check | tag | notes <file>"
    ;;
esac
