# HTTP Client Package

This package provides a common HTTP client with standardized headers and error handling for Buildkite API requests.

## Features

- Standardized authorization header handling
- Common error handling for API responses
- Support for different HTTP methods (GET, POST, PUT, DELETE)
- JSON request and response handling
- Configurable base URL and user agent

## Usage

### Creating a client

```go
import (
    "github.com/buildkite/cli/v3/internal/http"
)

// Basic client with token
client := http.NewClient("your-api-token")

// Client with custom base URL
client := http.NewClient(
    "your-api-token",
    http.WithBaseURL("https://api.example.com"),
)

// Client with custom user agent
client := http.NewClient(
    "your-api-token",
    http.WithUserAgent("my-app/1.0"),
)

// Client with custom HTTP client
client := http.NewClient(
    "your-api-token",
    http.WithHTTPClient(customHTTPClient),
)
```

### Making requests

```go
// GET request
var response SomeResponseType
err := client.Get(ctx, "/endpoint", &response)

// POST request with body
requestBody := map[string]string{"key": "value"}
var response SomeResponseType
err := client.Post(ctx, "/endpoint", requestBody, &response)

// PUT request
err := client.Put(ctx, "/endpoint", requestBody, &response)

// DELETE request
err := client.Delete(ctx, "/endpoint", &response)

// Custom method
err := client.Do(ctx, "PATCH", "/endpoint", requestBody, &response)
```

### Error handling

```go
err := client.Get(ctx, "/endpoint", &response)
if err != nil {
    // Check if it's an HTTP error
    if httpErr, ok := err.(*http.ErrorResponse); ok {
        fmt.Printf("HTTP error: %d %s\n", httpErr.StatusCode, httpErr.Status)
        fmt.Printf("Response body: %s\n", httpErr.Body)
    } else {
        fmt.Printf("Other error: %v\n", err)
    }
}
```
