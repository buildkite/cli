package build

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/keyring"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func TestAnnotationsTerminalSettings(t *testing.T) {
	type testCase struct{ name, source, scenario, format string }
	cases := []testCase{
		{name: "ambiguous default JSON", scenario: "no input"},
		{name: "ambiguous explicit JSON", scenario: "no input", format: "--json"},
		{name: "ambiguous YAML", scenario: "no input", format: "--yaml"},
		{name: "interactive control", scenario: "interactive", format: "--text"},
	}
	for _, source := range []string{"config", "environment", "flags"} {
		for _, scenario := range []string{"no input", "quiet repository", "quiet cross org"} {
			format := "--json"
			if scenario == "no input" {
				format = "--text"
			}
			cases = append(cases, testCase{name: source + "/" + scenario, source: source, scenario: scenario, format: format})
		}
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// Real terminal input distinguishes no-input from the non-TTY fallback;
			// terminal stdout also makes an accidentally enabled spinner observable.
			master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NONBLOCK, 0)
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
			if !term.IsTerminal(int(terminal.Fd())) {
				t.Fatal("test requires a real terminal")
			}
			// A regressed prompt can select a valid pipeline instead of hanging.
			if _, err := master.WriteString("1\n"); err != nil {
				t.Fatal(err)
			}
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer stderr.Close()
			stdin, stdout, oldStderr := os.Stdin, os.Stdout, os.Stderr
			os.Stdin, os.Stdout, os.Stderr = terminal, terminal, stderr
			defer func() { os.Stdin, os.Stdout, os.Stderr = stdin, stdout, oldStderr }()

			t.Chdir(t.TempDir())
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")
			t.Setenv("BUILDKITE_API_TOKEN", "")
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
			globals := cli.Globals{NoPager: true}
			noInput := tt.scenario == "no input" && tt.source != ""
			quiet := strings.HasPrefix(tt.scenario, "quiet")
			switch tt.source {
			case "config":
				settings := fmt.Sprintf("no_input: %t\nquiet: %t\n", noInput, quiet)
				if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "bk.yaml"), []byte(settings), 0o600); err != nil {
					t.Fatal(err)
				}
			case "environment":
				t.Setenv("BUILDKITE_NO_INPUT", fmt.Sprint(noInput))
				t.Setenv("BUILDKITE_QUIET", fmt.Sprint(quiet))
			case "flags":
				globals.NoInput, globals.Quiet = noInput, quiet
			}
			args := []string{"42"}
			switch tt.scenario {
			case "quiet repository":
				repo, err := git.PlainInit(".", false)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{"https://github.com/acme/widgets.git"}}); err != nil {
					t.Fatal(err)
				}
			case "quiet cross org":
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "other-org")
				if err := keyring.New().Set("other-org", "wrong-token"); err != nil {
					t.Fatal(err)
				}
				args = []string{"acme/widgets/42"}
			default:
				if err := os.WriteFile(".bk.yaml", []byte("selected_org: acme\npipelines: [widgets, other]\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tt.format != "" {
				args = append(args, tt.format)
			}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer target-token" {
					t.Error("unexpected method or organization credentials")
				}
				if quiet {
					// Exceed a spinner tick for both repository lookup and loading.
					time.Sleep(350 * time.Millisecond)
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v2/organizations/acme/pipelines":
					_, _ = w.Write([]byte(`[{"slug":"widgets","repository":"https://github.com/acme/widgets.git"}]`))
				case "/v2/organizations/acme/pipelines/widgets/builds/42/annotations":
					_, _ = w.Write([]byte(`[]`))
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			var cmd AnnotationsCmd
			var structured bytes.Buffer
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(&structured, stderr))
			ctx, err := parser.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			err = cmd.Run(ctx, globals)
			if tt.scenario == "no input" {
				if err == nil || !strings.Contains(err.Error(), "use --pipeline") || requests.Load() != 0 || structured.Len() != 0 {
					t.Fatalf("terminal selection was not suppressed: error=%v requests=%d output=%q", err, requests.Load(), structured.String())
				}
			}
			var terminalOutput bytes.Buffer
			chunk := make([]byte, 4096)
			for {
				count, readErr := unix.Read(int(master.Fd()), chunk)
				if count > 0 {
					terminalOutput.Write(chunk[:count])
				}
				if readErr == unix.EINTR {
					continue
				}
				if readErr == unix.EAGAIN {
					break
				}
				if readErr != nil {
					t.Fatal(readErr)
				}
				if count == 0 {
					break
				}
			}
			if tt.scenario != "no input" && err != nil {
				t.Fatalf("error=%v terminal=%q", err, terminalOutput.String())
			}
			prompted := strings.Contains(terminalOutput.String(), "Select a pipeline")
			if prompted != (tt.scenario == "interactive") {
				t.Fatalf("unexpected terminal prompt: %q", terminalOutput.String())
			}
			if tt.scenario != "no input" {
				wantRequests := int32(1)
				if tt.scenario == "quiet repository" {
					wantRequests = 2
				}
				if requests.Load() != wantRequests {
					t.Fatalf("requests=%d, want %d", requests.Load(), wantRequests)
				}
			}
			progress, err := os.ReadFile(stderr.Name())
			if err != nil {
				t.Fatal(err)
			}
			if quiet && (strings.Contains(string(progress), "Resolving pipeline") || strings.Contains(string(progress), "Loading annotations")) {
				t.Fatalf("quiet setting lost: %q", progress)
			}
		})
	}
}
