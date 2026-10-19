package job

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	buildkite "github.com/buildkite/go-buildkite/v5"
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

func getJobEnvironment(ctx context.Context, client *buildkite.Client, organization, jobID string) (buildkite.JobEnvs, error) {
	req, err := client.NewRequest(ctx, http.MethodGet, organizationJobPath(organization, jobID, "env"), nil)
	if err != nil {
		return buildkite.JobEnvs{}, fmt.Errorf("could not create job environment request")
	}
	req.Header.Set("Accept", "application/json")

	var environment buildkite.JobEnvs
	resp, err := client.Do(req, &environment)
	if err != nil {
		// SDK errors can contain raw response bodies or decoded values. Do not
		// wrap them: environment data must only reach the requested output.
		if resp != nil && resp.StatusCode >= 400 {
			return buildkite.JobEnvs{}, fmt.Errorf("could not read job environment (HTTP %d); check the job UUID, organization and read_job_env token scope", resp.StatusCode)
		}
		return buildkite.JobEnvs{}, fmt.Errorf("could not read job environment: request failed or response was invalid")
	}
	if environment.EnvironmentVariables == nil {
		environment.EnvironmentVariables = map[string]string{}
	}
	return environment, nil
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

func createVNCSession(ctx context.Context, client *buildkite.Client, organization, jobID string) (vncSession, error) {
	req, err := client.NewRequest(ctx, http.MethodPost, organizationJobPath(organization, jobID, "vnc-session"), nil)
	if err != nil {
		return vncSession{}, err
	}
	req.Header.Set("Accept", "application/json")

	var session vncSession
	if _, err := client.Do(req, &session); err != nil {
		return vncSession{}, err
	}
	if err := session.validate(); err != nil {
		return vncSession{}, err
	}

	return session, nil
}

func createSSHSession(ctx context.Context, client *buildkite.Client, organization, jobID string) (sshSession, error) {
	req, err := client.NewRequest(ctx, http.MethodPost, organizationJobPath(organization, jobID, "ssh-session"), nil)
	if err != nil {
		return sshSession{}, err
	}
	req.Header.Set("Accept", "application/json")

	var session sshSession
	if _, err := client.Do(req, &session); err != nil {
		return sshSession{}, err
	}
	if err := session.validate(); err != nil {
		return sshSession{}, err
	}

	return session, nil
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

func retryJob(ctx context.Context, client *buildkite.Client, organization, jobID string) (buildkite.Job, error) {
	req, err := client.NewRequest(ctx, "PUT", organizationJobPath(organization, jobID, "retry"), nil)
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
