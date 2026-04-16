package preflight

import (
	"context"
	"fmt"
	"strings"

	buildstate "github.com/buildkite/cli/v3/internal/build/state"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// BranchBuild represents a preflight branch and its associated build status.
type BranchBuild struct {
	Branch string
	Ref    string
	Build  *buildkite.Build
}

// IsCompleted returns true if the associated build has reached a terminal state
// (passed, failed, canceled, etc.), or if no build was found for the branch.
func (bb BranchBuild) IsCompleted() bool {
	if bb.Build == nil {
		return true
	}
	return buildstate.IsTerminal(buildstate.State(bb.Build.State))
}

// ListRemotePreflightBranches returns all remote branches matching bk/preflight/*.
func ListRemotePreflightBranches(dir string, debug bool) ([]BranchBuild, error) {
	return lsRemotePreflightBranches(dir, "refs/heads/bk/preflight/*", debug)
}

// LookupRemotePreflightBranch returns the remote bk/preflight/<uuid> branch if it
// exists, or nil if no such branch is present on the remote.
func LookupRemotePreflightBranch(dir, uuid string, debug bool) (*BranchBuild, error) {
	branches, err := lsRemotePreflightBranches(dir, "refs/heads/bk/preflight/"+uuid, debug)
	if err != nil {
		return nil, err
	}
	if len(branches) == 0 {
		return nil, nil
	}
	return &branches[0], nil
}

// lsRemotePreflightBranches runs ls-remote against origin with the given ref
// pattern and parses the results into BranchBuild entries.
func lsRemotePreflightBranches(dir, pattern string, debug bool) ([]BranchBuild, error) {
	out, err := gitOutput(dir, nil, debug, "ls-remote", "origin", pattern)
	if err != nil {
		return nil, fmt.Errorf("listing remote preflight branches: %w", err)
	}

	if out == "" {
		return nil, nil
	}

	var results []BranchBuild
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		ref := parts[1]
		branch := strings.TrimPrefix(ref, "refs/heads/")
		results = append(results, BranchBuild{Branch: branch, Ref: ref})
	}

	return results, nil
}

// maxResolveBuildPages is the maximum number of API pages to fetch when
// resolving builds. This prevents runaway pagination when orphaned branches
// have no matching builds.
const maxResolveBuildPages = 10

// ResolveBuilds looks up the most recent build for each preflight branch and
// populates the Build field. Branches with no matching build retain a nil Build.
func ResolveBuilds(ctx context.Context, client *buildkite.Client, org, pipeline string, branches []BranchBuild) error {
	if len(branches) == 0 {
		return nil
	}

	branchNames := make([]string, len(branches))
	for i := range branches {
		branchNames[i] = branches[i].Branch
	}

	resolved := make(map[string]*buildkite.Build, len(branches))
	opts := &buildkite.BuildsListOptions{
		Branch:      branchNames,
		ListOptions: buildkite.ListOptions{PerPage: 100},
	}

	for page := 0; page < maxResolveBuildPages; page++ {
		builds, resp, err := client.Builds.ListByPipeline(ctx, org, pipeline, opts)
		if err != nil {
			return fmt.Errorf("listing builds for preflight branches: %w", err)
		}

		for i := range builds {
			if _, exists := resolved[builds[i].Branch]; !exists {
				resolved[builds[i].Branch] = &builds[i]
			}
		}

		if len(builds) == 0 || len(resolved) >= len(branches) || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	for i := range branches {
		branches[i].Build = resolved[branches[i].Branch]
	}

	return nil
}
