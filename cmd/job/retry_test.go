package job

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"gopkg.in/yaml.v3"
)

func TestRetryCmdOutput(t *testing.T) {
	const originalID = "0190046e-e199-453b-a302-a21a4d649d31"
	const retryID = "0198d108-a532-4a62-9bd7-b2e744bf5c45"
	const webURL = "https://buildkite.com/buildkite/cli/builds/42#" + retryID
	for _, tt := range []struct {
		name   string
		flags  []string
		config string
		format string
		fail   bool
	}{
		{name: "json overrides text config", flags: []string{"--json"}, config: "text", format: "json"},
		{name: "yaml", flags: []string{"--yaml"}, config: "json", format: "yaml"},
		{name: "text", flags: []string{"--text"}, config: "json", format: "text"},
		{name: "configured output", config: "json", format: "json"},
		{name: "output flag", flags: []string{"-o", "json"}, config: "text", format: "json"},
		{name: "failure has no receipt", flags: []string{"--json"}, config: "json", fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodPut || r.URL.Path != "/v2/organizations/buildkite/jobs/"+originalID+"/retry" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if tt.fail {
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"message":"Job cannot be retried"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(buildkite.Job{ID: retryID, State: "scheduled", WebURL: webURL})
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "buildkite")
			t.Setenv("BUILDKITE_OUTPUT_FORMAT", tt.config)

			var cmd RetryCmd
			var stdout, stderr bytes.Buffer
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(&stdout, &stderr))
			ctx, err := parser.Parse(append([]string{originalID}, tt.flags...))
			if err != nil {
				t.Fatal(err)
			}
			err = cmd.Run(ctx, cli.Globals{NoInput: true, Quiet: true})
			if requests != 1 {
				t.Fatalf("requests = %d, want exactly one retry", requests)
			}
			if tt.fail {
				if err == nil || stdout.Len() != 0 {
					t.Fatalf("error = %v, stdout = %q; want error and no receipt", err, stdout.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.format == "text" {
				if got, want := stdout.String(), "Successfully retried job: "+webURL+"\n"; got != want {
					t.Fatalf("output = %q, want %q", got, want)
				}
				return
			}
			var receipt map[string]any
			if tt.format == "json" {
				err = json.Unmarshal(stdout.Bytes(), &receipt)
			} else {
				err = yaml.Unmarshal(stdout.Bytes(), &receipt)
			}
			if err != nil {
				t.Fatalf("decode receipt: %v; output = %q", err, stdout.String())
			}
			if receipt["id"] != retryID || receipt["state"] != "scheduled" {
				t.Fatalf("receipt = %v, want new retry UUID and scheduled state", receipt)
			}
		})
	}
}

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
