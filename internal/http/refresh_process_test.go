package http

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/buildkite/cli/v3/pkg/keyring"
)

func TestRefreshProcess(t *testing.T) {
	if endpoint := os.Getenv("BK_REFRESH_PROCESS_ENDPOINT"); endpoint != "" {
		// Only the synthetic TLS server in this test uses this transport.
		der, err := base64.StdEncoding.DecodeString(os.Getenv("BK_REFRESH_PROCESS_CERT"))
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		roots.AddCert(cert)
		http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
		kr := keyring.New()
		ts := NewTokenSource("expired")
		client := &http.Client{Transport: &RefreshTransport{
			Base: &AuthTransport{TokenSource: ts}, Org: "process-test", Keyring: kr,
			TokenSource: ts, APIEndpoints: []string{endpoint},
		}}
		resp, err := client.Get(endpoint + "/api")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		return
	}

	t.Setenv(keyring.CredentialStoreEnv, keyring.StoreSHM)
	t.Setenv(keyring.CredentialStorePathEnv, filepath.Join(t.TempDir(), "private", "credentials.json"))
	kr := keyring.New()
	if err := kr.Set("process-test", "expired"); err != nil {
		t.Fatal(err)
	}
	if err := kr.SetRefreshToken("process-test", "refresh-original"); err != nil {
		t.Fatal(err)
	}
	var stale, exchanges atomic.Int32
	bothStale := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			n := exchanges.Add(1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"access_token":"fresh-%d","refresh_token":"refresh-%d"}`, n, n)
			return
		}
		if r.Header.Get("Authorization") == "Bearer expired" {
			if stale.Add(1) == 2 {
				close(bothStale)
			}
			<-bothStale
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer fresh-") {
			t.Errorf("unexpected synthetic authorization")
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()
	t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))
	t.Setenv("BK_REFRESH_PROCESS_ENDPOINT", server.URL)
	t.Setenv("BK_REFRESH_PROCESS_CERT", base64.StdEncoding.EncodeToString(server.Certificate().Raw))
	commands := make([]*exec.Cmd, 2)
	for i := range commands {
		commands[i] = exec.Command(os.Args[0], "-test.run=^TestRefreshProcess$", "-test.timeout=15s")
		commands[i].Stdout = os.Stdout
		commands[i].Stderr = os.Stderr
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if got := exchanges.Load(); got != 1 {
		t.Fatalf("two processes exchanged refresh tokens %d times, want 1", got)
	}
	if token, err := kr.Get("process-test"); err != nil || token != "fresh-1" {
		t.Fatalf("persisted access token = %q, %v", token, err)
	}
	if token, err := kr.GetRefreshToken("process-test"); err != nil || token != "refresh-1" {
		t.Fatalf("persisted refresh token = %q, %v", token, err)
	}
}
