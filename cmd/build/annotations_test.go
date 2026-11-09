package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestAnnotationsCmd(t *testing.T) {
	const annotationsPath = "/v2/organizations/acme/pipelines/widgets/builds/42/annotations"
	const first = `{"id":"build-note","context":"coverage","style":"warning","scope":"build","priority":7,"body_html":"<p>Coverage &lt; 80%</p>\n","created_at":"2026-10-17T01:02:03Z","updated_at":"2026-10-18T04:05:06Z"}`
	const second = `{"id":"job-note","context":"tests","style":"error","scope":"job","job_id":"job-123","priority":3,"body_html":"<pre>Assertion failed\nexpected 2</pre>"}`
	for _, tt := range []struct {
		name, format, failAt, wantError string
		args                            []string
		pipelines                       []string
		empty, stored, noInput          bool
	}{
		{name: "qualified build default JSON"},
		{name: "number and qualified pipeline YAML", args: []string{"42", "-p", "acme/widgets", "--yaml"}, format: "yaml"},
		{name: "URL target credentials", args: []string{"https://buildkite.com/acme/widgets/builds/42", "--json"}, stored: true},
		{name: "plain pipeline", args: []string{"42", "-p", "widgets", "-o", "json"}, pipelines: []string{"unused"}},
		{name: "unique configured pipeline", args: []string{"42"}, pipelines: []string{"widgets"}},
		{name: "ambiguous default JSON", args: []string{"42"}, pipelines: []string{"widgets", "other"}, wantError: "use --pipeline"},
		{name: "ambiguous YAML", args: []string{"42", "--yaml"}, pipelines: []string{"widgets", "other"}, wantError: "use --pipeline"},
		{name: "ambiguous text with configured no input", args: []string{"42", "--text"}, pipelines: []string{"widgets", "other"}, noInput: true, wantError: "use --pipeline"},
		{name: "text", args: []string{"acme/widgets/42", "--text"}, format: "text"},
		{name: "empty JSON", empty: true},
		{name: "empty YAML", args: []string{"acme/widgets/42", "--output", "yaml"}, empty: true, format: "yaml"},
		{name: "empty text", args: []string{"acme/widgets/42", "--text"}, empty: true, format: "text"},
		{name: "forbidden", failAt: "1", wantError: "requires read_builds"},
		{name: "later page error", failAt: "3", wantError: "failed to list annotations"},
		{name: "invalid build", args: []string{"not-a-build"}, wantError: "unable to parse"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var pages []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != annotationsPath || r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer target-token" {
					t.Errorf("unexpected request or credentials: %s %s", r.Method, r.URL)
					http.NotFound(w, r)
					return
				}
				query := r.URL.Query()
				if query.Get("scope") != "all" || query.Get("per_page") != "100" || query.Has("omit_body") {
					t.Errorf("lost annotation scopes or bodies: %s", r.URL)
				}
				page := query.Get("page")
				pages = append(pages, page)
				if page == tt.failAt {
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"message":"Access denied"}`))
					return
				}
				if tt.empty {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				switch page {
				case "1":
					// A short page with a nonsequential next link must not stop retrieval.
					w.Header().Set("Link", fmt.Sprintf(`<https://api.buildkite.com%s?page=3>; rel="next"`, annotationsPath))
					_, _ = fmt.Fprintf(w, "[%s]", first)
				case "3":
					_, _ = fmt.Fprintf(w, "[%s]", second)
				default:
					t.Errorf("unexpected page: %s", page)
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
			t.Setenv("BUILDKITE_API_TOKEN", "target-token")
			t.Setenv("BUILDKITE_NO_INPUT", fmt.Sprint(tt.noInput))
			keyring.MockForTesting()
			t.Cleanup(keyring.ResetForTesting)
			if tt.stored {
				t.Setenv("BUILDKITE_API_TOKEN", "")
				if err := keyring.New().Set("acme", "target-token"); err != nil {
					t.Fatal(err)
				}
				if err := keyring.New().Set("other-org", "wrong-token"); err != nil {
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
			var cmd AnnotationsCmd
			var stdout, stderr bytes.Buffer
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(&stdout, &stderr))
			ctx, err := parser.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			// Text uses the standard pager's process stdout, even without a TTY.
			textFile, err := os.CreateTemp(t.TempDir(), "stdout")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = textFile.Close() })
			originalStdout := os.Stdout
			os.Stdout = textFile
			t.Cleanup(func() { os.Stdout = originalStdout })
			err = cmd.Run(ctx, cli.Globals{Quiet: true, NoPager: true})
			text, readErr := os.ReadFile(textFile.Name())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) || stdout.Len() != 0 || len(text) != 0 {
					t.Fatalf("error = %v, output = %q %q; want %q and no partial output", err, stdout.String(), text, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantPages := []string{"1", "3"}
			if tt.empty {
				wantPages = []string{"1"}
			}
			if !slices.Equal(pages, wantPages) {
				t.Fatalf("pages = %v, want %v", pages, wantPages)
			}
			if tt.format == "text" {
				wants := []string{"acme/widgets/42 (2)", "coverage [warning]", "<p>Coverage &lt; 80%</p>\n", "tests [error]", "scope: job", "Job: job-123", "<pre>Assertion failed\nexpected 2</pre>"}
				if tt.empty {
					wants = []string{"acme/widgets/42 (0)", "No annotations found."}
				}
				for _, want := range wants {
					if !strings.Contains(string(text), want) {
						t.Errorf("text missing %q: %s", want, text)
					}
				}
				return
			}
			// Decode independently of the output model, including REST field names.
			var result struct {
				Organization string `json:"organization" yaml:"organization"`
				Pipeline     string `json:"pipeline" yaml:"pipeline"`
				BuildNumber  int    `json:"build_number" yaml:"build_number"`
				Annotations  []struct {
					ID        string `json:"id" yaml:"id"`
					Context   string `json:"context" yaml:"context"`
					Style     string `json:"style" yaml:"style"`
					Scope     string `json:"scope" yaml:"scope"`
					JobID     string `json:"job_id" yaml:"job_id"`
					Priority  int    `json:"priority" yaml:"priority"`
					BodyHTML  string `json:"body_html" yaml:"body_html"`
					CreatedAt string `json:"created_at" yaml:"created_at"`
					UpdatedAt string `json:"updated_at" yaml:"updated_at"`
				} `json:"annotations" yaml:"annotations"`
			}
			if tt.format == "yaml" {
				err = yaml.Unmarshal(stdout.Bytes(), &result)
			} else {
				err = json.Unmarshal(stdout.Bytes(), &result)
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Organization != "acme" || result.Pipeline != "widgets" || result.BuildNumber != 42 || result.Annotations == nil {
				t.Fatalf("wrong build association or null list: %s", stdout.String())
			}
			if tt.empty {
				if len(result.Annotations) != 0 {
					t.Fatalf("nonempty result: %s", stdout.String())
				}
				return
			}
			if len(result.Annotations) != 2 {
				t.Fatalf("lost annotations: %s", stdout.String())
			}
			a, b := result.Annotations[0], result.Annotations[1]
			if a.ID != "build-note" || a.Context != "coverage" || a.Style != "warning" || a.Scope != "build" || a.Priority != 7 || a.BodyHTML != "<p>Coverage &lt; 80%</p>\n" || a.CreatedAt != "2026-10-17T01:02:03Z" || a.UpdatedAt != "2026-10-18T04:05:06Z" || b.ID != "job-note" || b.JobID != "job-123" || b.Scope != "job" || b.BodyHTML != "<pre>Assertion failed\nexpected 2</pre>" {
				t.Fatalf("lost annotation fields: %s", stdout.String())
			}
		})
	}
}
