package main

import (
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/afero"
)

func TestApplyExperiments(t *testing.T) {
	t.Run("preflight hidden when experiment disabled", func(t *testing.T) {
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
					t.Error("preflight should be hidden when experiment is disabled")
				}
				return
			}
		}
		t.Fatal("preflight command not found in parser")
	})

	t.Run("preflight visible when experiment enabled", func(t *testing.T) {
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
}
