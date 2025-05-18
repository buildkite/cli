package docs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// Template for llms.txt
const llmsTemplate = `# Buildkite CLI Documentation

{{ .RootDescription }}

## Commands
{{ range .Commands }}### {{ .CommandPath }}

{{ .ShortDesc }}
{{ if .LongDesc }}
{{ .LongDesc }}{{ end }}{{ if .Examples }}
Examples:
{{ .Examples }}{{ end }}
Usage:
{{ .Usage }}
{{ if .Flags }}
Flags:
{{ .Flags }}{{ end }}{{ if .HasSubCommands }}
Subcommands:
{{ range .SubCommands }}* {{ .CommandPath }} - {{ .ShortDesc }}
{{ end }}{{ end }}
{{ end }}
`

// CommandInfo holds information about a command for the template
type CommandInfo struct {
	CommandPath   string
	ShortDesc     string
	LongDesc      string
	Usage         string
	Examples      string
	Flags         string
	HasSubCommands bool
	SubCommands    []CommandInfo
}

// RootDocInfo holds information about the entire CLI for the template
type RootDocInfo struct {
	RootDescription string
	Commands       []CommandInfo
}

// generateLLMsDoc generates a single text file with all command documentation in llms.txt format
func generateLLMsDoc(rootCmd *cobra.Command, outputPath string) error {
	// Create a root doc info
	rootInfo := RootDocInfo{
		RootDescription: rootCmd.Long,
		Commands:       []CommandInfo{},
	}

	// First add the root command
	rootCommandInfos := []CommandInfo{}
	visitCommands(rootCmd, &rootCommandInfos, []string{})
	if len(rootCommandInfos) > 0 {
		rootInfo.Commands = append(rootInfo.Commands, rootCommandInfos[0])
	}

	// Then directly add all top-level commands to make sure they're included
	for _, cmd := range rootCmd.Commands() {
		if !cmd.Hidden && cmd.Name() != "help" {
			cmdInfos := []CommandInfo{}
			visitCommands(cmd, &cmdInfos, []string{rootCmd.Name()})
			if len(cmdInfos) > 0 {
				rootInfo.Commands = append(rootInfo.Commands, cmdInfos[0])
			}
		}
	}

	// Create the template
	tmpl, err := template.New("llms").Parse(llmsTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Render the template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, rootInfo); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Trim trailing whitespace and write to file
	content := strings.TrimRight(buf.String(), "\n\r\t ")
	// Ensure exactly one newline at the end
	content = content + "\n"

	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	return nil
}

// visitCommands recursively processes commands and their subcommands
func visitCommands(cmd *cobra.Command, cmdInfos *[]CommandInfo, path []string) {
	// Skip hidden commands and help commands
	if cmd.Hidden {
		return
	}

	// Create path
	cmdPath := cmd.CommandPath()

	// Extract flags (avoid duplicates)
	var flagsBuffer bytes.Buffer
	cmd.Flags().SortFlags = true
	cmd.Flags().SetOutput(&flagsBuffer)
	cmd.Flags().PrintDefaults()
	
	// Only include persistent flags for root command or if they add something new
	// This helps avoid duplicate flags like --verbose appearing twice
	if cmd.HasParent() {
		// For child commands, don't repeat global flags that are already shown
		// Could implement more advanced filtering here if needed
	}
	flagsString := strings.TrimSpace(flagsBuffer.String())

	// Collect subcommands
	var availableSubCommands []CommandInfo
	for _, subCmd := range cmd.Commands() {
		// Skip hidden and help commands
		if subCmd.Hidden || subCmd.Name() == "help" {
			continue
		}
		
		if subCmd.IsAvailableCommand() {
			// Create a new slice for this subcommand
			subCommandInfos := []CommandInfo{}
			// Process this subcommand
			visitCommands(subCmd, &subCommandInfos, append(path, cmd.Name()))
			// If we got any results (should be at least the command itself), add them
			if len(subCommandInfos) > 0 {
				availableSubCommands = append(availableSubCommands, subCommandInfos[0])
			}
		}
	}

	// Create command info
	cmdInfo := CommandInfo{
		CommandPath:    cmdPath,
		ShortDesc:      cmd.Short,
		LongDesc:       cmd.Long,
		Usage:          cmd.UseLine(),
		Examples:       cmd.Example,
		Flags:          flagsString,
		HasSubCommands: len(availableSubCommands) > 0,
		SubCommands:    availableSubCommands,
	}

	// Add to parent
	*cmdInfos = append(*cmdInfos, cmdInfo)
}

// NewCmdDocs creates a new docs command
func NewCmdDocs(f *factory.Factory) *cobra.Command {
	var outputDir string
	var format string

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Generate documentation for the CLI",
		Long: heredoc.Doc(`
			Generate documentation for the Buildkite CLI in different formats.
			
			This command can generate documentation in the following formats:
			- Markdown: Complete documentation tree for all commands
			- llms.txt: Single text file optimized for AI language models
		`),
		Example: heredoc.Doc(`
			# Generate markdown documentation
			$ bk docs --output ./docs
			
			# Generate llms.txt format
			$ bk docs --format llms --output ./llms.txt
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get the root command to generate docs for the entire CLI
			rootCmd := cmd.Root()

			switch format {
			case "markdown":
				// Create output directory if it doesn't exist
				if err := os.MkdirAll(outputDir, 0755); err != nil {
					return fmt.Errorf("failed to create output directory: %w", err)
				}

				// Generate markdown documentation
				filePrepender := func(filename string) string {
					return fmt.Sprintf("---\ntitle: %s\n---\n\n", filepath.Base(filename))
				}

				linkHandler := func(name string) string {
					return fmt.Sprintf("%s.md", name)
				}

				fmt.Printf("Generating markdown documentation in %s\n", outputDir)
				err := doc.GenMarkdownTreeCustom(rootCmd, outputDir, filePrepender, linkHandler)
				if err != nil {
					return fmt.Errorf("failed to generate markdown documentation: %w", err)
				}

			case "llms":
				// Generate a single llms.txt file
				// Create parent directory if needed
				dir := filepath.Dir(outputDir)
				if dir != "." {
					if err := os.MkdirAll(dir, 0755); err != nil {
						return fmt.Errorf("failed to create directory: %w", err)
					}
				}

				fmt.Printf("Generating llms.txt format documentation at %s\n", outputDir)
				if err := generateLLMsDoc(rootCmd, outputDir); err != nil {
					return fmt.Errorf("failed to generate llms.txt documentation: %w", err)
				}

			default:
				return fmt.Errorf("unsupported format: %s. Supported formats are 'markdown' and 'llms'", format)
			}

			fmt.Println("Documentation generated successfully")
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "./docs", "Output directory or file path for documentation")
	cmd.Flags().StringVarP(&format, "format", "f", "markdown", "Documentation format: markdown or llms")

	return cmd
}