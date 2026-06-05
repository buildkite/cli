package job

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestRetryJobUsesOrganizationEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/v2/organizations/buildkite/jobs/job-1/retry" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if len(body) != 0 {
			t.Fatalf("body = %q, want empty", body)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"job-2","state":"scheduled","retried_in_job_id":"job-2","web_url":"https://buildkite.com/buildkite/cli/builds/42#job-2"}`))
	}))
	defer server.Close()

	client, err := buildkite.NewOpts(
		buildkite.WithBaseURL(server.URL),
		buildkite.WithTokenAuth("test-token"),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	job, err := retryJob(context.Background(), client, "buildkite", "job-1")
	if err != nil {
		t.Fatalf("retryJob() error = %v", err)
	}
	if job.ID != "job-2" {
		t.Fatalf("job = %#v", job)
	}
}
