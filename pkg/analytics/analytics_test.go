package analytics

import (
	"net"
	"testing"
	"time"
)

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
	c.TrackCommand("version", []string{"version"}, nil)

	start := time.Now()
	c.Close()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Close took %v, want under 2s", elapsed)
	}
}
