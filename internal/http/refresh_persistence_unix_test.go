//go:build !windows

package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/pkg/keyring"
)

// Expose entry into a context-bound storage wait, without replacing the real
// SHM file lock. Only the keyring's operation context is wrapped, not HTTP I/O.
type storageWaitContext struct {
	context.Context
	waiting chan struct{}
	enabled *atomic.Bool
}

func (c storageWaitContext) Done() <-chan struct{} {
	if c.enabled.Load() {
		select {
		case c.waiting <- struct{}{}:
		default:
		}
	}
	return c.Context.Done()
}

type observedPersistenceStore struct {
	*keyring.Keyring
	waiting chan struct{}
	enabled *atomic.Bool
}

func (s *observedPersistenceStore) Lock(ctx context.Context, org string) (func(), error) {
	return s.Keyring.Lock(storageWaitContext{ctx, s.waiting, s.enabled}, org)
}

func (s *observedPersistenceStore) SetTokens(ctx context.Context, org, accessToken, refreshToken string) error {
	return s.Keyring.SetTokens(storageWaitContext{ctx, s.waiting, s.enabled}, org, accessToken, refreshToken)
}

func TestRefreshPersistenceCancellation(t *testing.T) {
	for _, phase := range []string{"before-exchange", "after-exchange"} {
		t.Run(phase, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private", "credentials.json")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreSHM)
			t.Setenv(keyring.CredentialStorePathEnv, path)
			kr := keyring.New()
			if err := kr.Set("cancel-rotation", "expired"); err != nil {
				t.Fatal(err)
			}
			if err := kr.SetRefreshToken("cancel-rotation", "refresh-original"); err != nil {
				t.Fatal(err)
			}
			file, err := os.OpenFile(filepath.Join(filepath.Dir(path), ".credentials.lock"), os.O_RDWR, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			hold := func() error { return syscall.Flock(int(file.Fd()), syscall.LOCK_EX) }
			var enabled atomic.Bool
			if phase == "before-exchange" {
				if err := hold(); err != nil {
					t.Fatal(err)
				}
				enabled.Store(true)
			}
			var exchanges atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/oauth/token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if err := r.ParseForm(); err != nil || r.Form.Get("refresh_token") != "refresh-original" || exchanges.Add(1) != 1 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
					return
				}
				// The exchange invalidates refresh-original. Contend with the next
				// storage write, after CheckWritable succeeded, before replying.
				if err := hold(); err != nil {
					t.Errorf("hold SHM storage lock: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				enabled.Store(true)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"refresh-rotated"}`)
			}))
			defer server.Close()
			original := http.DefaultTransport
			http.DefaultTransport = server.Client().Transport
			defer func() { http.DefaultTransport = original }()
			t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))
			waiting := make(chan struct{}, 1)
			source := NewTokenSource("expired")
			client := &http.Client{Transport: &RefreshTransport{
				Base:    &AuthTransport{Base: server.Client().Transport, TokenSource: source},
				Keyring: &observedPersistenceStore{kr, waiting, &enabled}, Org: "cancel-rotation",
				TokenSource: source, APIEndpoints: []string{server.URL},
			}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api", nil)
			done := make(chan error, 1)
			go func() {
				resp, err := client.Do(req)
				if resp != nil {
					_ = resp.Body.Close()
				}
				done <- err
			}()
			select {
			case <-waiting:
			case <-time.After(5 * time.Second):
				t.Fatal("refresh did not reach the contended storage wait")
			}
			cancel()
			if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("request cancellation = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("refresh did not finish after storage lock release")
			}
			wantAccess, wantRefresh, wantExchanges := "fresh", "refresh-rotated", int32(1)
			if phase == "before-exchange" {
				wantAccess, wantRefresh, wantExchanges = "expired", "refresh-original", 0
			}
			access, accessErr := kr.Get("cancel-rotation")
			refresh, refreshErr := kr.GetRefreshToken("cancel-rotation")
			if accessErr != nil || refreshErr != nil || access != wantAccess || refresh != wantRefresh || exchanges.Load() != wantExchanges {
				t.Fatalf("stored pair = %q/%q (%v, %v), exchanges = %d; want %q/%q and %d exchanges", access, refresh, accessErr, refreshErr, exchanges.Load(), wantAccess, wantRefresh, wantExchanges)
			}
			ctx, cancel = context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			release, err := kr.Lock(ctx, "cancel-rotation")
			if err != nil {
				t.Fatalf("organization lock was not released: %v", err)
			}
			release()
		})
	}
}
