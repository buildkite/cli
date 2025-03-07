# Test Utilities

This package provides reusable test helpers for the CLI, allowing for more consistent and easier testing across the codebase.

## Overview

The test utilities are organized into several categories:

1. **Factory Helpers** - `factory.go`
   - Create test factories for common test scenarios

2. **HTTP Server Mocks** - `http.go`
   - Mock HTTP servers for testing API interactions

3. **Command Testing** - `command.go`
   - Utilities for testing Cobra commands

4. **TUI Testing** - `tea.go`
   - Utilities for testing Bubble Tea TUI components

5. **Assertions** - `assert.go`
   - Common assertion helpers

## Usage Examples

### Creating a Test Factory

```go
// Create a test factory with a mock HTTP server
s := testutil.MockHTTPServer(`[{"slug": "my-pipeline"}]`)
defer s.Close()

f := testutil.CreateFactory(t, s.URL, "testOrg", testutil.GitRepository())
```

### Testing Command Execution

```go
// Create and execute a test command
cmd, err := testutil.CreateCommand(t, testutil.CommandInput{
    TestServerURL: server.URL,
    Flags:         map[string]string{"flag": "value"},
    Args:          []string{"arg1", "arg2"},
    NewCmd:        pkg.NewCommand,
})

err = cmd.Execute()
testutil.AssertNoError(t, err)
```

### Testing TUI Components

```go
// Test simple TUI output
model := NewModel()
testutil.AssertTeaOutput(t, model, "Expected output")

// Test complex TUI interactions
opts := testutil.DefaultTeaTestOptions()
testModel := teatest.NewTestModel(t, model)
testutil.TestTeaModel(t, model, "Initial output", opts)

// Send events
testModel.Send(someEvent)

// Wait for output
teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
    return testutil.Contains(bts, "Expected output after event")
})
```

### Using Assertions

```go
// Assert no error
testutil.AssertNoError(t, err)

// Assert error contains text
testutil.AssertErrorContains(t, err, "expected message")

// Assert error type
testutil.AssertErrorIs(t, err, ErrExpectedError)

// Assert equality
testutil.AssertEqual(t, result, expected, "Result value")
```

## Contributing

When adding new test helpers:

1. Place them in the appropriate file based on their function
2. Add comprehensive documentation
3. Follow existing patterns for consistency
4. Consider writing tests for complex helpers