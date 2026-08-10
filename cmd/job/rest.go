package job

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

type unblockJobOptions struct {
	Fields map[string]any `json:"fields,omitempty"`
}

type vncSession struct {
	Endpoint    string                `json:"endpoint"`
	AccessToken string                `json:"access_token"`
	ExpiresAt   time.Time             `json:"expires_at"`
	VNC         vncSessionCredentials `json:"vnc"`
}

type vncSessionCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
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

func (s vncSession) validate() error {
	switch {
	case s.Endpoint == "":
		return errors.New("buildkite API returned a VNC session without an endpoint")
	case s.AccessToken == "":
		return errors.New("buildkite API returned a VNC session without an access token")
	case s.ExpiresAt.IsZero():
		return errors.New("buildkite API returned a VNC session without an expiry")
	case s.VNC.Username == "":
		return errors.New("buildkite API returned a VNC session without a VNC username")
	case s.VNC.Password == "":
		return errors.New("buildkite API returned a VNC session without a VNC password")
	default:
		return nil
	}
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
