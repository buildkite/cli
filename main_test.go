package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/afero"
)

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	original, had := os.LookupEnv(key)
	if had {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("failed to unset env %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		var err error
		if had {
			err = os.Setenv(key, original)
		} else {
			err = os.Unsetenv(key)
		}
		if err != nil {
			t.Fatalf("failed to restore env %s: %v", key, err)
		}
	})
}

func TestApplyExperiments(t *testing.T) {
	t.Run("preflight visible by default", func(t *testing.T) {
		unsetEnv(t, "BUILDKITE_EXPERIMENTS")
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		applyExperiments(parser, conf)

		for _, node := range parser.Model.Children {
			if node.Name == "preflight" {
				if node.Hidden {
					t.Error("preflight should be visible by default")
				}
				return
			}
		}
		t.Fatal("preflight command not found in parser")
	})

	t.Run("preflight hidden when experiment disabled", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "alpha")
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		applyExperiments(parser, conf)

		for _, node := range parser.Model.Children {
			if node.Name == "preflight" {
				if !node.Hidden {
					t.Error("preflight should be hidden when experiment is disabled")
				}
				return
			}
		}
		t.Fatal("preflight command not found in parser")
	})

	t.Run("preflight hidden when experiments override is empty", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "")
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		applyExperiments(parser, conf)

		for _, node := range parser.Model.Children {
			if node.Name == "preflight" {
				if !node.Hidden {
					t.Error("preflight should be hidden when experiments override is empty")
				}
				return
			}
		}
		t.Fatal("preflight command not found in parser")
	})

	t.Run("preflight visible when experiment enabled explicitly", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		applyExperiments(parser, conf)

		for _, node := range parser.Model.Children {
			if node.Name == "preflight" {
				if node.Hidden {
					t.Error("preflight should be visible when experiment is enabled")
				}
				return
			}
		}
		t.Fatal("preflight command not found in parser")
	})

	t.Run("preflight root still parses with default subcommand", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight"}); err != nil {
			t.Fatalf("failed to parse preflight root command: %v", err)
		}
	})

	t.Run("preflight await-test-results parses without a value", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--await-test-results"}); err != nil {
			t.Fatalf("failed to parse preflight await-test-results flag: %v", err)
		}
		if !cli.Preflight.Run.AwaitTestResults.Enabled {
			t.Fatal("expected await-test-results to be enabled")
		}
		if cli.Preflight.Run.AwaitTestResults.Duration != 30*time.Second {
			t.Fatalf("expected default await-test-results duration, got %s", cli.Preflight.Run.AwaitTestResults.Duration)
		}
	})

	t.Run("preflight await-test-results parses with an explicit duration", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--await-test-results=45s"}); err != nil {
			t.Fatalf("failed to parse preflight await-test-results duration: %v", err)
		}
		if !cli.Preflight.Run.AwaitTestResults.Enabled {
			t.Fatal("expected await-test-results to be enabled")
		}
		if cli.Preflight.Run.AwaitTestResults.Duration != 45*time.Second {
			t.Fatalf("expected explicit await-test-results duration, got %s", cli.Preflight.Run.AwaitTestResults.Duration)
		}
	})

	t.Run("preflight exit-on parses repeated flags", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--exit-on=build-failing", "--exit-on=build-failing"}); err != nil {
			t.Fatalf("failed to parse repeated preflight exit-on flags: %v", err)
		}
		if len(cli.Preflight.Run.ExitOn) != 2 {
			t.Fatalf("expected 2 exit-on values, got %d", len(cli.Preflight.Run.ExitOn))
		}
	})

	t.Run("preflight exit-on rejects unknown values", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--exit-on=test-failed:3"}); err == nil {
			t.Fatal("expected parse error for invalid exit-on value")
		}
	})

	t.Run("preflight exit-on rejects incompatible combinations", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--exit-on=build-failing", "--exit-on=build-terminal"}); err == nil {
			t.Fatal("expected parse error for incompatible exit-on values")
		}
	})

	t.Run("preflight run subcommand still parses", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "run", "--await-test-results=45s"}); err != nil {
			t.Fatalf("failed to parse preflight run subcommand: %v", err)
		}
		if !cli.Preflight.Run.AwaitTestResults.Enabled {
			t.Fatal("expected run subcommand await-test-results to be enabled")
		}
		if cli.Preflight.Run.AwaitTestResults.Duration != 45*time.Second {
			t.Fatalf("expected explicit run subcommand await-test-results duration, got %s", cli.Preflight.Run.AwaitTestResults.Duration)
		}
	})

	t.Run("preflight help includes mirrored run flags", func(t *testing.T) {
		help, err := renderPreflightHelp()
		if err != nil {
			t.Fatalf("failed to render preflight help: %v", err)
		}
		for _, want := range []string{
			"--[no-]watch",
			"--exit-on=EXIT-ON,...",
			"--await-test-results",
			"--no-cleanup",
			"preflight cleanup [flags]",
		} {
			if !strings.Contains(help, want) {
				t.Fatalf("expected preflight help to contain %q, got:\n%s", want, help)
			}
		}
	})

	t.Run("preflight help requests are detected", func(t *testing.T) {
		tests := []struct {
			args []string
			want bool
		}{
			{args: []string{"preflight", "--help"}, want: true},
			{args: []string{"preflight", "-h"}, want: true},
			{args: []string{"help", "preflight"}, want: true},
			{args: []string{"preflight", "run", "--help"}, want: false},
		}

		for _, tt := range tests {
			if got := isPreflightHelpRequest(tt.args); got != tt.want {
				t.Fatalf("isPreflightHelpRequest(%q) = %v, want %v", tt.args, got, tt.want)
			}
		}
	})
}
