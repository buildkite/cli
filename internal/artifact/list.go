package artifact

import (
	"context"
	"strings"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

// List fetches all artifacts for a build, or for a specific job when jobUUID
// is non-empty, paginating through all results.
//
// path and state are optional server-side filters. state is lower-cased
// before being sent so callers can pass user input verbatim.
func List(ctx context.Context, client *buildkite.Client, org, pipeline, build, jobUUID, path, state string) ([]buildkite.Artifact, error) {
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
