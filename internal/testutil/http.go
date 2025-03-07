package testutil

import (
	"log"
	"net/http"
	"net/http/httptest"
)

// MockHTTPServer creates a test HTTP server that returns the specified response
func MockHTTPServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(response))
		if err != nil {
			log.Fatalf("error writing response: %v", err)
		}
	}))
}

// MockHTTPServerWithHandler creates a test HTTP server with a custom handler
func MockHTTPServerWithHandler(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}
