package artifact

import (
	"context"
	"fmt"
	"slices"
	"strings"

	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

// AllowedStates is the closed set of artifact states the Buildkite API
// accepts as the state filter. Exposed so callers can reference the same
// list in help text and up-front validation.
//
// See https://buildkite.com/docs/apis/rest-api/artifacts#list-artifacts-for-a-build
var AllowedStates = []string{"new", "finished", "error", "deleted", "expired"}

// ValidateState rejects state values the Buildkite API won't accept, so a
// typo like "finshed" surfaces as a validation error instead of silently
// returning zero artifacts.
//
// An empty string is treated as "no filter" and always accepted. Comparison
// is case-insensitive so callers can pass user input verbatim.
func ValidateState(state string) error {
	if state == "" {
		return nil
	}
	if slices.Contains(AllowedStates, strings.ToLower(state)) {
		return nil
	}
	return bkErrors.NewValidationError(
		nil,
		fmt.Sprintf("invalid artifact state %q", state),
		fmt.Sprintf("state must be one of: %s", strings.Join(AllowedStates, ", ")),
	)
}

// List fetches all artifacts for a build, or for a specific job when jobUUID
// is non-empty, paginating through all results.
//
// path and state are optional server-side filters. state is validated via
// ValidateState and lower-cased before being sent, so callers can pass user
// input verbatim.
func List(ctx context.Context, client *buildkite.Client, org, pipeline, build, jobUUID, path, state string) ([]buildkite.Artifact, error) {
	if err := ValidateState(state); err != nil {
		return nil, err
	}

	var all []buildkite.Artifact
	opts := &buildkite.ArtifactListOptions{
		Path:        path,
		State:       strings.ToLower(state),
		ListOptions: buildkite.ListOptions{PerPage: 100},
	}

	for {
		var artifacts []buildkite.Artifact
		var resp *buildkite.Response
		var err error

		// ListByJob and ListByBuild both take *ArtifactListOptions, which
		// carries Path and State — so the same filters flow into either
		// endpoint when jobUUID is combined with path / state.
		if jobUUID != "" {
			artifacts, resp, err = client.Artifacts.ListByJob(ctx, org, pipeline, build, jobUUID, opts)
		} else {
			artifacts, resp, err = client.Artifacts.ListByBuild(ctx, org, pipeline, build, opts)
		}
		if err != nil {
			return nil, err
		}

		all = append(all, artifacts...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return all, nil
}
