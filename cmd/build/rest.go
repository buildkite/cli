package build

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/buildkite/cli/v3/internal/build/view"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type buildDetails struct {
	buildkite.Build
	Jobs []buildJob `json:"jobs,omitempty"`
}

type buildJob struct {
	buildkite.Job
	BrokenReason string `json:"broken_reason,omitempty"`
}

func getBuildDetails(ctx context.Context, client *buildkite.Client, opts view.ViewOptions, jobStates []string) (buildDetails, error) {
	query := url.Values{}
	for _, state := range jobStates {
		query.Add("job_states[]", state)
	}
	u := fmt.Sprintf("v2/organizations/%s/pipelines/%s/builds/%d",
		url.PathEscape(opts.Organization), url.PathEscape(opts.Pipeline), opts.BuildNumber)
	if encoded := query.Encode(); encoded != "" {
		u += "?" + encoded
	}
	req, err := client.NewRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return buildDetails{}, err
	}
	var build buildDetails
	_, err = client.Do(req, &build)
	return build, err
}
