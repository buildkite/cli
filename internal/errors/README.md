# Error Handling Package

This package provides standardized error handling for the Buildkite CLI. It handles different types of errors consistently, provides helpful error messages and suggestions to users, and ensures a unified approach to error reporting across the application.

## Features

- **Categorized Errors**: Different error types (validation, API, not found, etc.) with specific handling
- **Contextual Error Messages**: Errors include context about what operation failed
- **Helpful Suggestions**: Error messages include suggestions on how to fix the issue
- **Command Integration**: Easy integration with Cobra commands
- **API Error Handling**: Specialized handling for API errors with status code interpretation
- **Exit Codes**: Appropriate exit codes for different error types

## Usage

### Creating Errors

```go
import (
    bkErrors "github.com/buildkite/cli/v3/internal/errors"
)

// Create various types of errors
validationErr := bkErrors.NewValidationError(
    err, // Original error (can be nil)
    "Invalid input", // Details about the error
    "Try using a different value", // Suggestions (optional)
    "Check the documentation for valid options" // More suggestions
)

apiErr := bkErrors.NewAPIError(err, "API request failed")

notFoundErr := bkErrors.NewResourceNotFoundError(err, "Resource not found")
```

### Adding Context to Errors

```go
// Add suggestions to an existing error
err = bkErrors.WithSuggestions(err, 
    "Try this instead", 
    "Or try this other option"
)

// Add details to an existing error
err = bkErrors.WithDetails(err, "Additional context")
```

### Checking Error Types

```go
if bkErrors.IsNotFound(err) {
    // Handle not found error
}

if bkErrors.IsValidationError(err) {
    // Handle validation error
}

if bkErrors.IsAPIError(err) {
    // Handle API error
}
```

### Wrapping API Errors

```go
// Wrap HTTP errors with appropriate context
err = bkErrors.WrapAPIError(err, "fetching pipeline")
```

### Command Integration

```go
// Wrap a command's RunE function with standard error handling
cmd := &cobra.Command{
    RunE: bkErrors.WrapRunE(func(cmd *cobra.Command, args []string) error {
        // Command implementation
        return err
    }),
}
```

### Using the Error Handler

```go
// Create an error handler
handler := bkErrors.NewHandler().
    WithVerbose(verbose).
    WithWriter(os.Stderr)

// Handle an error
handler.Handle(err)

// Handle an error with operation context
handler.HandleWithDetails(err, "creating resource")

// Print a warning
handler.PrintWarning("Something might be wrong: %s", details)
```

### Using the Command Error Handler

```go
// In main.go:
func main() {
    rootCmd, _ := root.NewCmdRoot(f)
    
    // Execute with error handling
    bkErrors.ExecuteWithErrorHandling(rootCmd, verbose)
}
```
