package job

import (
	"context"
	"fmt"
	"net/url"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

type unblockJobOptions struct {
	Fields map[string]any `json:"fields,omitempty"`
}

func organizationJobPath(organization, jobID, action string) string {
	return fmt.Sprintf(
		"v2/organizations/%s/jobs/%s/%s",
		url.PathEscape(organization),
		url.PathEscape(jobID),
		action,
	)
}

func getJobLog(ctx context.Context, client *buildkite.Client, organization, jobID string) (buildkite.JobLog, error) {
	req, err := client.NewRequest(ctx, "GET", organizationJobPath(organization, jobID, "log"), nil)
	if err != nil {
		return buildkite.JobLog{}, err
	}
	req.Header.Set("Accept", "application/json")

	var jobLog buildkite.JobLog
	if _, err := client.Do(req, &jobLog); err != nil {
		return buildkite.JobLog{}, err
	}

	return jobLog, nil
}

func reprioritizeJob(ctx context.Context, client *buildkite.Client, organization, jobID string, priority int) (buildkite.Job, error) {
	req, err := client.NewRequest(ctx, "PUT", organizationJobPath(organization, jobID, "reprioritize"), &buildkite.JobReprioritizationOptions{
		Priority: priority,
	})
	if err != nil {
		return buildkite.Job{}, err
	}

	var job buildkite.Job
	if _, err := client.Do(req, &job); err != nil {
		return buildkite.Job{}, err
	}

	return job, nil
}

func unblockJob(ctx context.Context, client *buildkite.Client, organization, jobID string, fields map[string]any) (buildkite.Job, error) {
	req, err := client.NewRequest(ctx, "PUT", organizationJobPath(organization, jobID, "unblock"), &unblockJobOptions{
		Fields: fields,
	})
	if err != nil {
		return buildkite.Job{}, err
	}

	var job buildkite.Job
	if _, err := client.Do(req, &job); err != nil {
		return buildkite.Job{}, err
	}

	return job, nil
}
