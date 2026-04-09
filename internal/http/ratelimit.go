package http

import (
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	// DefaultMaxRateLimitRetries is the default number of times to retry a
	// rate-limited request.
	DefaultMaxRateLimitRetries = 3

	// defaultFallbackDelay is used when the server returns 429 but the
	// RateLimit-Reset header is missing or unparseable.
	defaultFallbackDelay = 10 * time.Second
)

// OnRateLimitFunc is called before sleeping for a rate-limit backoff.
// attempt is zero-indexed; delay is how long the transport will sleep.
type OnRateLimitFunc func(attempt int, delay time.Duration)

// RateLimitTransport wraps an http.RoundTripper and automatically retries
// requests that receive an HTTP 429 response, sleeping for the duration
// indicated by the RateLimit-Reset header.
type RateLimitTransport struct {
	// Transport is the underlying RoundTripper. If nil, http.DefaultTransport
	// is used.
	Transport http.RoundTripper

	// MaxRetries is the maximum number of retry attempts on 429. Zero means
	// no retries; negative values are treated as zero.
	MaxRetries int

	// MaxRetryDelay caps the sleep duration for any single retry. Zero means
	// no cap is applied.
	MaxRetryDelay time.Duration

	// OnRateLimit is an optional callback invoked before each backoff sleep.
	OnRateLimit OnRateLimitFunc
}

// NewRateLimitTransport returns a RateLimitTransport wrapping the given
// transport with sensible defaults.
func NewRateLimitTransport(transport http.RoundTripper) *RateLimitTransport {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &RateLimitTransport{
		Transport:  transport,
		MaxRetries: DefaultMaxRateLimitRetries,
	}
}

// RoundTrip implements http.RoundTripper. On a 429 response it reads the
// RateLimit-Reset header (seconds until the rate-limit window resets) and
// sleeps for that duration before retrying, up to MaxRetries times.
func (t *RateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	for attempt := 0; ; attempt++ {
		// Reset the request body for retries.
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}

		resp, err := transport.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusTooManyRequests || attempt >= t.MaxRetries {
			return resp, nil
		}

		delay, ok := parseRateLimitReset(resp)
		if !ok {
			delay = defaultFallbackDelay
		}
		if t.MaxRetryDelay > 0 && delay > t.MaxRetryDelay {
			delay = t.MaxRetryDelay
		}

		// Drain and close the 429 response body before retrying.
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if t.OnRateLimit != nil {
			t.OnRateLimit(attempt, delay)
		}

		// Sleep for the backoff duration, but honour context cancellation.
		timer := time.NewTimer(delay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
}

// parseRateLimitReset reads the RateLimit-Reset header and returns the
// duration to wait plus a boolean indicating whether the value was valid.
func parseRateLimitReset(resp *http.Response) (time.Duration, bool) {
	s := resp.Header.Get("RateLimit-Reset")
	if s == "" {
		return 0, false
	}
	seconds, err := strconv.Atoi(s)
	if err != nil || seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}
