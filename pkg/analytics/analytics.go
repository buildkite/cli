package analytics

import (
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/posthog/posthog-go"
)

var (
	// Set via -ldflags at build time: -X github.com/buildkite/cli/v3/pkg/analytics.apiKey=...
	apiKey  = ""
	apiHost = "https://us.i.posthog.com"
)

var (
	client posthog.Client
	once   sync.Once
)

type Client struct {
	posthog  posthog.Client
	disabled bool
	userID   string
	org      string
}

func Init(version string, enabled bool) *Client {
	if !enabled {
		return &Client{disabled: true}
	}

	key := apiKey
	if envKey := os.Getenv("BK_ANALYTICS_KEY"); envKey != "" {
		key = envKey
	}

	if key == "" || os.Getenv("CI") != "" {
		return &Client{disabled: true}
	}

	once.Do(func() {
		var err error
		client, err = posthog.NewWithConfig(key, posthog.Config{
			Endpoint: apiHost,
			Logger:   noopLogger{},
		})
		if err != nil {
			client = nil
		}
	})

	if client == nil {
		return &Client{disabled: true}
	}

	return &Client{
		posthog: client,
		userID:  getUserID(),
	}
}

func (c *Client) SetOrg(org string) {
	if c.disabled {
		return
	}
	c.org = org
}

func (c *Client) TrackCommand(subcommand string, fullArgs []string, properties map[string]interface{}) {
	if c.disabled || c.posthog == nil {
		return
	}

	props := posthog.NewProperties()
	props.Set("command", strings.Join(fullArgs, " "))
	props.Set("channel", "cli")
	props.Set("os", runtime.GOOS)
	props.Set("arch", runtime.GOARCH)
	if c.org != "" {
		props.Set("organization", c.org)
	}

	for k, v := range properties {
		props.Set(k, v)
	}

	_ = c.posthog.Enqueue(posthog.Capture{
		DistinctId: c.userID,
		Event:      subcommand,
		Properties: props,
	})
}

func (c *Client) Close() {
	if c.disabled || c.posthog == nil {
		return
	}
	_ = c.posthog.Close()
}

func getUserID() string {
	if id := os.Getenv("BUILDKITE_BUILD_ID"); id != "" {
		return "build:" + id
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "anonymous"
	}
	return "host:" + hostname
}

// ParseSubcommand extracts the subcommand path from Kong's command string,
// removing angle-bracket arguments like "<pipeline>".
func ParseSubcommand(kongCommand string) string {
	parts := strings.Fields(kongCommand)
	var cmdParts []string
	for _, p := range parts {
		if !strings.HasPrefix(p, "<") {
			cmdParts = append(cmdParts, p)
		}
	}
	return strings.Join(cmdParts, " ")
}
