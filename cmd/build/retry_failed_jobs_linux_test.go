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
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"golang.org/x/sys/unix"
)

func TestRetryFailedJobsRepositoryQuiet(t *testing.T) {
	for _, source := range []string{"config", "environment", "progress control"} {
		t.Run(source, func(t *testing.T) {
			// Non-TTY stdout suppresses progress regardless of quiet settings.
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
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer stderr.Close()
			stdout, oldStderr := os.Stdout, os.Stderr
			os.Stdout, os.Stderr = terminal, stderr
			defer func() { os.Stdout, os.Stderr = stdout, oldStderr }()

			t.Chdir(t.TempDir())
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")
			t.Setenv("BUILDKITE_OUTPUT_FORMAT", "json")
			t.Setenv("BUILDKITE_QUIET", "")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			t.Setenv(keyring.CredentialStorePathEnv, filepath.Join(t.TempDir(), "credentials.json"))
			keyring.MockForTesting()
			t.Cleanup(keyring.ResetForTesting)
			switch source {
			case "config":
				if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "bk.yaml"), []byte("quiet: true\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "environment":
				t.Setenv("BUILDKITE_QUIET", "true")
			}
			repo, err := git.PlainInit(".", false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{"https://github.com/acme/widgets.git"}}); err != nil {
				t.Fatal(err)
			}
			lookups, retries := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing credentials")
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/acme/pipelines":
					lookups++
					// Exceed a spinner tick so losing quiet mode is observable.
					time.Sleep(350 * time.Millisecond)
					fmt.Fprint(w, `[{"slug":"widgets","repository":"https://github.com/acme/widgets.git"}]`)
				case r.Method == http.MethodPut && r.URL.Path == "/v2/organizations/acme/pipelines/widgets/builds/42/retry_failed_jobs":
					retries++
					w.WriteHeader(http.StatusAccepted)
					fmt.Fprint(w, `{"retried_jobs_count":2}`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			var cmd RetryFailedJobsCmd
			var output bytes.Buffer
			parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(&output, stderr))
			ctx, err := parser.Parse([]string{"42", "--json"})
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Run(ctx, cli.Globals{Yes: true, NoInput: true}); err != nil {
				t.Fatal(err)
			}
			var receipt retryFailedJobsOutput
			if err := json.Unmarshal(output.Bytes(), &receipt); err != nil || receipt.Organization != "acme" || receipt.Pipeline != "widgets" || receipt.RetriedJobsCount != 2 || lookups != 1 || retries != 1 {
				t.Fatalf("incorrect discovery/retry receipt: %s; lookups=%d retries=%d err=%v", &output, lookups, retries, err)
			}
			progress, err := os.ReadFile(stderr.Name())
			if err != nil {
				t.Fatal(err)
			}
			if visible := strings.Contains(string(progress), "Resolving pipeline"); visible != (source == "progress control") {
				t.Fatalf("unexpected discovery progress for %s: %q", source, progress)
			}
		})
	}
}
