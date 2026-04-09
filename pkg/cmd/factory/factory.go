package factory

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/Khan/genqlient/graphql"
	"github.com/buildkite/cli/v3/cmd/version"
	"github.com/buildkite/cli/v3/internal/config"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
)

var baseUserAgent string

type Factory struct {
	Config        *config.Config
	GitRepository *git.Repository
	GraphQLClient graphql.Client
	RestAPIClient *buildkite.Client
	Version       string
	SkipConfirm   bool
	NoInput       bool
	Quiet         bool
	NoPager       bool
	Debug         bool
}

// FactoryOpt is a functional option for configuring the Factory
type FactoryOpt func(*factoryConfig)

type factoryConfig struct {
	debug       bool
	orgOverride string
	transport   http.RoundTripper
}

// WithDebug enables debug output for REST API calls
func WithDebug(debug bool) FactoryOpt {
	return func(c *factoryConfig) {
		c.debug = debug
	}
}

// WithOrgOverride overrides the configured organization slug for API token
// resolution. When set, the factory will use the token for this org instead
// of the currently selected org.
func WithOrgOverride(org string) FactoryOpt {
	return func(c *factoryConfig) {
		c.orgOverride = org
	}
}

// WithTransport sets a custom http.RoundTripper for the REST API client.
// It is composed with the debug transport when debug mode is enabled.
func WithTransport(t http.RoundTripper) FactoryOpt {
	return func(c *factoryConfig) {
		c.transport = t
	}
}

// debugTransport wraps an http.RoundTripper and logs requests/responses with sensitive headers redacted
type debugTransport struct {
	transport http.RoundTripper
}

// sensitiveHeaders contains headers that should be redacted in debug output
var sensitiveHeaders = []string{"Authorization"}

func (d *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Save and restore the request body so that dumping it does not consume
	// the body before the real transport sends it. req.Clone() shares the
	// underlying Body reader, so DumpRequestOut on a clone drains the
	// original — leading to an empty/malformed request reaching the server.
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("debug transport: reading request body: %w", err)
		}
		// Restore the body for the actual request
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Build a clone with its own copy of the body for dumping
	reqCopy := req.Clone(req.Context())
	redactHeaders(reqCopy.Header)
	if bodyBytes != nil {
		reqCopy.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	if dump, err := httputil.DumpRequestOut(reqCopy, true); err == nil {
		fmt.Fprintf(os.Stderr, "DEBUG request uri=%s\n%s\n", req.URL, dump)
	}

	resp, err := d.transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if dump, err := httputil.DumpResponse(resp, true); err == nil {
		fmt.Fprintf(os.Stderr, "DEBUG response uri=%s\n%s\n", req.URL, dump)
	}

	return resp, nil
}

// redactHeaders replaces sensitive header values with [REDACTED]
func redactHeaders(headers http.Header) {
	for _, header := range sensitiveHeaders {
		if values := headers.Values(header); len(values) > 0 {
			for i, v := range values {
				// Keep the auth type (Bearer, Basic, etc.) but redact the token
				if parts := strings.SplitN(v, " ", 2); len(parts) == 2 {
					headers[header][i] = parts[0] + " [REDACTED]"
				} else {
					headers[header][i] = "[REDACTED]"
				}
			}
		}
	}
}

type gqlHTTPClient struct {
	client    *http.Client
	token     string
	userAgent string
}

func init() {
	baseUserAgent = fmt.Sprintf("%s buildkite-cli/%s", buildkite.DefaultUserAgent, version.Version)
}

func buildUserAgent(suffix string) string {
	if suffix == "" {
		return baseUserAgent
	}
	return fmt.Sprintf("%s %s", baseUserAgent, suffix)
}

func (a *gqlHTTPClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.token))
	req.Header.Set("User-Agent", a.userAgent)
	return a.client.Do(req)
}

func New(opts ...FactoryOpt) (*Factory, error) {
	cfg := &factoryConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	if err != nil {
		if err == git.ErrRepositoryNotExists {
			repo = nil
		}
	}

	conf := config.New(nil, repo)

	token := conf.APIToken()
	if cfg.orgOverride != "" {
		if t := conf.APITokenForOrg(cfg.orgOverride); t != "" {
			token = t
		}
	}

	userAgent := buildUserAgent(cfg.userAgentSuffix)

	// Build client options
	clientOpts := []buildkite.ClientOpt{
		buildkite.WithBaseURL(conf.RESTAPIEndpoint()),
		buildkite.WithTokenAuth(token),
		buildkite.WithUserAgent(userAgent),
	}

	// Use custom transport if provided (caller is responsible for wrapping
	// http.DefaultTransport if needed), then wrap with debug transport when enabled.
	transport := http.RoundTripper(http.DefaultTransport)
	if cfg.transport != nil {
		transport = cfg.transport
	}
	if cfg.debug {
		transport = &debugTransport{transport: transport}
	}
	clientOpts = append(clientOpts, buildkite.WithHTTPClient(&http.Client{Transport: transport}))

	buildkiteClient, err := buildkite.NewOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating buildkite client: %w", err)
	}

	graphqlHTTPClient := &gqlHTTPClient{client: http.DefaultClient, token: token, userAgent: userAgent}

	return &Factory{
		Config:        conf,
		GitRepository: repo,
		GraphQLClient: graphql.NewClient(conf.GetGraphQLEndpoint(), graphqlHTTPClient),
		RestAPIClient: buildkiteClient,
		Version:       version.Version,
		NoPager:       conf.PagerDisabled(),
		Quiet:         conf.Quiet(),
		NoInput:       conf.NoInput(),
		Debug:         cfg.debug,
	}, nil
}
