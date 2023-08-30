package api

import "net/http"

type headerRoundTripper struct {
	next    http.RoundTripper
	headers map[string]string
}

func NewHTTPClient(headers map[string]string) *http.Client {
	transport := newHeaderRoundTripper(headers, http.DefaultTransport)

	return &http.Client{
		Transport: transport,
	}
}

// RoundTrip implements http.RoundTripper.
func (hrt headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range hrt.headers {
		req.Header.Set(k, v)
	}

	return hrt.next.RoundTrip(req)
}

func newHeaderRoundTripper(headers map[string]string, rt http.RoundTripper) http.RoundTripper {
	return headerRoundTripper{
		headers: headers,
		next:    rt,
	}
}
