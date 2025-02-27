package validation

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

func TestGetCommandPath(t *testing.T) {
	t.Parallel()

	t.Run("returns correct path for simple commands", func(t *testing.T) {
		t.Parallel()

		// Create a command hierarchy
		validateCmd := &cobra.Command{Use: "validate"}
		pipelineCmd := &cobra.Command{Use: "pipeline"}
		rootCmd := &cobra.Command{Use: "bk"}

		// Set up command hierarchy properly using AddCommand
		pipelineCmd.AddCommand(validateCmd)
		rootCmd.AddCommand(pipelineCmd)

		path := getCommandPath(validateCmd)
		expected := "bk pipeline validate"

		if path != expected {
			t.Errorf("Expected path %q, got %q", expected, path)
		}
	})

	t.Run("handles Use field with arguments", func(t *testing.T) {
		t.Parallel()

		// Create a command with Use field containing arguments
		cmd := &cobra.Command{Use: "validate [flags]"}
		parentCmd := &cobra.Command{Use: "pipeline <command>"}
		rootCmd := &cobra.Command{Use: "bk"}

		// Set up command hierarchy properly using AddCommand
		parentCmd.AddCommand(cmd)
		rootCmd.AddCommand(parentCmd)

		path := getCommandPath(cmd)
		expected := "bk pipeline validate"

		if path != expected {
			t.Errorf("Expected path %q, got %q", expected, path)
		}
	})
}

func TestCheckValidConfigurationExemptions(t *testing.T) {
	t.Parallel()

	t.Run("exempted commands skip validation", func(t *testing.T) {
		t.Parallel()

		// Create a config with empty token (would normally fail)
		conf := config.New(afero.NewMemMapFs(), nil)

		// Create a validate command
		validateCmd := &cobra.Command{
			Use:   "validate",
			Short: "validate command",
		}

		// Create a pipeline parent command
		pipelineCmd := &cobra.Command{
			Use:   "pipeline",
			Short: "pipeline command",
		}

		// Root command
		rootCmd := &cobra.Command{
			Use:   "bk",
			Short: "root command",
		}

		// Build command hierarchy properly
		pipelineCmd.AddCommand(validateCmd)
		rootCmd.AddCommand(pipelineCmd)

		// Check configuration for the pipeline validate command
		validator := CheckValidConfiguration(conf)
		err := validator(validateCmd, nil)
		// Should not error even with empty config
		if err != nil {
			t.Errorf("Expected no error for exempted command, got: %v", err)
		}
	})

	t.Run("command name path is correctly built", func(t *testing.T) {
		t.Parallel()

		// Create a config with empty token (would normally fail)
		conf := config.New(afero.NewMemMapFs(), nil)

		// Create a command structure with complex Use fields
		validateCmd := &cobra.Command{
			Use: "validate [flags]", // Contains extra content
		}

		pipelineCmd := &cobra.Command{
			Use: "pipeline <command>", // Contains extra content
		}

		rootCmd := &cobra.Command{
			Use: "bk [command]", // Contains extra content
		}

		// Build hierarchy properly
		pipelineCmd.AddCommand(validateCmd)
		rootCmd.AddCommand(pipelineCmd)

		// Check configuration for the pipeline validate command
		validator := CheckValidConfiguration(conf)
		err := validator(validateCmd, nil)
		// Should not error - should recognize "pipeline validate" pattern
		if err != nil {
			t.Errorf("Expected no error for command with pattern matching 'pipeline validate', got: %v", err)
		}
	})

	t.Run("non-exempted commands require validation", func(t *testing.T) {
		t.Parallel()

		// Create a config with empty token (should fail)
		conf := config.New(afero.NewMemMapFs(), nil)

		// Create a non-exempted command
		otherCmd := &cobra.Command{
			Use: "other",
		}

		// Check configuration for a non-exempt command
		validator := CheckValidConfiguration(conf)
		err := validator(otherCmd, nil)

		// Should error with empty config
		if err == nil {
			t.Error("Expected error for non-exempted command, got nil")
		}
	})
}
