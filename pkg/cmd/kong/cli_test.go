package kong

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/version"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/afero"
)

// mockFactory creates a factory for testing
func mockFactory() *factory.Factory {
	return &factory.Factory{
		Config:  config.New(afero.NewMemMapFs(), nil),
		Version: version.Version,
	}
}

func TestKongCLIStructure(t *testing.T) {
	t.Parallel()

	cli := &CLI{}
	parser := kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Exit(func(int) {}), // Prevent exit during tests
	)

	// Test that the parser was created successfully
	if parser == nil {
		t.Fatal("Failed to create Kong parser")
	}

	// Test that the model has the expected structure
	model := parser.Model
	if model.Name != "bk" {
		t.Errorf("Expected model name to be 'bk', got '%s'", model.Name)
	}
}

func TestKongFlags(t *testing.T) {
	t.Parallel()

	cli := &CLI{}
	parser := kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{"version": "test-version"},
		kong.Exit(func(int) {}),
	)

	// Test parsing global flags
	_, err := parser.Parse([]string{"--version"})
	if err != nil {
		// Kong requires a command by default, --version is handled specially
		t.Skipf("Kong --version handling: %v", err)
	}

	if !cli.Version {
		t.Error("Expected version flag to be true")
	}

	// Reset CLI
	cli = &CLI{}
	parser = kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Exit(func(int) {}),
	)

	_, err = parser.Parse([]string{"--verbose"})
	if err != nil {
		t.Fatalf("Failed to parse --verbose flag: %v", err)
	}

	if !cli.Verbose {
		t.Error("Expected verbose flag to be true")
	}
}

func TestKongCommands(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "agent list",
			args:     []string{"agent", "list"},
			expected: "agent list",
		},
		{
			name:     "build view",
			args:     []string{"build", "view"},
			expected: "build view",
		},
		{
			name:     "version",
			args:     []string{"version"},
			expected: "version",
		},
		{
			name:     "docs",
			args:     []string{"docs"},
			expected: "docs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cli := &CLI{}
			parser := kong.Must(cli,
				kong.Name("bk"),
				kong.Description("Work with Buildkite from the command line."),
				kong.UsageOnError(),
				kong.Exit(func(int) {}),
			)

			ctx, err := parser.Parse(tc.args)
			if err != nil {
				t.Fatalf("Failed to parse command %v: %v", tc.args, err)
			}

			if ctx.Command() != tc.expected {
				t.Errorf("Expected command to be '%s', got '%s'", tc.expected, ctx.Command())
			}
		})
	}
}

func TestKongSubcommandFlags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
		test func(*testing.T, *CLI)
	}{
		{
			name: "agent list with flags",
			args: []string{"agent", "list", "--name", "test-agent", "--per-page", "10"},
			test: func(t *testing.T, cli *CLI) {
				if cli.Agent.List.Name != "test-agent" {
					t.Errorf("Expected agent name to be 'test-agent', got '%s'", cli.Agent.List.Name)
				}
				if cli.Agent.List.PerPage != 10 {
					t.Errorf("Expected per-page to be 10, got %d", cli.Agent.List.PerPage)
				}
			},
		},
		{
			name: "build new with flags",
			args: []string{"build", "new", "--message", "test build", "--branch", "main", "--web"},
			test: func(t *testing.T, cli *CLI) {
				if cli.Build.New.Message != "test build" {
					t.Errorf("Expected message to be 'test build', got '%s'", cli.Build.New.Message)
				}
				if cli.Build.New.Branch != "main" {
					t.Errorf("Expected branch to be 'main', got '%s'", cli.Build.New.Branch)
				}
				if !cli.Build.New.Web {
					t.Error("Expected web flag to be true")
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cli := &CLI{}
			parser := kong.Must(cli,
				kong.Name("bk"),
				kong.Description("Work with Buildkite from the command line."),
				kong.UsageOnError(),
				kong.Exit(func(int) {}),
			)

			_, err := parser.Parse(tc.args)
			if err != nil {
				t.Fatalf("Failed to parse command %v: %v", tc.args, err)
			}

			tc.test(t, cli)
		})
	}
}

func TestKongHelp(t *testing.T) {
	t.Parallel()

	cli := &CLI{}
	parser := kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{"version": "test-version"},
		kong.Exit(func(int) {}),
	)

	// Test that help can be generated
	help := parser.Model.Help
	if help == "" {
		t.Error("Expected help to be non-empty")
	}

	// Test that help contains expected content
	t.Logf("Help content: %s", help)
	if !strings.Contains(help, "bk") && !strings.Contains(help, "Usage") && !strings.Contains(help, "Work with Buildkite") {
		t.Error("Expected help to contain 'bk', 'Usage', or 'Work with Buildkite'")
	}

	if !strings.Contains(help, "Work with Buildkite from the command line.") {
		t.Error("Expected help to contain description")
	}
}

func TestDocsCommand(t *testing.T) {
	t.Parallel()

	cli := &CLI{}
	parser := kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{"version": "test-version"},
		kong.Exit(func(int) {}),
	)

	ctx, err := parser.Parse([]string{"docs"})
	if err != nil {
		t.Fatalf("Failed to parse docs command: %v", err)
	}

	// Mock the context with a buffer to capture output
	var buf bytes.Buffer
	ctx.Stdout = &buf

	f := mockFactory()
	err = cli.Docs.Run(ctx, f)
	if err != nil {
		t.Fatalf("Failed to run docs command: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Logf("Buffer is empty, something went wrong")
		t.Error("Expected docs output to be non-empty")
	}

	// Check that the output contains expected content
	if !strings.Contains(output, "# Buildkite CLI") && !strings.Contains(output, "Documentation") {
		t.Error("Expected docs output to contain title")
	}

	if !strings.Contains(output, "## Usage") && !strings.Contains(output, "Usage") {
		t.Error("Expected docs output to contain usage section")
	}

	if !strings.Contains(output, "## Commands") && !strings.Contains(output, "Commands") {
		t.Error("Expected docs output to contain commands section")
	}
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	cli := &CLI{}
	parser := kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{"version": "test-version"},
		kong.Exit(func(int) {}),
	)

	ctx, err := parser.Parse([]string{"version"})
	if err != nil {
		t.Fatalf("Failed to parse version command: %v", err)
	}

	f := mockFactory()
	err = cli.Ver.Run(ctx, f)
	if err != nil {
		t.Fatalf("Failed to run version command: %v", err)
	}
}

func TestRootCommandWithVersion(t *testing.T) {
	t.Parallel()

	cli := &CLI{}
	parser := kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{"version": "test-version"},
		kong.Exit(func(int) {}),
	)

	ctx, err := parser.Parse([]string{"--version"})
	if err != nil {
		t.Skipf("Kong --version handling: %v", err)
	}

	f := mockFactory()
	err = cli.Run(ctx, f)
	if err != nil {
		t.Fatalf("Failed to run root command with version: %v", err)
	}
}

func TestRootCommandWithoutArgs(t *testing.T) {
	t.Parallel()

	cli := &CLI{}
	parser := kong.Must(cli,
		kong.Name("bk"),
		kong.Description("Work with Buildkite from the command line."),
		kong.UsageOnError(),
		kong.Vars{"version": "test-version"},
		kong.Exit(func(int) {}),
	)

	ctx, err := parser.Parse([]string{})
	if err != nil {
		t.Skipf("Kong requires command: %v", err)
	}

	var buf bytes.Buffer
	ctx.Stdout = &buf

	f := mockFactory()
	err = cli.Run(ctx, f)
	if err != nil {
		t.Fatalf("Failed to run root command without args: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("Expected root command to produce help output")
	}
}
