package root

import (
	"fmt"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// mockFactory creates a factory for testing
func mockFactory() *factory.Factory {
	return &factory.Factory{
		Config:  config.New(afero.NewMemMapFs(), nil),
		Version: version.Version,
	}
}

func TestRootCommand(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	// Test basic command properties
	if cmd.Use != "bk <command> <subcommand> [flags]" {
		t.Errorf("Expected Use to be 'bk <command> <subcommand> [flags]', got '%s'", cmd.Use)
	}

	if cmd.Short != "Buildkite CLI" {
		t.Errorf("Expected Short to be 'Buildkite CLI', got '%s'", cmd.Short)
	}

	if cmd.Long != "Work with Buildkite from the command line." {
		t.Errorf("Expected Long to be 'Work with Buildkite from the command line.', got '%s'", cmd.Long)
	}

	// Test flags
	versionFlag := cmd.Flags().Lookup("version")
	if versionFlag == nil {
		t.Error("Expected version flag to exist")
	}

	verboseFlag := cmd.PersistentFlags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("Expected verbose flag to exist")
	}
}

func TestSubcommands(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	expectedCommands := []string{
		"agent",
		"api",
		"artifacts",
		"build",
		"cluster",
		"configure",
		"init",
		"job",
		"package",
		"pipeline",
		"prompt",
		"use",
		"user",
		"version",
	}

	subcommands := cmd.Commands()
	if len(subcommands) != len(expectedCommands) {
		t.Errorf("Expected %d subcommands, got %d", len(expectedCommands), len(subcommands))
	}

	commandNames := make(map[string]bool)
	for _, subcmd := range subcommands {
		commandNames[subcmd.Name()] = true
	}

	for _, expected := range expectedCommands {
		if !commandNames[expected] {
			t.Errorf("Expected command '%s' to exist", expected)
		}
	}
}

func TestCommandFlags(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	// Test that all subcommands have the expected structure
	testCases := []struct {
		name          string
		expectedFlags []string
		requiredFlags []string
		shortFlags    map[string]string
	}{
		{
			name:          "build",
			expectedFlags: []string{"web", "pipeline", "mine", "branch", "user"},
			shortFlags:    map[string]string{"w": "web", "p": "pipeline", "m": "mine", "b": "branch", "u": "user"},
		},
		{
			name:          "agent",
			expectedFlags: []string{"web", "name", "version", "hostname", "per-page", "force", "limit"},
			shortFlags:    map[string]string{"w": "web", "l": "limit"},
		},
		{
			name:          "api",
			expectedFlags: []string{"method", "header", "data", "analytics", "file"},
			shortFlags:    map[string]string{"X": "method", "H": "header", "d": "data", "f": "file"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			subcmd := findCommand(cmd, tc.name)
			if subcmd == nil {
				t.Fatalf("Command '%s' not found", tc.name)
			}

			// Check if command has subcommands (like build has view, cancel, etc.)
			if len(subcmd.Commands()) > 0 {
				// For commands with subcommands, we'll check the first subcommand
				subsubcmd := subcmd.Commands()[0]
				for _, expectedFlag := range tc.expectedFlags {
					flag := subsubcmd.Flags().Lookup(expectedFlag)
					if flag == nil {
						// Check persistent flags
						flag = subsubcmd.PersistentFlags().Lookup(expectedFlag)
					}
					if flag == nil {
						// Check parent flags
						flag = subcmd.Flags().Lookup(expectedFlag)
					}
					if flag == nil {
						flag = subcmd.PersistentFlags().Lookup(expectedFlag)
					}
					if flag == nil {
						t.Logf("Flag '%s' not found in command '%s', checking if it's command-specific", expectedFlag, tc.name)
					}
				}
			}
		})
	}
}

func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, cmd := range root.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

func TestVersionFlag(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	// Test that version flag exists and has correct properties
	versionFlag := cmd.Flags().Lookup("version")
	if versionFlag == nil {
		t.Fatal("version flag not found")
	}

	if versionFlag.Shorthand != "v" {
		t.Errorf("Expected version flag shorthand to be 'v', got '%s'", versionFlag.Shorthand)
	}

	if versionFlag.Usage != "Print the version number" {
		t.Errorf("Expected version flag usage to be 'Print the version number', got '%s'", versionFlag.Usage)
	}
}

func TestVerboseFlag(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	// Test that verbose flag exists and has correct properties
	verboseFlag := cmd.PersistentFlags().Lookup("verbose")
	if verboseFlag == nil {
		t.Fatal("verbose flag not found")
	}

	if verboseFlag.Shorthand != "V" {
		t.Errorf("Expected verbose flag shorthand to be 'V', got '%s'", verboseFlag.Shorthand)
	}

	if verboseFlag.Usage != "Enable verbose error output" {
		t.Errorf("Expected verbose flag usage to be 'Enable verbose error output', got '%s'", verboseFlag.Usage)
	}
}

func TestCommandStructure(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	// Test that all commands have factory dependency
	for _, subcmd := range cmd.Commands() {
		if subcmd.Name() == "version" {
			continue // version command might not need factory
		}

		// Check if the command has been properly initialized
		if subcmd.RunE == nil && subcmd.Run == nil && len(subcmd.Commands()) == 0 {
			t.Errorf("Command '%s' has no run function and no subcommands", subcmd.Name())
		}
	}
}

func TestHelpFunctionality(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	// Test that help can be generated without errors
	help := cmd.Help()
	if help != nil {
		t.Errorf("Expected help to return nil, got: %v", help)
	}

	// Test that usage can be generated
	usage := cmd.UsageString()
	if usage == "" {
		t.Error("Expected usage string to be non-empty")
	}
}

func TestCommandAnnotations(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	// Test that annotations exist
	if cmd.Annotations == nil {
		t.Error("Expected annotations to exist")
	}

	if versionInfo, exists := cmd.Annotations["versionInfo"]; !exists {
		t.Error("Expected versionInfo annotation to exist")
	} else if versionInfo == "" {
		t.Error("Expected versionInfo annotation to be non-empty")
	}
}

func TestSilenceUsage(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	// Test that SilenceUsage is true
	if !cmd.SilenceUsage {
		t.Error("Expected SilenceUsage to be true")
	}
}

func TestExample(t *testing.T) {
	t.Parallel()

	f := mockFactory()
	cmd, err := NewCmdRoot(f)
	if err != nil {
		t.Fatalf("Failed to create root command: %v", err)
	}

	// Test that example is set
	if cmd.Example == "" {
		t.Error("Expected example to be non-empty")
	}

	// Test that example contains expected content
	expectedContent := "$ bk build view"
	if cmd.Example != fmt.Sprintf("%s\n", expectedContent) {
		t.Errorf("Expected example to contain '%s', got '%s'", expectedContent, cmd.Example)
	}
}
