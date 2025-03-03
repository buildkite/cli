package errors

import (
	"errors"
	"fmt"
	"strings"
)

// Standard error types that can be used to categorize errors
var (
	// ErrConfiguration indicates an error in the user's configuration
	ErrConfiguration = errors.New("configuration error")

	// ErrValidation indicates invalid input from the user
	ErrValidation = errors.New("validation error")

	// ErrAPI indicates an error from the Buildkite API
	ErrAPI = errors.New("API error")

	// ErrResourceNotFound indicates a requested resource was not found
	ErrResourceNotFound = errors.New("resource not found")

	// ErrPermissionDenied indicates the user lacks permission
	ErrPermissionDenied = errors.New("permission denied")

	// ErrAuthentication indicates an issue with authentication
	ErrAuthentication = errors.New("authentication error")

	// ErrInternal indicates an internal error in the CLI
	ErrInternal = errors.New("internal error")

	// ErrUserAborted indicates the user has canceled an operation
	ErrUserAborted = errors.New("user aborted")
)

// Error represents a CLI error with context
type Error struct {
	// Original is the underlying error
	Original error

	// Category is the broad category of the error
	Category error

	// Details contains additional detail about the error
	Details string

	// Suggestions provides hints on how to fix the error
	Suggestions []string
}

// Error implements the error interface
func (e *Error) Error() string {
	var msg strings.Builder

	if e.Category != nil {
		msg.WriteString(e.Category.Error())
		msg.WriteString(": ")
	}

	// First include the original error, if present
	if e.Original != nil {
		msg.WriteString(e.Original.Error())
	}

	// Then include details if present, regardless of whether Original is present
	if e.Details != "" {
		// Only add a separator if we've already written something
		if e.Original != nil {
			msg.WriteString(" (")
			msg.WriteString(e.Details)
			msg.WriteString(")")
		} else {
			msg.WriteString(e.Details)
		}
	}

	return msg.String()
}

// FormattedError returns a formatted multi-line error message suitable for display
func (e *Error) FormattedError() string {
	var msg strings.Builder

	// Build the main error message
	if e.Category != nil {
		// Write category with uppercase first letter
		category := e.Category.Error()
		if len(category) > 0 {
			msg.WriteString(strings.ToUpper(category[:1]) + category[1:])
			msg.WriteString(": ")
		}
	}

	if e.Original != nil {
		msg.WriteString(e.Original.Error())
	} else if e.Details != "" {
		msg.WriteString(e.Details)
	}

	// Add detailed suggestions if available
	if len(e.Suggestions) > 0 {
		msg.WriteString("\n\n")
		for i, suggestion := range e.Suggestions {
			if i > 0 {
				msg.WriteString("\n")
			}
			msg.WriteString("• ")
			msg.WriteString(suggestion)
		}
	}

	return msg.String()
}

// Unwrap implements the errors.Unwrap interface to allow using errors.Is and errors.As
func (e *Error) Unwrap() error {
	if e.Original != nil {
		return e.Original
	}
	return e.Category
}

// Is implements the errors.Is interface to allow checking error types
func (e *Error) Is(target error) bool {
	return errors.Is(e.Category, target) || (e.Original != nil && errors.Is(e.Original, target))
}

// NewError creates a new Error with the given attributes
func NewError(original error, category error, details string, suggestions ...string) *Error {
	return &Error{
		Original:    original,
		Category:    category,
		Details:     details,
		Suggestions: suggestions,
	}
}

// WithSuggestions adds suggestions to an existing error
func WithSuggestions(err error, suggestions ...string) error {
	if cliErr, ok := err.(*Error); ok {
		cliErr.Suggestions = append(cliErr.Suggestions, suggestions...)
		return cliErr
	}

	// If it's not already a CLI error, create a new one
	return NewError(err, nil, "", suggestions...)
}

// WithDetails adds details to an existing error
func WithDetails(err error, details string) error {
	if cliErr, ok := err.(*Error); ok {
		if cliErr.Details == "" {
			cliErr.Details = details
		} else {
			cliErr.Details = fmt.Sprintf("%s: %s", cliErr.Details, details)
		}
		return cliErr
	}

	// If it's not already a CLI error, create a new one
	return NewError(err, nil, details)
}

// NewConfigurationError creates a new configuration error
func NewConfigurationError(err error, details string, suggestions ...string) error {
	return NewError(err, ErrConfiguration, details, suggestions...)
}

// NewValidationError creates a new validation error
func NewValidationError(err error, details string, suggestions ...string) error {
	return NewError(err, ErrValidation, details, suggestions...)
}

// NewAPIError creates a new API error
func NewAPIError(err error, details string, suggestions ...string) error {
	return NewError(err, ErrAPI, details, suggestions...)
}

// NewResourceNotFoundError creates a new resource not found error
func NewResourceNotFoundError(err error, details string, suggestions ...string) error {
	return NewError(err, ErrResourceNotFound, details, suggestions...)
}

// NewPermissionDeniedError creates a new permission denied error
func NewPermissionDeniedError(err error, details string, suggestions ...string) error {
	return NewError(err, ErrPermissionDenied, details, suggestions...)
}

// NewAuthenticationError creates a new authentication error
func NewAuthenticationError(err error, details string, suggestions ...string) error {
	return NewError(err, ErrAuthentication, details, suggestions...)
}

// NewInternalError creates a new internal error
func NewInternalError(err error, details string, suggestions ...string) error {
	return NewError(err, ErrInternal, details, suggestions...)
}

// NewUserAbortedError creates a new user aborted error
func NewUserAbortedError(err error, details string, suggestions ...string) error {
	return NewError(err, ErrUserAborted, details, suggestions...)
}

// IsNotFound returns true if the error indicates a resource was not found
func IsNotFound(err error) bool {
	return errors.Is(err, ErrResourceNotFound)
}

// IsValidationError returns true if the error indicates a validation failure
func IsValidationError(err error) bool {
	return errors.Is(err, ErrValidation)
}

// IsAPIError returns true if the error indicates an API failure
func IsAPIError(err error) bool {
	return errors.Is(err, ErrAPI)
}

// IsAuthenticationError returns true if the error indicates an authentication failure
func IsAuthenticationError(err error) bool {
	return errors.Is(err, ErrAuthentication)
}

// IsPermissionDeniedError returns true if the error indicates a permission issue
func IsPermissionDeniedError(err error) bool {
	return errors.Is(err, ErrPermissionDenied)
}

// IsConfigurationError returns true if the error indicates a configuration issue
func IsConfigurationError(err error) bool {
	return errors.Is(err, ErrConfiguration)
}

// IsUserAborted returns true if the error indicates the user aborted the operation
func IsUserAborted(err error) bool {
	return errors.Is(err, ErrUserAborted)
}
