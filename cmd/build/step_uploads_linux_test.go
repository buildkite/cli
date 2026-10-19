package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"golang.org/x/sys/unix"
)

// A terminal is required: non-TTY input already prevents pipeline selection,
// and non-TTY output suppresses the spinner even when quiet settings are lost.
func TestStepUploadsTerminalSettings(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		settings    string
		blocked     bool
		crossOrg    bool
		interactive bool
		progress    bool
	}{
		{name: "ambiguous JSON", args: []string{"42", "--json"}, blocked: true},
		{name: "ambiguous default JSON", args: []string{"42"}, blocked: true},
		{name: "ambiguous YAML", args: []string{"42", "--yaml"}, blocked: true},
		{name: "config no input", args: []string{"42", "--text"}, settings: "config", blocked: true},
		{name: "environment no input", args: []string{"42", "--text"}, settings: "environment", blocked: true},
		{name: "config quiet", settings: "config"},
		{name: "environment quiet", settings: "environment"},
		{name: "config quiet after org override", settings: "config", crossOrg: true},
		{name: "environment quiet after org override", settings: "environment", crossOrg: true},
		{name: "interactive control", args: []string{"42", "--text"}, interactive: true},
		{name: "progress control", progress: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer master.Close()
			if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
				t.Fatal(err)
			}
			n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
			if err != nil {
				t.Fatal(err)
			}
			terminal, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer terminal.Close()
			// A regressed prompt must complete and fail assertions, not hang.
			if _, err := master.WriteString("1\n"); err != nil {
				t.Fatal(err)
			}
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer stderr.Close()
			stdin, stdout, originalStderr := os.Stdin, os.Stdout, os.Stderr
			os.Stdin, os.Stdout, os.Stderr = terminal, terminal, stderr
			defer func() { os.Stdin, os.Stdout, os.Stderr = stdin, stdout, originalStderr }()

			t.Chdir(t.TempDir())
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")
			t.Setenv("BUILDKITE_OUTPUT_FORMAT", "json")
			t.Setenv("BUILDKITE_NO_INPUT", "")
			t.Setenv("BUILDKITE_QUIET", "")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			t.Setenv(keyring.CredentialStorePathEnv, filepath.Join(t.TempDir(), "credentials.json"))
			keyring.MockForTesting()
			t.Cleanup(keyring.ResetForTesting)
			if err := keyring.New().Set("acme", "target-token"); err != nil {
				t.Fatal(err)
			}
			if err := keyring.New().Set("other-org", "other-token"); err != nil {
				t.Fatal(err)
			}
			if tt.crossOrg {
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "other-org")
			}
			if err := os.WriteFile(".bk.yaml", []byte("selected_org: acme\npipelines: [widgets, other]\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch tt.settings {
			case "config":
				if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "bk.yaml"), []byte("no_input: true\nquiet: true\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "environment":
				t.Setenv("BUILDKITE_NO_INPUT", "true")
				t.Setenv("BUILDKITE_QUIET", "true")
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodGet || r.URL.Path != "/v2/organizations/acme/pipelines/widgets/builds/42/step-uploads" || r.Header.Get("Authorization") != "Bearer target-token" {
					t.Errorf("wrong request target or credentials: %s %s", r.Method, r.URL)
				}
				if tt.progress {
					deadline := time.Now().Add(5 * time.Second)
					for {
						progress, err := os.ReadFile(stderr.Name())
						if err != nil {
							t.Error(err)
							break
						}
						if strings.Contains(string(progress), "Loading step uploads") {
							break
						}
						if time.Now().After(deadline) {
							t.Error("timed out waiting for spinner output")
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
				} else if !tt.blocked && !tt.interactive {
					// Exceed a spinner tick to make a lost quiet setting observable.
					time.Sleep(350 * time.Millisecond)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"items":[],"links":{}}`))
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			args := tt.args
			if len(args) == 0 {
				args = []string{"acme/widgets/42", "--json"}
			}
			var cmd StepUploadsCmd
			var structured, diagnostics bytes.Buffer
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(&structured, &diagnostics))
			ctx, err := parser.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			err = cmd.Run(ctx, cli.Globals{NoPager: true})
			if tt.blocked {
				if err == nil || !strings.Contains(err.Error(), "use --pipeline") || requests != 0 || structured.Len() != 0 {
					t.Fatalf("selection was not blocked: err=%v requests=%d output=%q", err, requests, structured.String())
				}
			} else {
				if err != nil || requests != 1 {
					t.Fatalf("command failed: err=%v requests=%d", err, requests)
				}
				if !tt.interactive {
					var result stepUploadsOutput
					if err := json.Unmarshal(structured.Bytes(), &result); err != nil || result.Organization != "acme" || result.StepUploads == nil {
						t.Fatalf("invalid result: %q, err=%v", structured.String(), err)
					}
				}
			}
			progress, err := os.ReadFile(stderr.Name())
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(progress), "Loading step uploads"); !tt.interactive && got != tt.progress {
				t.Fatalf("spinner visible=%t, want %t: %q", got, tt.progress, progress)
			}
		})
	}
}
