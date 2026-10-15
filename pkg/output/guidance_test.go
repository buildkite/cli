package output_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/pkg/output"
)

func TestPlainGuidance(t *testing.T) {
	for _, interactive := range []bool{false, true} {
		guide := output.NewGuidance(&bytes.Buffer{}, interactive)
		heading := "Choose a queue"
		warning := "No connected agents observed."
		if interactive {
			heading = "\n" + heading
			warning = "Warning: " + warning
		}
		if got := guide.Heading("Choose a queue"); got != heading {
			t.Fatalf("plain heading = %q, want %q", got, heading)
		}
		if got := guide.Warning("No connected agents observed."); got != warning {
			t.Fatalf("plain warning = %q, want %q", got, warning)
		}
		if got := guide.Success("Selected queue: linux"); got != "Selected queue: linux" {
			t.Fatalf("plain completion contains styling: %q", got)
		}
	}
}

func TestRedirectedGuidance(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	guide := output.NewGuidance(file, true)
	if output.ColorEnabledFor(file) || strings.Contains(guide.Heading("Confirm destination"), "\x1b") {
		t.Fatal("redirected file enabled ANSI styling")
	}
}
