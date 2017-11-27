package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

const (
	DefaultEndpoint = "https://graphql.buildkite.com/v1"
)

var (
	DebugHTTP bool
)

func NewClientWithEndpoint(token string, endpoint string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	header := make(http.Header)
	header.Add("Content-Type", "application/json")
	header.Add("Authorization", "Bearer "+token)
	return &Client{
		endpoint:   u,
		header:     header,
		httpClient: http.DefaultClient,
	}, nil
}

func NewClient(token string) (*Client, error) {
	return NewClientWithEndpoint(token, DefaultEndpoint)
}

type Client struct {
	token      string
	endpoint   *url.URL
	httpClient *http.Client
	header     http.Header
}

func (client *Client) Do(query string) (*Response, error) {
	b, err := json.Marshal(map[string]string{
		"query": query,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, client.endpoint.String(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header = client.header

	if DebugHTTP {
		if dump, err := httputil.DumpRequest(req, true); err == nil {
			fmt.Printf("DEBUG request uri=%s\n%s\n", req.URL, dump)
		}
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if DebugHTTP {
		if dump, err := httputil.DumpResponse(resp, true); err == nil {
			fmt.Printf("DEBUG response uri=%s\n%s\n", req.URL, dump)
		}
	}

	if err := checkResponse(resp); err != nil {
		return &Response{resp}, err
	}

	return &Response{resp}, nil
}

type Response struct {
	*http.Response
}

func (r *Response) DecodeInto(v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

type errorResponse struct {
	Response *http.Response
	Errors   []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (r *errorResponse) Error() string {
	var errors []string
	for _, err := range r.Errors {
		errors = append(errors, err.Message)
	}
	return strings.Join(errors, ", ")
}

func checkResponse(r *http.Response) error {
	if c := r.StatusCode; 200 <= c && c <= 299 {
		return nil
	}
	data, err := ioutil.ReadAll(r.Body)
	errResp := &errorResponse{Response: r}
	if err = json.Unmarshal(data, errResp); err != nil {
		return fmt.Errorf("Failed to parse error response for code %d: %v", r.StatusCode, err)
	}
	return errResp
}
