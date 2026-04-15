package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteTextOrStructured(t *testing.T) {
	t.Parallel()

	t.Run("writes text for text output", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		if err := WriteTextOrStructured(&buf, FormatText, []string{}, "No pipelines found."); err != nil {
			t.Fatalf("WriteTextOrStructured() error = %v", err)
		}

		if got := strings.TrimSpace(buf.String()); got != "No pipelines found." {
			t.Fatalf("WriteTextOrStructured() = %q, want %q", got, "No pipelines found.")
		}
	})

	t.Run("writes structured empty collections for json output", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		if err := WriteTextOrStructured(&buf, FormatJSON, []string{}, "ignored"); err != nil {
			t.Fatalf("WriteTextOrStructured() error = %v", err)
		}

		if got := strings.TrimSpace(buf.String()); got != "[]" {
			t.Fatalf("WriteTextOrStructured() = %q, want %q", got, "[]")
		}
	})

	t.Run("writes structured null values for json output", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		if err := WriteTextOrStructured(&buf, FormatJSON, nil, "ignored"); err != nil {
			t.Fatalf("WriteTextOrStructured() error = %v", err)
		}

		if got := strings.TrimSpace(buf.String()); got != "null" {
			t.Fatalf("WriteTextOrStructured() = %q, want %q", got, "null")
		}
	})
}
