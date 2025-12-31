package io

import (
	"os"
	"os/exec"
	"testing"
)

func TestPagerReturnsStdoutWhenNoPagerTrue(t *testing.T) {
	w, cleanup := Pager(true)
	defer cleanup()

	if w != os.Stdout {
		t.Errorf("expected os.Stdout when noPager=true, got %v", w)
	}
}

func TestPagerReturnsStdoutWhenNotTTY(t *testing.T) {
	w, cleanup := Pager(false)
	defer cleanup()

	if w != os.Stdout {
		t.Errorf("expected os.Stdout when not a TTY, got %v", w)
	}
}

func TestPagerReturnsStdoutWhenPagerNotFound(t *testing.T) {
	originalPager := os.Getenv("PAGER")
	defer os.Setenv("PAGER", originalPager)

	os.Setenv("PAGER", "nonexistent-pager-command-12345")

	w, cleanup := Pager(false)
	defer cleanup()

	if w != os.Stdout {
		t.Errorf("expected os.Stdout when pager not found, got %v", w)
	}
}

func TestPagerReturnsStdoutWhenPagerEnvMalformed(t *testing.T) {
	originalPager := os.Getenv("PAGER")
	defer os.Setenv("PAGER", originalPager)

	os.Setenv("PAGER", "less \"unclosed")

	w, cleanup := Pager(false)
	defer cleanup()

	if w != os.Stdout {
		t.Errorf("expected os.Stdout when PAGER env is malformed, got %v", w)
	}
}

func TestPagerReturnsStdoutWhenPagerEnvEmpty(t *testing.T) {
	originalPager := os.Getenv("PAGER")
	defer os.Setenv("PAGER", originalPager)

	os.Setenv("PAGER", "")

	w, cleanup := Pager(false)
	defer cleanup()

	if w != os.Stdout {
		t.Errorf("expected os.Stdout, got %v", w)
	}
}

func TestPagerCleanupIsIdempotent(t *testing.T) {
	originalPager := os.Getenv("PAGER")
	defer os.Setenv("PAGER", originalPager)

	os.Setenv("PAGER", "nonexistent-pager")

	_, cleanup := Pager(false)

	err1 := cleanup()
	err2 := cleanup()
	err3 := cleanup()

	if err1 != nil {
		t.Errorf("first cleanup returned error: %v", err1)
	}
	if err2 != nil {
		t.Errorf("second cleanup returned error: %v", err2)
	}
	if err3 != nil {
		t.Errorf("third cleanup returned error: %v", err3)
	}
}

func TestPagerWithCatCommand(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat command not found")
	}

	originalPager := os.Getenv("PAGER")
	defer os.Setenv("PAGER", originalPager)

	os.Setenv("PAGER", "cat")

	w, cleanup := Pager(false)
	defer cleanup()

	if w != os.Stdout {
		t.Errorf("expected os.Stdout in non-TTY, got %v", w)
	}
}

func TestIsLessPager(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Unix less",
			path:     "/usr/bin/less",
			expected: true,
		},
		{
			name:     "Windows less.exe",
			path:     "less.exe",
			expected: true,
		},
		{
			name:     "less in current dir",
			path:     "./less",
			expected: true,
		},
		{
			name:     "not less - cat",
			path:     "/usr/bin/cat",
			expected: false,
		},
		{
			name:     "not less - more",
			path:     "/usr/bin/more",
			expected: false,
		},
		{
			name:     "substring match should fail",
			path:     "/usr/bin/lessjs",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLessPager(tt.path)
			if result != tt.expected {
				t.Errorf("isLessPager(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestHasFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flags    []string
		expected bool
	}{
		{
			name:     "flag present",
			args:     []string{"-R", "-X"},
			flags:    []string{"-R"},
			expected: true,
		},
		{
			name:     "flag not present",
			args:     []string{"-X", "-F"},
			flags:    []string{"-R"},
			expected: false,
		},
		{
			name:     "multiple flags, one matches",
			args:     []string{"-R", "-X"},
			flags:    []string{"-R", "--RAW-CONTROL-CHARS"},
			expected: true,
		},
		{
			name:     "long flag matches",
			args:     []string{"--RAW-CONTROL-CHARS"},
			flags:    []string{"-R", "--RAW-CONTROL-CHARS"},
			expected: true,
		},
		{
			name:     "flag with value using equals",
			args:     []string{"--option=value"},
			flags:    []string{"--option"},
			expected: true,
		},
		{
			name:     "empty args",
			args:     []string{},
			flags:    []string{"-R"},
			expected: false,
		},
		{
			name:     "empty flags",
			args:     []string{"-R"},
			flags:    []string{},
			expected: false,
		},
		{
			name:     "substring should not match",
			args:     []string{"-RX"},
			flags:    []string{"-R"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasFlag(tt.args, tt.flags...)
			if result != tt.expected {
				t.Errorf("hasFlag(%v, %v) = %v, expected %v", tt.args, tt.flags, result, tt.expected)
			}
		})
	}
}

func TestPagerAddsRawFlagToLess(t *testing.T) {

	tests := []struct {
		name          string
		pagerPath     string
		initialArgs   []string
		shouldAddFlag bool
	}{
		{
			name:          "less without -R should add it",
			pagerPath:     "/usr/bin/less",
			initialArgs:   []string{"-X"},
			shouldAddFlag: true,
		},
		{
			name:          "less with -R should not add it",
			pagerPath:     "/usr/bin/less",
			initialArgs:   []string{"-R", "-X"},
			shouldAddFlag: false,
		},
		{
			name:          "less with --RAW-CONTROL-CHARS should not add -R",
			pagerPath:     "/usr/bin/less",
			initialArgs:   []string{"--RAW-CONTROL-CHARS"},
			shouldAddFlag: false,
		},
		{
			name:          "non-less pager should not add -R",
			pagerPath:     "/usr/bin/cat",
			initialArgs:   []string{},
			shouldAddFlag: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldAdd := isLessPager(tt.pagerPath) && !hasFlag(tt.initialArgs, "-R", "--RAW-CONTROL-CHARS")
			if shouldAdd != tt.shouldAddFlag {
				t.Errorf("expected shouldAdd=%v, got %v", tt.shouldAddFlag, shouldAdd)
			}
		})
	}
}

func TestPagerWriteAndCleanup(t *testing.T) {
	// Use cat as a simple pager that will work in tests
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat command not found")
	}

	originalPager := os.Getenv("PAGER")
	defer os.Setenv("PAGER", originalPager)

	os.Setenv("PAGER", "cat")

	w, cleanup := Pager(false)
	defer cleanup()

	if w == nil {
		t.Fatal("expected non-nil writer")
	}

	// Test that cleanup doesn't return an error
	if err := cleanup(); err != nil {
		t.Errorf("cleanup returned error: %v", err)
	}
}

func TestPagerCleanupAfterFailedStart(t *testing.T) {

	originalPager := os.Getenv("PAGER")
	defer os.Setenv("PAGER", originalPager)

	os.Setenv("PAGER", "false")

	w, cleanup := Pager(false)

	if w != os.Stdout {
		t.Errorf("expected os.Stdout, got %v", w)
	}

	if err := cleanup(); err != nil {
		t.Errorf("cleanup returned error: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Errorf("second cleanup returned error: %v", err)
	}
}
