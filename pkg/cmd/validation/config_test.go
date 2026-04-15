package validation

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	bkKeyring "github.com/buildkite/cli/v3/pkg/keyring"
)

func TestValidateConfiguration_ExemptCommands(t *testing.T) {
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
	conf := newTestConfig(t)

	for _, path := range []string{
		"pipeline validate",
		"pipeline migrate",
		"configure",
		"configure default",
		"configure add",
	} {
		if err := ValidateConfiguration(conf, path); err != nil {
			t.Fatalf("expected no error for exempt command %q, got %v", path, err)
		}
	}
}

func TestValidateConfiguration_MissingValues(t *testing.T) {
	t.Run("missing token and org", func(t *testing.T) {
		t.Setenv("BUILDKITE_API_TOKEN", "")
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
		conf := newTestConfig(t)
		if err := ValidateConfiguration(conf, "pipeline view"); err == nil {
			t.Fatalf("expected error when token and org are missing")
		}
	})

	t.Run("missing token", func(t *testing.T) {
		t.Setenv("BUILDKITE_API_TOKEN", "")
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "org")
		conf := newTestConfig(t)
		if err := ValidateConfiguration(conf, "pipeline view"); err == nil {
			t.Fatalf("expected error when token is missing")
		}
	})

	t.Run("token and org present", func(t *testing.T) {
		t.Setenv("BUILDKITE_API_TOKEN", "token2")
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "org2")
		conf := newTestConfig(t)
		if err := ValidateConfiguration(conf, "pipeline view"); err != nil {
			t.Fatalf("expected no error when token and org are set, got %v", err)
		}
	})

	t.Run("missing org warning is written to stderr", func(t *testing.T) {
		t.Setenv("BUILDKITE_API_TOKEN", "token")
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
		conf := newTestConfig(t)

		stdout, stderr := captureStandardStreams(t, func() {
			if err := ValidateConfiguration(conf, "pipeline view"); err != nil {
				t.Fatalf("expected no error when only org is missing, got %v", err)
			}
		})

		if stdout != "" {
			t.Fatalf("expected stdout to remain empty, got %q", stdout)
		}

		if !strings.Contains(stderr, "Warning: no organization set") {
			t.Fatalf("expected stderr warning, got %q", stderr)
		}
	})
}

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	bkKeyring.MockForTesting()
	return config.New(nil, nil)
}

func captureStandardStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() stdout error = %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() stderr error = %v", err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	fn()

	if err := stdoutW.Close(); err != nil {
		t.Fatalf("stdout close error = %v", err)
	}
	if err := stderrW.Close(); err != nil {
		t.Fatalf("stderr close error = %v", err)
	}

	stdoutBytes, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("stdout read error = %v", err)
	}
	stderrBytes, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("stderr read error = %v", err)
	}

	if err := stdoutR.Close(); err != nil {
		t.Fatalf("stdout reader close error = %v", err)
	}
	if err := stderrR.Close(); err != nil {
		t.Fatalf("stderr reader close error = %v", err)
	}

	return string(stdoutBytes), string(stderrBytes)
}
