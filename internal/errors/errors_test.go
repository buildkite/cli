package errors

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestErrorInterface(t *testing.T) {
	t.Parallel()

	t.Run("implements error interface", func(t *testing.T) {
		t.Parallel()

		originalErr := fmt.Errorf("original error")
		err := NewError(originalErr, ErrAPI, "additional details")

		// Check that Error() returns a non-empty string
		if err.Error() == "" {
			t.Error("Error() should return a non-empty string")
		}

		// Check that the error string contains both the category and original error
		errStr := err.Error()
		if !strings.Contains(errStr, "API error") {
			t.Errorf("Error string %q should contain category 'API error'", errStr)
		}
		if !strings.Contains(errStr, "original error") {
			t.Errorf("Error string %q should contain original error message", errStr)
		}
	})

	t.Run("formatted error includes suggestions", func(t *testing.T) {
		t.Parallel()

		originalErr := fmt.Errorf("resource not found")
		suggestions := []string{"Check the resource name", "Verify you have access"}
		err := NewError(originalErr, ErrResourceNotFound, "failed to get resource", suggestions...)

		formatted := err.FormattedError()
		for _, suggestion := range suggestions {
			if !strings.Contains(formatted, suggestion) {
				t.Errorf("Formatted error should contain suggestion %q, got: %q", suggestion, formatted)
			}
		}
	})
}

func TestErrorCategorization(t *testing.T) {
	t.Parallel()

	t.Run("errors.Is works with standard error types", func(t *testing.T) {
		t.Parallel()

		apiErr := NewAPIError(nil, "API request failed")
		if !errors.Is(apiErr, ErrAPI) {
			t.Error("errors.Is should identify API error category")
		}

		validationErr := NewValidationError(nil, "Invalid input")
		if !errors.Is(validationErr, ErrValidation) {
			t.Error("errors.Is should identify validation error category")
		}
	})

	t.Run("error type checking functions work", func(t *testing.T) {
		t.Parallel()

		apiErr := NewAPIError(nil, "API request failed")
		if !IsAPIError(apiErr) {
			t.Error("IsAPIError should return true for API errors")
		}

		validationErr := NewValidationError(nil, "Invalid input")
		if !IsValidationError(validationErr) {
			t.Error("IsValidationError should return true for validation errors")
		}

		notFoundErr := NewResourceNotFoundError(nil, "Resource not found")
		if !IsNotFound(notFoundErr) {
			t.Error("IsNotFound should return true for not found errors")
		}
	})
}

func TestErrorWrapping(t *testing.T) {
	t.Parallel()

	t.Run("WithSuggestions adds suggestions", func(t *testing.T) {
		t.Parallel()

		originalErr := NewValidationError(nil, "Invalid input")
		errWithSuggestions := WithSuggestions(originalErr, "Try this instead", "Or this")

		// Verify that it's still a validation error
		if !IsValidationError(errWithSuggestions) {
			t.Error("Error category should be preserved when adding suggestions")
		}

		// Verify suggestions were added
		cliErr, ok := errWithSuggestions.(*Error)
		if !ok {
			t.Fatal("WithSuggestions should return a *Error")
		}
		if len(cliErr.Suggestions) != 2 {
			t.Errorf("Expected 2 suggestions, got %d", len(cliErr.Suggestions))
		}
	})

	t.Run("WithDetails adds details", func(t *testing.T) {
		t.Parallel()

		originalErr := NewAPIError(nil, "API request failed")
		errWithDetails := WithDetails(originalErr, "Additional context")

		// Verify that it's still an API error
		if !IsAPIError(errWithDetails) {
			t.Error("Error category should be preserved when adding details")
		}

		// Verify details were added
		cliErr, ok := errWithDetails.(*Error)
		if !ok {
			t.Fatal("WithDetails should return a *Error")
		}
		if !strings.Contains(cliErr.Details, "Additional context") {
			t.Errorf("Details should contain added context, got %q", cliErr.Details)
		}
	})

	t.Run("Unwrap returns original error", func(t *testing.T) {
		t.Parallel()

		originalErr := fmt.Errorf("original error")
		wrappedErr := NewAPIError(originalErr, "API request failed")

		unwrappedErr := errors.Unwrap(wrappedErr)
		if unwrappedErr != originalErr {
			t.Errorf("Unwrap should return original error, got %v", unwrappedErr)
		}
	})

	t.Run("Unwrap with nil original returns category", func(t *testing.T) {
		t.Parallel()

		wrappedErr := NewAPIError(nil, "API request failed")

		unwrappedErr := errors.Unwrap(wrappedErr)
		if unwrappedErr != ErrAPI {
			t.Errorf("Unwrap should return category when original is nil, got %v", unwrappedErr)
		}
	})

	t.Run("errors.Is works with original error", func(t *testing.T) {
		t.Parallel()

		originalErr := fmt.Errorf("not found: resource does not exist")
		wrappedErr := NewResourceNotFoundError(originalErr, "Could not find resource")

		// Should match both the category and the original error
		if !errors.Is(wrappedErr, ErrResourceNotFound) {
			t.Error("errors.Is should match error category")
		}
		if !errors.Is(wrappedErr, originalErr) {
			t.Error("errors.Is should match original error")
		}
	})
}

func TestErrorCreationHelpers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		createFunc func(error, string, ...string) error
		errorType  error
		checkFunc  func(error) bool
	}{
		{
			name:       "NewConfigurationError",
			createFunc: NewConfigurationError,
			errorType:  ErrConfiguration,
			checkFunc:  IsConfigurationError,
		},
		{
			name:       "NewValidationError",
			createFunc: NewValidationError,
			errorType:  ErrValidation,
			checkFunc:  IsValidationError,
		},
		{
			name:       "NewAPIError",
			createFunc: NewAPIError,
			errorType:  ErrAPI,
			checkFunc:  IsAPIError,
		},
		{
			name:       "NewResourceNotFoundError",
			createFunc: NewResourceNotFoundError,
			errorType:  ErrResourceNotFound,
			checkFunc:  IsNotFound,
		},
		{
			name:       "NewPermissionDeniedError",
			createFunc: NewPermissionDeniedError,
			errorType:  ErrPermissionDenied,
			checkFunc:  IsPermissionDeniedError,
		},
		{
			name:       "NewAuthenticationError",
			createFunc: NewAuthenticationError,
			errorType:  ErrAuthentication,
			checkFunc:  IsAuthenticationError,
		},
		{
			name:       "NewUserAbortedError",
			createFunc: NewUserAbortedError,
			errorType:  ErrUserAborted,
			checkFunc:  IsUserAborted,
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture test case value
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			originalErr := fmt.Errorf("some error")
			details := "detailed error message"
			suggestion := "helpful suggestion"

			err := tc.createFunc(originalErr, details, suggestion)

			// Check that the error has the right category
			if !errors.Is(err, tc.errorType) {
				t.Errorf("Error should be of type %v", tc.errorType)
			}

			// Check that the error type check function works
			if !tc.checkFunc(err) {
				t.Errorf("Type check function should return true")
			}

			// Check that details and suggestions are included
			cliErr, ok := err.(*Error)
			if !ok {
				t.Fatal("Error should be a *Error")
			}
			if cliErr.Details != details {
				t.Errorf("Expected details %q, got %q", details, cliErr.Details)
			}
			if len(cliErr.Suggestions) != 1 || cliErr.Suggestions[0] != suggestion {
				t.Errorf("Expected suggestion %q, got %v", suggestion, cliErr.Suggestions)
			}
		})
	}
}
