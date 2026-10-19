package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"gopkg.in/yaml.v3"
)

func TestRetryFailedJobsCmd(t *testing.T) {
	for _, tt := range []struct {
		name         string
		args         []string
		states       string
		format       string
		count        int
		status       int
		stored       bool
		missingAuth  bool
		ambiguous    bool
		confirm      string
		noYes        bool
		envNoInput   bool
		userNoInput  bool
		response     string
		configFormat string
		failOutput   bool
		wantError    string
		noWrite      bool
	}{
		{name: "default all failures JSON", count: 3},
		{name: "explicit empty states", args: []string{"acme/widgets/42", "--states="}, count: 2},
		{name: "selected states YAML", args: []string{"42", "-p", "acme/widgets", "--states", "failed,soft_failed", "--yaml"}, states: "failed,soft_failed", format: "yaml", count: 2},
		{name: "other failure types text", args: []string{"acme/widgets/42", "--states", "canceled,expired,timed_out", "--text"}, states: "canceled,expired,timed_out", format: "text", count: 4},
		{name: "URL target org stored credentials", args: []string{"https://buildkite.com/acme/widgets/builds/42"}, stored: true, count: 1},
		{name: "zero is still accepted", count: 0},
		{name: "interactive confirmation JSON", confirm: "yes", noYes: true, count: 3},
		{name: "declined", confirm: "n", noYes: true, noWrite: true},
		{name: "no input without authorization", noYes: true, wantError: "--no-input", noWrite: true},
		{name: "environment no input cross org", envNoInput: true, noYes: true, wantError: "--no-input", noWrite: true},
		{name: "user configured no input cross org", userNoInput: true, noYes: true, wantError: "--no-input", noWrite: true},
		{name: "yes overrides configured no input", envNoInput: true, count: 1},
		{name: "no target credentials", missingAuth: true, wantError: "no stored credentials", noWrite: true},
		{name: "ambiguous JSON does not prompt", args: []string{"42", "--json"}, ambiguous: true, wantError: "use --pipeline", noWrite: true},
		{name: "ambiguous YAML does not prompt", args: []string{"42", "--yaml"}, ambiguous: true, wantError: "use --pipeline", noWrite: true},
		{name: "invalid build", args: []string{"not-a-build"}, wantError: "unable to parse", noWrite: true},
		{name: "extra build component", args: []string{"acme/widgets/42/99"}, wantError: "unable to parse", noWrite: true},
		{name: "empty organization", args: []string{"/widgets/42"}, wantError: "unable to parse", noWrite: true},
		{name: "empty pipeline", args: []string{"acme//42"}, wantError: "unable to parse", noWrite: true},
		{name: "zero build", args: []string{"acme/widgets/0"}, wantError: "positive", noWrite: true},
		{name: "negative build", args: []string{"acme/widgets/-42"}, wantError: "positive", noWrite: true},
		{name: "lookalike URL host", args: []string{"https://buildkiteXcom/acme/widgets/builds/42"}, wantError: "unable to parse", noWrite: true},
		{name: "prefixed URL", args: []string{"https://evil.example/https://buildkite.com/acme/widgets/builds/42"}, wantError: "unable to parse", noWrite: true},
		{name: "path traversal", args: []string{"acme/../42"}, wantError: "unable to parse", noWrite: true},
		{name: "extra pipeline component", args: []string{"42", "-p", "acme/widgets/other"}, wantError: "invalid --pipeline", noWrite: true},
		{name: "conflicting pipeline", args: []string{"acme/widgets/42", "-p", "other"}, wantError: "conflicts with build", noWrite: true},
		{name: "conflicting pipeline org", args: []string{"acme/widgets/42", "-p", "other/widgets"}, wantError: "conflicts with build", noWrite: true},
		{name: "matching explicit pipeline", args: []string{"acme/widgets/42", "-p", "acme/widgets"}, count: 1},
		{name: "invalid state", args: []string{"acme/widgets/42", "--states", "passed"}, wantError: "invalid state", noWrite: true},
		{name: "trailing comma cannot broaden states", args: []string{"acme/widgets/42", "--states", "failed,"}, wantError: "invalid state", noWrite: true},
		{name: "forbidden", status: http.StatusForbidden, wantError: "Access denied"},
		{name: "invalid request", status: http.StatusBadRequest, wantError: "Access denied"},
		{name: "missing queued count", response: `{}`, wantError: "accepted, but its queued-job receipt is unavailable"},
		{name: "null queued count", response: `{"retried_jobs_count":null}`, wantError: "check the build before resubmitting"},
		{name: "negative queued count", response: `{"retried_jobs_count":-1}`, wantError: "check the build before resubmitting"},
		{name: "null response", response: `null`, wantError: "check the build before resubmitting"},
		{name: "malformed accepted response", response: `{`, wantError: "accepted, but its queued-job receipt is unavailable"},
		{name: "unexpected success status", status: http.StatusOK, response: `{"retried_jobs_count":3}`, wantError: "expected 202 Accepted"},
		{name: "invalid configured format cannot write", configFormat: "bogus", wantError: "unsupported output format", noWrite: true},
		{name: "explicit format overrides bad configuration", configFormat: "bogus", args: []string{"acme/widgets/42", "--json"}, count: 1},
		{name: "output failure after acceptance", failOutput: true, count: 3, wantError: "accepted, but writing its receipt failed; check the build before resubmitting"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodPut || r.URL.Path != "/v2/organizations/acme/pipelines/widgets/builds/42/retry_failed_jobs" || r.URL.RawQuery != "" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					t.Error("missing target credentials or JSON content type")
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["states"] != tt.states || (tt.states == "" && len(body) != 0) || (tt.states != "" && len(body) != 1) {
					t.Errorf("body = %v, want only states %q (omitted for default)", body, tt.states)
				}
				w.Header().Set("Content-Type", "application/json")
				if tt.status >= 400 {
					w.WriteHeader(tt.status)
					_, _ = io.WriteString(w, `{"message":"Access denied"}`)
					return
				}
				status := tt.status
				if status == 0 {
					status = http.StatusAccepted
				}
				w.WriteHeader(status)
				if tt.response != "" {
					_, _ = io.WriteString(w, tt.response)
					return
				}
				// A stale build state is not evidence that retries completed.
				_, _ = fmt.Fprintf(w, `{"id":"build-uuid","number":42,"pipeline":{"slug":"widgets","id":"pipeline-uuid"},"state":"failed","retried_jobs_count":%d,"web_url":"https://buildkite.com/acme/widgets/builds/42","env":{"PRIVATE":"not-output"}}`, tt.count)
			}))
			defer server.Close()
			t.Chdir(t.TempDir())
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_NO_INPUT", "")
			if tt.envNoInput {
				t.Setenv("BUILDKITE_NO_INPUT", "true")
			}
			if tt.userNoInput {
				if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "bk.yaml"), []byte("no_input: true\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			t.Setenv(keyring.CredentialStorePathEnv, filepath.Join(t.TempDir(), "credentials.json"))
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "other-org")
			t.Setenv("BUILDKITE_OUTPUT_FORMAT", "json")
			if tt.configFormat != "" {
				t.Setenv("BUILDKITE_OUTPUT_FORMAT", tt.configFormat)
			}
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			keyring.MockForTesting()
			t.Cleanup(keyring.ResetForTesting)
			if tt.stored || tt.missingAuth {
				t.Setenv("BUILDKITE_API_TOKEN", "")
				if err := keyring.New().Set("other-org", "wrong-token"); err != nil {
					t.Fatal(err)
				}
				if tt.stored {
					if err := keyring.New().Set("acme", "test-token"); err != nil {
						t.Fatal(err)
					}
				}
			}
			if tt.ambiguous {
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")
				if err := os.WriteFile(".bk.yaml", []byte("selected_org: acme\npipelines: [widgets, other]\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var prompt string
			if tt.confirm != "" {
				stdin, err := os.CreateTemp(t.TempDir(), "stdin")
				if err != nil {
					t.Fatal(err)
				}
				defer stdin.Close()
				_, _ = fmt.Fprintln(stdin, tt.confirm)
				_, _ = stdin.Seek(0, 0)
				stderr, err := os.CreateTemp(t.TempDir(), "stderr")
				if err != nil {
					t.Fatal(err)
				}
				defer stderr.Close()
				oldIn, oldErr := os.Stdin, os.Stderr
				os.Stdin, os.Stderr = stdin, stderr
				defer func() { os.Stdin, os.Stderr = oldIn, oldErr }()
				defer func() {
					data, _ := os.ReadFile(stderr.Name())
					prompt = string(data)
					for _, want := range []string{"acme/widgets/42", "all retryable failure types", "Not limited to infrastructure failures", "side effects, including deployments"} {
						if !strings.Contains(prompt, want) {
							t.Errorf("confirmation missing %q: %s", want, prompt)
						}
					}
				}()
			}
			args := tt.args
			if args == nil {
				args = []string{"acme/widgets/42"}
			}
			var cmd RetryFailedJobsCmd
			var stdout, stderr bytes.Buffer
			var writer io.Writer = &stdout
			if tt.failOutput {
				closed, err := os.CreateTemp(t.TempDir(), "closed-output")
				if err != nil {
					t.Fatal(err)
				}
				if err := closed.Close(); err != nil {
					t.Fatal(err)
				}
				writer = closed
			}
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(writer, &stderr))
			ctx, err := parser.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			err = cmd.Run(ctx, cli.Globals{Yes: !tt.noYes, NoInput: tt.confirm == "" && !tt.envNoInput && !tt.userNoInput, Quiet: true})
			wantRequests := 1
			if tt.noWrite {
				wantRequests = 0
			}
			if requests != wantRequests {
				t.Fatalf("requests = %d, want %d", requests, wantRequests)
			}
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) || stdout.Len() != 0 {
					t.Fatalf("error = %v, output = %q; want %q and no receipt", err, stdout.String(), tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.noWrite {
				if stdout.Len() != 0 {
					t.Fatalf("unexpected receipt: %s", &stdout)
				}
				return
			}
			if tt.format == "text" {
				if got, want := stdout.String(), "Retry request accepted for acme/widgets/42: 4 jobs queued for retry (not completed). https://buildkite.com/acme/widgets/builds/42\n"; got != want {
					t.Fatalf("output = %q, want %q", got, want)
				}
				return
			}
			var result map[string]any
			if tt.format == "yaml" {
				err = yaml.Unmarshal(stdout.Bytes(), &result)
			} else {
				err = json.Unmarshal(stdout.Bytes(), &result)
			}
			if err != nil {
				t.Fatal(err)
			}
			if result["organization"] != "acme" || result["pipeline"] != "widgets" || fmt.Sprint(result["build_number"]) != "42" || fmt.Sprint(result["retried_jobs_count"]) != fmt.Sprint(tt.count) || result["web_url"] != "https://buildkite.com/acme/widgets/builds/42" {
				t.Fatalf("incorrect receipt: %v", result)
			}
			if len(result) != 6 || !strings.Contains(fmt.Sprint(result["note"]), "not completed or successful") {
				t.Fatalf("missing async caveat or unexpected build data: %v", result)
			}
		})
	}
}
