package factory

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"regexp"
	"strings"

	"github.com/Khan/genqlient/graphql"
	"github.com/buildkite/cli/v3/cmd/version"
	"github.com/buildkite/cli/v3/internal/config"
	bkhttp "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/pkg/keyring"
	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
)

var userAgent string

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
		fmt.Fprintf(os.Stderr, "DEBUG request uri=%s\n%s\n", req.URL, redactBody(string(dump)))
	}

	resp, err := d.transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if dump, err := httputil.DumpResponse(resp, true); err == nil {
		fmt.Fprintf(os.Stderr, "DEBUG response uri=%s\n%s\n", req.URL, redactBody(string(dump)))
	}

	return resp, nil
}

// sensitiveBodyPatterns matches token values in form-encoded request bodies
// and JSON response bodies that should be redacted in debug output.
var sensitiveBodyPatterns = regexp.MustCompile(
	`((?:refresh_token|access_token|code|code_verifier)=)[^&\s]+` +
		`|("(?:access_token|refresh_token|code)":\s*")[^"]+("?)`,
)

// redactBody replaces sensitive token values in HTTP dumps.
func redactBody(dump string) string {
	return sensitiveBodyPatterns.ReplaceAllStringFunc(dump, func(match string) string {
		// Form-encoded: key=value
		if idx := strings.IndexByte(match, '='); idx > 0 && !strings.HasPrefix(match, `"`) {
			return match[:idx+1] + "[REDACTED]"
		}
		// JSON: "key": "value"
		return sensitiveBodyPatterns.ReplaceAllString(match, `${1}[REDACTED]${2}`)
	})
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
	client *http.Client
}

func init() {
	userAgent = fmt.Sprintf("%s buildkite-cli/%s", buildkite.DefaultUserAgent, version.Version)
}

func (a *gqlHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Auth and User-Agent are injected by AuthTransport in the
	// shared HTTP transport chain, so we don't set them here.
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

	// Build the HTTP transport chain.
	//
	// The chain is (outermost first):
	//   RefreshTransport → AuthTransport → debugTransport → DefaultTransport
	//
	// AuthTransport reads the current token from a shared TokenSource on
	// every request, so after a refresh all subsequent requests (REST and
	// GraphQL) immediately use the new token — no stale cached values.
	var transport = http.DefaultTransport

	if cfg.debug {
		transport = &debugTransport{transport: transport}
	}

	tokenSource := bkhttp.NewTokenSource(token)

	transport = &bkhttp.AuthTransport{
		Base:        transport,
		TokenSource: tokenSource,
		UserAgent:   userAgent,
	}

	// Add refresh transport if a refresh token is available for this org
	org := conf.OrganizationSlug()
	if cfg.orgOverride != "" {
		org = cfg.orgOverride
	}

	kr := keyring.New()
	if refreshToken, err := kr.GetRefreshToken(org); err == nil && refreshToken != "" {
		transport = &bkhttp.RefreshTransport{
			Base:        transport,
			Org:         org,
			Keyring:     kr,
			TokenSource: tokenSource,
		}
	}

	httpClient := &http.Client{Transport: transport}

	// go-buildkite still needs WithTokenAuth to satisfy its constructor
	// requirement, but our AuthTransport is the canonical source of the
	// Authorization header.
	clientOpts := []buildkite.ClientOpt{
		buildkite.WithBaseURL(conf.RESTAPIEndpoint()),
		buildkite.WithTokenAuth(token),
		buildkite.WithUserAgent(userAgent),
		buildkite.WithHTTPClient(httpClient),
	}

	buildkiteClient, err := buildkite.NewOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating buildkite client: %w", err)
	}

	graphqlHTTPClient := &gqlHTTPClient{client: httpClient}

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
