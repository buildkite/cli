package build

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"gopkg.in/yaml.v3"
)

func TestStepUploadsCmd(t *testing.T) {
	const uploadsPath = "/v2/organizations/acme/pipelines/widgets/builds/42/step-uploads"
	const cursor = "opaque+/=&cursor"
	for _, tt := range []struct {
		name      string
		args      []string
		empty     bool
		status    int
		later     bool
		badLink   bool
		stored    bool
		pipelines []string
		wantError string
	}{
		{name: "qualified JSON", args: []string{"acme/widgets/42", "--json"}},
		{name: "number and pipeline YAML", args: []string{"42", "-p", "acme/widgets", "--yaml"}},
		{name: "URL and target org credentials", args: []string{"https://buildkite.com/acme/widgets/builds/42"}, stored: true},
		{name: "unique configured pipeline", args: []string{"42"}, pipelines: []string{"widgets"}},
		{name: "ambiguous default JSON", args: []string{"42"}, pipelines: []string{"widgets", "other"}, wantError: "use --pipeline"},
		{name: "ambiguous explicit JSON", args: []string{"42", "--json"}, pipelines: []string{"widgets", "other"}, wantError: "use --pipeline"},
		{name: "ambiguous YAML", args: []string{"42", "--yaml"}, pipelines: []string{"widgets", "other"}, wantError: "use --pipeline"},
		{name: "text", args: []string{"acme/widgets/42", "--text"}},
		{name: "empty array", empty: true},
		{name: "empty text", args: []string{"acme/widgets/42", "--text"}, empty: true},
		{name: "expired history", status: http.StatusGone, wantError: "maximum lifetime"},
		{name: "later page expired", status: http.StatusGone, later: true, wantError: "maximum lifetime"},
		{name: "forbidden", status: http.StatusForbidden, wantError: "requires read_builds"},
		{name: "not found", status: http.StatusNotFound, wantError: "404"},
		{name: "later page denied", status: http.StatusForbidden, later: true, wantError: "requires read_builds"},
		{name: "bad continuation", badLink: true, wantError: "invalid step uploads next link"},
		{name: "invalid build", args: []string{"not-a-build"}, wantError: "unable to parse"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var cursors []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" || r.URL.Path != uploadsPath {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					http.NotFound(w, r)
					return
				}
				query := r.URL.Query()
				after := query.Get("after")
				cursors = append(cursors, after)
				if tt.status != 0 && (!tt.later || after != "") {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(`{"message":"Unavailable"}`))
					return
				}
				if tt.empty {
					_, _ = w.Write([]byte(`{"items":[],"links":{}}`))
					return
				}
				switch after {
				case "":
					if query.Get("per_page") != "100" || query.Has("page") {
						t.Errorf("incorrect initial pagination: %s", r.URL)
					}
					// A short page still has a next link. Ignore page-number Link headers.
					w.Header().Set("Link", `<https://api.buildkite.com/ignored?page=9>; rel="next"`)
					next := "https://api.buildkite.com" + uploadsPath + "?after=" + url.QueryEscape(cursor) + "&per_page=7"
					if tt.badLink {
						next = "https://api.buildkite.com" + uploadsPath + "?per_page=invalid"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"items": json.RawMessage(`[
							{"uuid":"pending","state":"pending","source":"job","source_job_id":"job-1"},
							{"uuid":"processing","state":"processing","source":"job","source_job_id":"job-2","created_jobs_count":null},
							{"uuid":"zero","state":"applied","source":"job","source_job_id":"job-3","created_jobs_count":0,"replace_existing_steps":true}
						]`),
						"links": map[string]string{"next": next},
					})
				case cursor:
					if query.Get("per_page") != "7" || query.Has("before") || query.Has("page") {
						t.Errorf("next link options not preserved: %s", r.URL)
					}
					_, _ = w.Write([]byte(`{"items":[
						{"uuid":"rejected","state":"rejected","source":"job","source_job_id":"job-4","rejection_type":"validation_error","message":"Check the pipeline upload command output for details."},
						{"uuid":"failed","state":"failed","source":"job","source_job_id":"job-5","message":"The step upload failed."},
						{"uuid":"applied","state":"applied","source":"job","source_job_id":"job-6","created_jobs_count":7,"created_at":"2026-10-18T10:15:32Z","processed_at":"2026-10-18T10:15:33Z"}
					],"links":{}}`))
				default:
					t.Errorf("invented cursor: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Chdir(t.TempDir())
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			t.Setenv(keyring.CredentialStorePathEnv, filepath.Join(t.TempDir(), "credentials.json"))
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "other-org")
			t.Setenv("BUILDKITE_OUTPUT_FORMAT", "json")
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			keyring.MockForTesting()
			t.Cleanup(keyring.ResetForTesting)
			if tt.stored {
				t.Setenv("BUILDKITE_API_TOKEN", "")
				if err := keyring.New().Set("acme", "test-token"); err != nil {
					t.Fatal(err)
				}
			}
			if len(tt.pipelines) > 0 {
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")
				data, err := yaml.Marshal(map[string]any{"selected_org": "acme", "pipelines": tt.pipelines})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(".bk.yaml", data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			args := tt.args
			if len(args) == 0 {
				args = []string{"acme/widgets/42"}
			}
			var cmd StepUploadsCmd
			var stdout, stderr bytes.Buffer
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(&stdout, &stderr))
			ctx, err := parser.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			// Text uses the pager's process stdout; capture it separately from Kong.
			capture, err := os.CreateTemp(t.TempDir(), "stdout")
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			originalStdout := os.Stdout
			os.Stdout = capture
			t.Cleanup(func() { os.Stdout = originalStdout })
			err = cmd.Run(ctx, cli.Globals{Quiet: true, NoPager: true})
			processOutput, readErr := os.ReadFile(capture.Name())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) || stdout.Len() != 0 || len(processOutput) != 0 {
					t.Fatalf("error = %v, output = %q/%q; want %q and no partial output", err, stdout.String(), processOutput, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(args, "--text") {
				text := string(processOutput)
				wants := []string{"acme/widgets/42", "maximum lifetime"}
				if tt.empty {
					wants = append(wants, "No step uploads found.")
				} else {
					wants = append(wants, "pending (pending)", "processing (processing)", "zero (applied)", "Source: job  Job: job-4", "Created jobs: -", "Created jobs: 0", "Created jobs: 7", "Replace existing steps: true", "Rejection: validation_error", "Message: Check the pipeline upload command output for details.", "Message: The step upload failed.")
				}
				for _, want := range wants {
					if !strings.Contains(text, want) {
						t.Errorf("text missing %q: %s", want, text)
					}
				}
				return
			}
			if len(processOutput) != 0 {
				t.Fatalf("structured output polluted by prompts: %q", processOutput)
			}
			var result stepUploadsOutput
			if slices.Contains(args, "--yaml") {
				err = yaml.Unmarshal(stdout.Bytes(), &result)
			} else {
				err = json.Unmarshal(stdout.Bytes(), &result)
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Organization != "acme" || result.Pipeline != "widgets" || result.BuildNumber != 42 || result.Note == "" {
				t.Fatalf("wrong build association or missing note: %+v", result)
			}
			if tt.empty {
				if result.StepUploads == nil || len(result.StepUploads) != 0 {
					t.Fatalf("expected empty array: %s", stdout.String())
				}
				return
			}
			if !slices.Equal(cursors, []string{"", cursor}) || len(result.StepUploads) != 6 {
				t.Fatalf("lost pages: cursors = %v, result = %+v", cursors, result)
			}
			u := result.StepUploads
			if u[0].CreatedJobsCount != nil || u[1].CreatedJobsCount != nil || u[2].CreatedJobsCount == nil || *u[2].CreatedJobsCount != 0 || u[5].CreatedJobsCount == nil || *u[5].CreatedJobsCount != 7 {
				t.Fatalf("lost missing/null/zero/nonzero count distinction: %+v", u)
			}
			if !u[2].ReplaceExistingSteps || u[3].RejectionType == nil || *u[3].RejectionType != "validation_error" || u[3].Message == nil || *u[3].Message != "Check the pipeline upload command output for details." || u[4].Message == nil || *u[4].Message != "The step upload failed." || u[5].CreatedAt == nil || u[5].ProcessedAt == nil {
				t.Fatalf("lost outcome details: %+v", u)
			}
			if !slices.Contains(args, "--yaml") {
				var wire struct {
					Uploads []map[string]any `json:"step_uploads"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &wire); err != nil {
					t.Fatal(err)
				}
				if _, exists := wire.Uploads[0]["created_jobs_count"]; exists || wire.Uploads[2]["created_jobs_count"] != float64(0) || wire.Uploads[5]["created_jobs_count"] != float64(7) {
					t.Fatalf("wrong JSON count contract: %s", stdout.String())
				}
			}
		})
	}
}
