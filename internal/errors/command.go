package errors

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// CommandErrorHandler provides error handling functionality for cobra commands
type CommandErrorHandler struct {
	handler *Handler
}

// NewCommandErrorHandler creates a new CommandErrorHandler
func NewCommandErrorHandler() *CommandErrorHandler {
	return &CommandErrorHandler{
		handler: NewHandler(),
	}
}

// WithVerbose sets the verbose flag
func (c *CommandErrorHandler) WithVerbose(verbose bool) *CommandErrorHandler {
	c.handler.WithVerbose(verbose)
	return c
}

// HandleCommandError handles an error from a command
func (c *CommandErrorHandler) HandleCommandError(cmd *cobra.Command, err error) {
	if err == nil {
		return
	}

	// Override writer to use command's error output
	c.handler.WithWriter(cmd.ErrOrStderr())

	// Get the command path for context
	cmdPath := getCommandPath(cmd)

	// Handle the error with command path as context
	c.handler.HandleWithDetails(err, cmdPath)
}

// getCommandPath returns the full path of a command (e.g., "bk pipeline view")
func getCommandPath(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}

	names := []string{cmd.Name()}
	parent := cmd.Parent()

	for parent != nil {
		names = append([]string{parent.Name()}, names...)
		parent = parent.Parent()
	}

	return strings.Join(names, " ")
}

// ExecuteWithErrorHandling runs a cobra command with standardized error handling
func ExecuteWithErrorHandling(cmd *cobra.Command, verbose bool) int {
	// Silence Cobra's error printing
	cmd.SilenceErrors = true

	// Create an error handler
	errorHandler := NewCommandErrorHandler().WithVerbose(verbose)

	// Execute the command
	err := cmd.Execute()
	if err != nil {
		// Handle the error
		errorHandler.HandleCommandError(cmd, err)
		return GetExitCodeForError(err)
	}

	return ExitCodeSuccess
}

// WrapRunE wraps a RunE function with standard error handling
func WrapRunE(fn func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Call the original function
		err := fn(cmd, args)
		if err != nil {
			// For not found errors, add a suggestion to use --help
			if IsNotFound(err) {
				err = WithSuggestions(err, fmt.Sprintf("Try '%s --help' for more information", cmd.CommandPath()))
			}

			// For validation errors, add context about the command
			if IsValidationError(err) {
				err = WithDetails(err, fmt.Sprintf("When executing %s", cmd.CommandPath()))
			}
		}
		return err
	}
}
