package analytics

import (
	"errors"
	"net"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/posthog/posthog-go"
)

type captureClient struct {
	posthog.Client
	events []posthog.Capture
}

func (c *captureClient) Enqueue(message posthog.Message) error {
	c.events = append(c.events, message.(posthog.Capture))
	return errors.New("delivery unavailable")
}

func TestTrackCommand(t *testing.T) {
	for _, tt := range []struct {
		command, group, action, outcome, org string
	}{
		{"build view", "build", "view", "success", "acme"},
		{"pipeline alias add", "pipeline", "alias add", "error", ""},
		{"version", "version", "", "success", ""},
		{"build", "build", "", "unknown_command", ""},
		{"", "", "", "unknown_command", ""},
	} {
		t.Run(tt.command+"/"+tt.outcome, func(t *testing.T) {
			transport := &captureClient{}
			c := &Client{posthog: transport, userID: "test-user", version: "v3.42.0"}
			c.SetOrg(tt.org)
			c.TrackCommand(tt.command, tt.outcome)
			if len(transport.events) != 1 {
				t.Fatalf("got %d events, want one", len(transport.events))
			}
			event := transport.events[0]
			if event.Event != "platform:cli:command_executed" || event.DistinctId != "test-user" {
				t.Fatalf("unexpected event: %+v", event)
			}
			want := posthog.Properties{
				"outcome": tt.outcome, "channel": "cli", "cli_version": "v3.42.0",
				"os": runtime.GOOS, "arch": runtime.GOARCH,
			}
			if tt.command != "" {
				want["command"], want["command_group"], want["command_action"] = tt.command, tt.group, tt.action
			}
			if tt.org != "" {
				want["organization_slug"] = tt.org
			}
			// Exact comparison also rejects raw args and incorrectly named org fields.
			if !reflect.DeepEqual(event.Properties, want) {
				t.Errorf("properties = %#v, want %#v", event.Properties, want)
			}
		})
	}
}

func TestDisabledTracking(t *testing.T) {
	transport := &captureClient{}
	c := &Client{posthog: transport, disabled: true}
	c.TrackCommand("version", "success")
	if len(transport.events) != 0 {
		t.Fatal("disabled telemetry enqueued an event")
	}
	(&Client{}).TrackCommand("version", "success")
}

// Telemetry must never delay CLI exit. This pins the shutdown bound by
// pointing the client at a server that accepts connections but never
// responds, and asserting Close returns promptly instead of blocking on
// the flush.
func TestCloseIsBoundedWhenEndpointIsUnresponsive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	// Hold accepted connections open without ever responding. The accept
	// goroutine owns closing conns, once Accept fails after ln is closed.
	conns := make(chan net.Conn, 16)
	go func() {
		defer close(conns)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conns <- conn
		}
	}()
	defer func() {
		ln.Close()
		for conn := range conns {
			conn.Close()
		}
	}()

	t.Setenv("BK_ANALYTICS_KEY", "test-key")
	t.Setenv("CI", "")

	origHost := apiHost
	apiHost = "http://" + ln.Addr().String()
	t.Cleanup(func() { apiHost = origHost })

	c := Init("test", true)
	if c.disabled {
		t.Fatal("expected client to be enabled")
	}
	if c.version != "test" {
		t.Fatalf("version = %q, want test", c.version)
	}
	c.TrackCommand("version", "success")

	start := time.Now()
	c.Close()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Close took %v, want under 2s", elapsed)
	}
}
