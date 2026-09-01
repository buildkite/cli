package secret

import (
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestResolveSecretValue(t *testing.T) {
	t.Parallel()

	t.Run("direct value", func(t *testing.T) {
		t.Parallel()

		got, err := resolveSecretValue("s3cr3t", "", strings.NewReader("ignored"))
		if err != nil {
			t.Fatalf("resolveSecretValue() error = %v", err)
		}
		if got != "s3cr3t" {
			t.Errorf("resolveSecretValue() = %q, want %q", got, "s3cr3t")
		}
	})

	t.Run("file preserves multiline content and trailing newline", func(t *testing.T) {
		t.Parallel()

		want := "line one\nline two\r\n"
		path := t.TempDir() + "/secret.txt"
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}

		got, err := resolveSecretValue("", path, strings.NewReader(""))
		if err != nil {
			t.Fatalf("resolveSecretValue() error = %v", err)
		}
		if got != want {
			t.Errorf("resolveSecretValue() = %q, want %q", got, want)
		}
	})

	t.Run("stdin preserves exact content", func(t *testing.T) {
		t.Parallel()

		want := "line one\nline two\r\n"
		got, err := resolveSecretValue("", "-", strings.NewReader(want))
		if err != nil {
			t.Fatalf("resolveSecretValue() error = %v", err)
		}
		if got != want {
			t.Errorf("resolveSecretValue() = %q, want %q", got, want)
		}
	})

	tests := []struct {
		name      string
		contents  []byte
		wantError string
	}{
		{name: "empty file", contents: nil, wantError: "cannot be empty"},
		{name: "invalid UTF-8", contents: []byte{0xff, 0xfe}, wantError: "not valid UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := t.TempDir() + "/secret.txt"
			if err := os.WriteFile(path, tt.contents, 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := resolveSecretValue("", path, strings.NewReader(""))
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("resolveSecretValue() error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}

	for _, tt := range tests {
		t.Run("stdin "+tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveSecretValue("", "-", strings.NewReader(string(tt.contents)))
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("resolveSecretValue() error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}

	t.Run("unreadable path", func(t *testing.T) {
		t.Parallel()

		path := t.TempDir() + "/missing"
		_, err := resolveSecretValue("", path, strings.NewReader(""))
		if err == nil || !strings.Contains(err.Error(), "read secret value from file") {
			t.Fatalf("resolveSecretValue() error = %v, want file read error", err)
		}
	})
}

func TestCreateCmdValueSourcesAreMutuallyExclusive(t *testing.T) {
	t.Parallel()

	var cmd CreateCmd
	parser, err := kong.New(&cmd, kong.Vars{"output_default_format": ""})
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	_, err = parser.Parse([]string{
		"--cluster-uuid", "cluster-123",
		"--key", "MY_SECRET",
		"--value", "s3cr3t",
		"--value-file", "secret.txt",
	})
	if err == nil {
		t.Fatal("Parse() expected an error for --value with --value-file, got nil")
	}
}
