package build

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/cli"
	"golang.org/x/sys/unix"
)

// A real terminal distinguishes --no-input from the non-TTY fallback and makes
// progress observable. No external terminal utilities or new dependencies.
func TestCompareTerminalSettings(t *testing.T) {
	for _, source := range []string{"config", "environment", "flags"} {
		for _, scenario := range []string{"no input", "quiet", "quiet after org override"} {
			t.Run(source+"/"+scenario, func(t *testing.T) {
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
				// If no-input regresses, allow the prompt to finish instead of hang.
				if _, err := master.WriteString("1\n"); err != nil {
					t.Fatal(err)
				}
				readErr, writeErr, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer readErr.Close()
				defer writeErr.Close()
				stdin, stdout, stderr := os.Stdin, os.Stdout, os.Stderr
				os.Stdin, os.Stdout, os.Stderr = terminal, terminal, writeErr
				defer func() { os.Stdin, os.Stdout, os.Stderr = stdin, stdout, stderr }()
				t.Setenv("BUILDKITE_NO_INPUT", "")
				t.Setenv("BUILDKITE_QUIET", "")
				f := comparisonFixture{globals: &cli.Globals{NoPager: true}}
				switch source {
				case "config":
					f.settings = true
				case "environment":
					t.Setenv("BUILDKITE_NO_INPUT", "true")
					t.Setenv("BUILDKITE_QUIET", "true")
				case "flags":
					f.globals.NoInput, f.globals.Quiet = true, true
				}
				args := []string{"acme/widgets/42", "--baseline", "39", "--json"}
				if scenario == "no input" {
					f.pipelines = []string{"widgets", "other"}
					args = []string{"42", "--text"}
				} else {
					// Exceed one spinner tick so an overwritten quiet flag is visible.
					f.delay = 250 * time.Millisecond
					if scenario == "quiet" {
						f.pipelines = []string{"widgets"}
					}
				}
				_, _, err = runComparison(t, &f, args...)
				if scenario == "no input" {
					if err == nil || !strings.Contains(err.Error(), "use --pipeline") || len(f.requests) != 0 {
						t.Fatalf("configured no-input allowed selection: err=%v requests=%v", err, f.requests)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				_ = writeErr.Close()
				progress, err := io.ReadAll(readErr)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(progress), "Comparing builds") {
					t.Fatalf("quiet setting lost: %q", progress)
				}
			})
		}
	}
}
