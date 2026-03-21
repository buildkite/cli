package preflight

import (
	"os"
	"strings"
	"testing"
)

func TestRun_Success(t *testing.T) {
	err := run(os.Environ(), "echo", "hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRun_Failure(t *testing.T) {
	err := run(os.Environ(), "false")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "false") {
		t.Errorf("error should contain command name, got %q", err)
	}
}

func TestRunQuiet_Success(t *testing.T) {
	err := runQuiet(os.Environ(), "echo", "hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunQuiet_Failure(t *testing.T) {
	err := runQuiet(os.Environ(), "sh", "-c", "echo 'something went wrong' >&2; exit 1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "sh") {
		t.Errorf("error should contain command name, got %q", err)
	}
}

func TestRunOut_Success(t *testing.T) {
	out, err := runOut(os.Environ(), "echo", "hello world")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", out)
	}
}

func TestRunOut_TrimsWhitespace(t *testing.T) {
	out, err := runOut(os.Environ(), "printf", "  padded  \n")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "padded" {
		t.Errorf("expected %q, got %q", "padded", out)
	}
}

func TestRunOut_Failure(t *testing.T) {
	out, err := runOut(os.Environ(), "sh", "-c", "echo 'partial output'; exit 1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if out != "" {
		t.Errorf("expected empty output on failure, got %q", out)
	}
	if !strings.Contains(err.Error(), "sh") {
		t.Errorf("error should contain command name, got %q", err)
	}
}
