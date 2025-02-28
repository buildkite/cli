package errors

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Exit codes for different error types
const (
	ExitCodeSuccess          = 0
	ExitCodeGenericError     = 1
	ExitCodeValidationError  = 2
	ExitCodeAPIError         = 3
	ExitCodeNotFoundError    = 4
	ExitCodePermissionError  = 5
	ExitCodeConfigError      = 6
	ExitCodeAuthError        = 7
	ExitCodeInternalError    = 8
	ExitCodeUserAbortedError = 130 // Same as Ctrl+C in bash
)

// Styling for error output
var (
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true) // Red
	warningStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // Yellow
	suggestionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))            // Cyan
	detailStyle     = lipgloss.NewStyle().Faint(true)                                // Faint
)

// Handler processes errors from commands and formats them appropriately
type Handler struct {
	// Writer is where error messages will be written
	Writer io.Writer
	// ExitFunc is the function called to exit the program with a specific code
	ExitFunc func(int)
	// Verbose enables more detailed error messages
	Verbose bool
}

// NewHandler creates a new Handler with default settings
func NewHandler() *Handler {
	return &Handler{
		Writer:   os.Stderr,
		ExitFunc: os.Exit,
		Verbose:  false,
	}
}

// WithWriter sets the writer for error output
func (h *Handler) WithWriter(w io.Writer) *Handler {
	h.Writer = w
	return h
}

// WithExitFunc sets the exit function
func (h *Handler) WithExitFunc(f func(int)) *Handler {
	h.ExitFunc = f
	return h
}

// WithVerbose sets the verbose flag
func (h *Handler) WithVerbose(v bool) *Handler {
	h.Verbose = v
	return h
}

// Handle processes an error and outputs it appropriately
func (h *Handler) Handle(err error) {
	if err == nil {
		return
	}

	// Get the exit code based on error type
	exitCode := h.getExitCode(err)

	// Format the error message
	message := h.formatError(err)

	// Write the error message
	fmt.Fprintln(h.Writer, message)

	// Call the exit function with the appropriate code
	if h.ExitFunc != nil {
		h.ExitFunc(exitCode)
	}
}

// getExitCode determines the appropriate exit code based on the error type
func (h *Handler) getExitCode(err error) int {
	switch {
	case IsValidationError(err):
		return ExitCodeValidationError
	case IsAPIError(err):
		return ExitCodeAPIError
	case IsNotFound(err):
		return ExitCodeNotFoundError
	case IsPermissionDeniedError(err):
		return ExitCodePermissionError
	case IsConfigurationError(err):
		return ExitCodeConfigError
	case IsAuthenticationError(err):
		return ExitCodeAuthError
	case IsUserAborted(err):
		return ExitCodeUserAbortedError
	case errors.Is(err, ErrInternal):
		return ExitCodeInternalError
	default:
		return ExitCodeGenericError
	}
}

// formatError creates a formatted error message based on the error type
func (h *Handler) formatError(err error) string {
	prefix := errorStyle.Render("Error:")

	if cliErr, ok := err.(*Error); ok {
		// For CLI errors, use the formatted error message
		var message string

		if cliErr.Category != nil {
			// Get a more specific prefix based on the error category
			prefix = h.getCategoryPrefix(cliErr.Category)
		}

		// If verbose mode is enabled, include more details
		if h.Verbose {
			message = cliErr.FormattedError()
		} else {
			// In non-verbose mode, include the main error message and the first suggestion
			message = cliErr.Error()
			if len(cliErr.Suggestions) > 0 {
				message = fmt.Sprintf("%s\n%s", message,
					suggestionStyle.Render(fmt.Sprintf("Tip: %s", cliErr.Suggestions[0])))
			}
		}

		return fmt.Sprintf("%s %s", prefix, message)
	}

	// For regular errors, just return the error message
	return fmt.Sprintf("%s %s", prefix, err.Error())
}

// getCategoryPrefix returns an appropriate prefix for the error category
func (h *Handler) getCategoryPrefix(category error) string {
	switch category {
	case ErrValidation:
		return errorStyle.Render("Validation Error:")
	case ErrAPI:
		return errorStyle.Render("API Error:")
	case ErrResourceNotFound:
		return errorStyle.Render("Not Found:")
	case ErrPermissionDenied:
		return errorStyle.Render("Permission Denied:")
	case ErrConfiguration:
		return errorStyle.Render("Configuration Error:")
	case ErrAuthentication:
		return errorStyle.Render("Authentication Error:")
	case ErrUserAborted:
		return warningStyle.Render("Aborted:")
	case ErrInternal:
		return errorStyle.Render("Internal Error:")
	default:
		return errorStyle.Render("Error:")
	}
}

// HandleWithDetails processes an error with additional contextual details
func (h *Handler) HandleWithDetails(err error, operation string) {
	if err == nil {
		return
	}

	// Add operation context to the error
	var contextualErr error
	if operation != "" {
		// Check if it's already a CLI error
		if cliErr, ok := err.(*Error); ok {
			// Create a copy to avoid modifying the original error
			newCliErr := &Error{
				Original:    cliErr.Original,
				Category:    cliErr.Category,
				Suggestions: append([]string{}, cliErr.Suggestions...),
			}
			
			// Add operation to details
			if cliErr.Details == "" {
				newCliErr.Details = fmt.Sprintf("failed during: %s", operation)
			} else {
				newCliErr.Details = fmt.Sprintf("%s (during: %s)", cliErr.Details, operation)
			}
			contextualErr = newCliErr
		} else {
			// Wrap in a new error with operation details
			contextualErr = NewError(err, nil, fmt.Sprintf("failed during: %s", operation))
		}
	} else {
		contextualErr = err
	}

	// Handle the contextual error
	h.Handle(contextualErr)
}

// PrintWarning prints a warning message
func (h *Handler) PrintWarning(format string, args ...interface{}) {
	prefix := warningStyle.Render("Warning:")
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(h.Writer, "%s %s\n", prefix, message)
}

// MessageForError returns a formatted message for an error without exiting
func MessageForError(err error) string {
	if err == nil {
		return ""
	}

	handler := NewHandler()
	return handler.formatError(err)
}

// GetExitCodeForError returns the exit code for a given error
func GetExitCodeForError(err error) int {
	if err == nil {
		return ExitCodeSuccess
	}

	handler := NewHandler()
	return handler.getExitCode(err)
}
