package job

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
)

func TestCancelCmdCancelsSingleJobViaOrganizationEndpoint(t *testing.T) {
	const jobID = "0190046e-e199-453b-a302-a21a4d649d31"
	const webURL = "https://buildkite.com/buildkite/cli/builds/42#" + jobID
	for _, tt := range []struct {
		name   string
		state  string
		output string
		fail   bool
	}{
		{name: "scheduled job is canceled", state: "canceled", output: "Job canceled: "},
		{name: "running job is canceling", state: "canceling", output: "Job canceling: "},
		{name: "failure has no receipt", fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodPut || r.URL.Path != "/v2/organizations/buildkite/jobs/"+jobID+"/cancel" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if body, _ := io.ReadAll(r.Body); len(body) != 0 {
					t.Errorf("body = %q, want empty", body)
				}
				w.Header().Set("Content-Type", "application/json")
				if tt.fail {
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"message":"You can only cancel jobs that have started"}`))
					return
				}
				_, _ = w.Write([]byte(`{"id":"` + jobID + `","state":"` + tt.state + `","web_url":"` + webURL + `"}`))
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "buildkite")

			var cmd CancelCmd
			var stdout, stderr bytes.Buffer
			parser := kong.Must(&cmd, kong.Writers(&stdout, &stderr))
			ctx, err := parser.Parse([]string{jobID})
			if err != nil {
				t.Fatal(err)
			}
			err = cmd.Run(ctx, cli.Globals{Yes: true, NoInput: true, Quiet: true})
			if requests != 1 {
				t.Fatalf("requests = %d, want exactly one cancel", requests)
			}
			if tt.fail {
				if err == nil || !strings.Contains(err.Error(), "You can only cancel jobs that have started") || stdout.Len() != 0 {
					t.Fatalf("error = %v, stdout = %q; want error and no receipt", err, stdout.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got, want := stdout.String(), tt.output+webURL+"\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestCancelCmdRequiresConfirmationBeforeCancelling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request without confirmation: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "buildkite")

	var cmd CancelCmd
	parser := kong.Must(&cmd, kong.Writers(io.Discard, io.Discard))
	ctx, err := parser.Parse([]string{"0190046e-e199-453b-a302-a21a4d649d31"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Run(ctx, cli.Globals{NoInput: true, Quiet: true}); err == nil {
		t.Fatal("expected an error when confirmation is required but input is disabled")
	}
}
