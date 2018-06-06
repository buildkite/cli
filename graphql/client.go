package graphql

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

func (client *Client) Do(query string, vars map[string]interface{}) (*Response, error) {
	b, err := json.MarshalIndent(struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}{
		Query:     strings.TrimSpace(query),
		Variables: vars,
	}, "", "  ")
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

	if err := checkResponseForErrors(resp); err != nil {
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
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (r *errorResponse) Error() string {
	var errors []string
	for _, err := range r.Errors {
		errors = append(errors, err.Message)
	}
	return fmt.Sprintf("GraphQL error: %s", strings.Join(errors, ", "))
}

func checkResponseForErrors(r *http.Response) error {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(data))

	var errResp errorResponse

	_ = json.Unmarshal(data, &errResp)
	if len(errResp.Errors) > 0 {
		return &errResp
	}

	return nil
}
