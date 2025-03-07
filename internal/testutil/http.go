package testutil

import (
	"net/http"
	"net/http/httptest"
)

// MockHTTPServer creates a test HTTP server that returns the specified response
func MockHTTPServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	}))
}

// MockHTTPServerWithHandler creates a test HTTP server with a custom handler
func MockHTTPServerWithHandler(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}
