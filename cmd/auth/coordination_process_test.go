package auth

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bkhttp "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
)

// Pauses a real mutation between its separate storage writes. The pipes are
// barriers, not a replacement credential store or a timing-based scheduler.
type stagedCredentialStore struct {
	*keyring.Keyring
	stage bool
	bound bool
}

func (s *stagedCredentialStore) Lock(ctx context.Context, org string) (func(), error) {
	if s.bound {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
	}
	return s.Keyring.Lock(ctx, org)
}

func (s *stagedCredentialStore) pause() {
	if s.stage {
		fmt.Println("holding")
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
}

func (s *stagedCredentialStore) Set(org, token string) error {
	s.pause()
	return s.Keyring.Set(org, token)
}

func (s *stagedCredentialStore) Delete(org string) error {
	err := s.Keyring.Delete(org)
	s.pause()
	return err
}

func TestCredentialMutationProcesses(t *testing.T) {
	if role := os.Getenv("BK_AUTH_PROCESS_ROLE"); role != "" {
		runCredentialProcess(t, role)
		return
	}
	for _, mutation := range []string{"oauth", "pat", "logout"} {
		for _, order := range []string{"mutation-first", "refresh-first", "terminal-first"} {
			t.Run(mutation+"/"+order, func(t *testing.T) {
				t.Setenv(keyring.CredentialStoreEnv, keyring.StoreSHM)
				t.Setenv(keyring.CredentialStorePathEnv, filepath.Join(t.TempDir(), "private", "credentials.json"))
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				t.Setenv("BUILDKITE_NO_KEYRING", "1")
				kr := keyring.New()
				if err := kr.Set("mutation-test", "expired"); err != nil {
					t.Fatal(err)
				}
				if err := kr.SetRefreshToken("mutation-test", "refresh-original"); err != nil {
					t.Fatal(err)
				}
				var exchanges atomic.Int32
				entered, resume := make(chan struct{}), make(chan struct{})
				var resumeOnce sync.Once
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/oauth/token" {
						if exchanges.Add(1) == 1 {
							close(entered)
						}
						<-resume
						w.Header().Set("Content-Type", "application/json")
						if order == "terminal-first" {
							w.WriteHeader(http.StatusBadRequest)
							_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
						} else {
							_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"refresh-fresh"}`)
						}
						return
					}
					if r.Header.Get("Authorization") == "Bearer expired" {
						w.WriteHeader(http.StatusUnauthorized)
					}
				}))
				defer func() {
					resumeOnce.Do(func() { close(resume) })
					server.Close()
				}()
				t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))
				t.Setenv("BK_AUTH_PROCESS_ENDPOINT", server.URL)
				t.Setenv("BK_AUTH_PROCESS_CERT", base64.StdEncoding.EncodeToString(server.Certificate().Raw))

				if order == "mutation-first" {
					cmd, gate := startCredentialProcess(t, mutation, "stage")
					// Refresh must wait even though the access/refresh pair is partial.
					blocked, _ := startCredentialProcess(t, "refresh", "bound")
					waitCredentialProcess(t, blocked)
					_ = gate.Close()
					waitCredentialProcess(t, cmd)
					want := "ok"
					if mutation == "logout" {
						want = "unauthorized"
					}
					refresh, _ := startCredentialProcess(t, "refresh", want)
					waitCredentialProcess(t, refresh)
					if n := exchanges.Load(); n != 0 {
						t.Fatalf("refresh after replacement/removal exchanged %d times", n)
					}
				} else {
					want := "ok"
					if order == "terminal-first" {
						want = "unauthorized"
					}
					refresh, _ := startCredentialProcess(t, "refresh", want)
					select {
					case <-entered:
					case <-time.After(5 * time.Second):
						t.Fatal("refresh did not reach local OAuth server")
					}
					if mutation == "oauth" && order == "refresh-first" {
						other, _ := startCredentialProcess(t, "other-org", "ok")
						waitCredentialProcess(t, other)
						if token, err := kr.Get("other-org"); err != nil || token != "control-access" {
							t.Fatalf("unrelated organization write during refresh = %q, %v", token, err)
						}
					}
					// Each command must honor the refresh holder's lock, not just
					// serialize refresh against other refresh transports.
					blocked, _ := startCredentialProcess(t, mutation, "bound")
					waitCredentialProcess(t, blocked)
					resumeOnce.Do(func() { close(resume) })
					waitCredentialProcess(t, refresh)
					cmd, _ := startCredentialProcess(t, mutation, "ok")
					waitCredentialProcess(t, cmd)
					if n := exchanges.Load(); n != 1 {
						t.Fatalf("refresh exchanged %d times, want 1", n)
					}
				}
				access, accessErr := kr.Get("mutation-test")
				refresh, refreshErr := kr.GetRefreshToken("mutation-test")
				switch mutation {
				case "oauth":
					if accessErr != nil || refreshErr != nil || access != "replacement" || refresh != "refresh-replacement" {
						t.Fatalf("mixed OAuth credentials: %q/%q (%v, %v)", access, refresh, accessErr, refreshErr)
					}
				case "pat":
					if accessErr != nil || access != "replacement" || refresh != "" {
						t.Fatalf("mixed PAT credentials: %q/%q (%v, %v)", access, refresh, accessErr, refreshErr)
					}
				case "logout":
					if access != "" || refresh != "" {
						t.Fatalf("logout resurrected credentials: %q/%q", access, refresh)
					}
				}
			})
		}
	}
}

func startCredentialProcess(t *testing.T, role, mode string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCredentialMutationProcesses$", "-test.timeout=10s")
	cmd.Env = append(os.Environ(), "BK_AUTH_PROCESS_ROLE="+role, "BK_AUTH_PROCESS_MODE="+mode)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	gate, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gate.Close(); _ = cmd.Process.Kill() })
	if mode == "stage" {
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() || scanner.Text() != "holding" {
			t.Fatal("mutation did not reach storage barrier")
		}
	}
	return cmd, gate
}

func waitCredentialProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("credential process: %v", err)
	}
}

func runCredentialProcess(t *testing.T, role string) {
	t.Helper()
	mode := os.Getenv("BK_AUTH_PROCESS_MODE")
	// Use auto in mutation processes and explicit SHM in refresh processes.
	store := keyring.StoreAuto
	if role == "refresh" {
		store = keyring.StoreSHM
	}
	kr, err := keyring.NewWithCredentialStore(store)
	if err != nil {
		t.Fatal(err)
	}
	s := &stagedCredentialStore{Keyring: kr, stage: mode == "stage", bound: mode == "bound"}
	if role == "other-org" {
		release, err := kr.Lock(context.Background(), "other-org")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if err := kr.Set("other-org", "control-access"); err != nil {
			t.Fatal(err)
		}
		return
	}
	if role == "refresh" {
		der, err := base64.StdEncoding.DecodeString(os.Getenv("BK_AUTH_PROCESS_CERT"))
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
		ts := bkhttp.NewTokenSource("expired")
		endpoint := os.Getenv("BK_AUTH_PROCESS_ENDPOINT")
		client := &http.Client{Transport: &bkhttp.RefreshTransport{
			Base: &bkhttp.AuthTransport{TokenSource: ts}, Keyring: s, Org: "mutation-test",
			TokenSource: ts, APIEndpoints: []string{endpoint},
		}}
		resp, err := client.Get(endpoint + "/api")
		if mode == "bound" {
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("refresh did not wait for mutation: %v", err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		want := http.StatusOK
		if mode == "unauthorized" {
			want = http.StatusUnauthorized
		}
		if resp.StatusCode != want {
			t.Fatalf("refresh status = %d, want %d", resp.StatusCode, want)
		}
		return
	}
	f, err := factory.New()
	if err != nil {
		t.Fatal(err)
	}
	switch role {
	case "oauth":
		_, err = persistOAuthLogin(f, s, "mutation-test", "replacement", "refresh-replacement")
	case "pat":
		err = loginWithToken(f, "mutation-test", "replacement", s)
	case "logout":
		err = deleteCredentials(s, "mutation-test")
	default:
		t.Fatalf("unknown role %q", role)
	}
	if mode == "bound" {
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("mutation did not wait for refresh: %v", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
}
