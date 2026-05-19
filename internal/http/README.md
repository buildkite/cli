# HTTP Client Package

This package contains the Buildkite CLI's shared HTTP helpers. It has two
responsibilities:

- a small JSON `Client` used by commands that call arbitrary REST endpoints
- `http.RoundTripper` implementations used by the command factory for
  authentication, OAuth token refresh, debug logging, and rate-limit retry

## Command factory transport chain

Most commands should use the REST and GraphQL clients from
`pkg/cmd/factory.Factory`. The factory builds one `http.Client` and shares it
between the go-buildkite REST client and GraphQL client.

The transport chain is assembled in `pkg/cmd/factory/factory.go`:

```text
RefreshTransport -> AuthTransport -> debugTransport -> base transport
```

`debugTransport` is only present when debug logging is enabled. Commands may
also provide a custom base transport with `factory.WithTransport`, for example
to add `RateLimitTransport` before the auth and refresh wrappers are applied.

### Authentication headers

`AuthTransport` injects `Authorization: Bearer <token>` and the CLI user agent
on every request. It reads the token from a shared `TokenSource` instead of
capturing a string at construction time, so a refreshed token is visible to
subsequent REST and GraphQL requests without rebuilding clients.

### OAuth token refresh

`RefreshTransport` is installed only when the current organization has a stored
refresh token in the keyring. Refresh tokens are stored under the separate
keyring service `buildkite-cli-refresh`; access tokens use `buildkite-cli`.

On a `401 Unauthorized` response, `RefreshTransport`:

1. checks that a refresh token is still available for the organization
2. serializes refresh attempts with a mutex
3. compares the failed access token with `TokenSource` after acquiring the lock
   so concurrent requests can reuse a token already refreshed by another request
4. exchanges the refresh token via `pkg/oauth.RefreshAccessToken`
5. stores the new access token, rotates the refresh token if one was returned,
   updates `TokenSource`, and retries the original request once

The original request body is buffered when needed so the retry can replay it.
If refresh fails, the original `401` response is returned and a warning is
written to stderr. Terminal OAuth errors (`invalid_grant`,
`unauthorized_client`, and `invalid_client`) delete the stored refresh token;
transient failures such as network errors preserve it so a later request can
try again.

`cmd/auth/login.go` stores refresh tokens only when the system keyring is
available. `cmd/auth/logout.go` removes both access and refresh tokens for the
organization being logged out.

## Rate-limit retry

`RateLimitTransport` retries `429 Too Many Requests` responses. It sleeps for
the `RateLimit-Reset` header value, interpreted as seconds, before retrying.
If the header is missing or invalid, it falls back to a 10 second delay.

Use `NewRateLimitTransport` for the default of three retries. `MaxRetryDelay`
can cap any individual sleep, and `OnRateLimit` can report retry timing to the
caller:

```go
rl := bkhttp.NewRateLimitTransport(nethttp.DefaultTransport)
rl.MaxRetryDelay = 60 * time.Second
rl.OnRateLimit = func(attempt int, delay time.Duration) {
    fmt.Fprintf(os.Stderr, "rate limited; retrying in %v\n", delay)
}
```

Current call sites include `bk api`, which wraps the standalone client, and
`bk preflight`, which passes the transport into the factory with
`factory.WithTransport`.

## Standalone JSON client

`Client` is useful when a command needs a direct REST call instead of the
go-buildkite typed client. It handles:

- JSON request and response bodies
- `GET`, `POST`, `PUT`, `DELETE`, and custom methods through `Do`
- configurable base URL, user agent, and underlying `http.Client`
- `ErrorResponse` values for HTTP status codes `>= 400`

```go
import (
    "errors"
    nethttp "net/http"

    bkhttp "github.com/buildkite/cli/v3/internal/http"
)

client := bkhttp.NewClient(
    token,
    bkhttp.WithBaseURL("https://api.buildkite.com"),
    bkhttp.WithHTTPClient(&nethttp.Client{Transport: rl}),
)

var response SomeResponseType
if err := client.Get(ctx, "/v2/user", &response); err != nil {
    var httpErr *bkhttp.ErrorResponse
    if errors.As(err, &httpErr) && httpErr.IsUnauthorized() {
        // This client does not install RefreshTransport automatically.
        // Use the factory clients when OAuth refresh behavior is required.
    }
}
```

`ErrorResponse` stores the status code, status text, request URL, response
headers, and response body. Helper methods cover common cases:
`IsBadRequest`, `IsUnauthorized`, `IsForbidden`, `IsNotFound`,
`IsTooManyRequests`, and `IsServerError`.

## Related packages

- `pkg/cmd/factory` wires the transports into shared REST and GraphQL clients.
- `pkg/oauth` implements the OAuth PKCE flow and refresh-token exchange.
- `pkg/keyring` stores access and refresh tokens by organization.
- `internal/errors` converts `ErrorResponse` values into CLI error categories
  and suggestions.
