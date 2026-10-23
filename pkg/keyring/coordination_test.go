package keyring

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	oskeyring "github.com/zalando/go-keyring"
)

func TestCredentialLockReloadsBackendPreference(t *testing.T) {
	t.Setenv(CredentialStorePathEnv, filepath.Join(t.TempDir(), "private", "credentials.json"))
	MockForTesting()
	t.Cleanup(ResetForTesting)
	shm, _ := NewWithCredentialStore(StoreSHM)
	if err := shm.Set("preference-test", "old-access"); err != nil {
		t.Fatal(err)
	}
	if err := shm.SetRefreshToken("preference-test", "old-refresh"); err != nil {
		t.Fatal(err)
	}
	auto, _ := NewWithCredentialStore(StoreAuto)
	if token, err := auto.Get("preference-test"); err != nil || token != "old-access" {
		t.Fatalf("initial SHM credential = %q, %v", token, err)
	}
	replacement, _ := NewWithCredentialStore(StoreKeyring)
	release, err := replacement.Lock(context.Background(), "preference-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := replacement.SetRefreshToken("preference-test", "new-refresh"); err != nil {
		release()
		t.Fatal(err)
	}
	if err := replacement.Set("preference-test", "new-access"); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	release, err = auto.Lock(context.Background(), "preference-test")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if access, err := auto.Get("preference-test"); err != nil || access != "new-access" {
		t.Fatalf("replacement access = %q, %v", access, err)
	}
	if refresh, err := auto.GetRefreshToken("preference-test"); err != nil || refresh != "new-refresh" {
		t.Fatalf("replacement refresh = %q, %v", refresh, err)
	}
	for _, service := range []string{serviceName, refreshServiceName} {
		if _, err := shmBackend.Get(service, "preference-test"); !errors.Is(err, oskeyring.ErrNotFound) {
			t.Fatalf("stale SHM credential remains: %v", err)
		}
		if preferred, err := shmBackend.Preferred(service, "preference-test"); err != nil || preferred {
			t.Fatalf("stale SHM preference remains: %v", err)
		}
	}
}

func TestCredentialLockProcess(t *testing.T) {
	if dir := os.Getenv("BK_LOCK_PROCESS_DIR"); dir != "" {
		var release func()
		var err error
		if os.Getenv("BK_LOCK_PROCESS_SHM") != "" {
			var file *os.File
			file, err = openLockFile(shmCredentialLockPath(shmCredentialPath()))
			if err == nil {
				err = waitForLock(context.Background(), file)
				release = func() { _ = file.Close() }
			}
		} else if os.Getenv("BK_LOCK_PROCESS_PUBLIC") != "" {
			kr, _ := NewWithCredentialStore(StoreAuto)
			if token, err := kr.Get("environment-identity-test"); err != nil || token != "synthetic" {
				t.Fatalf("shared synthetic credential = %q, %v", token, err)
			}
			release, err = kr.Lock(context.Background(), "environment-identity-test")
		} else {
			release, err = lockCredentials(context.Background(), dir, "held-org")
		}
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		fmt.Println("locked")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}

	dir := filepath.Join(t.TempDir(), "locks")
	t.Setenv("BK_LOCK_PROCESS_DIR", dir)
	cmd := startLockHolder(t)

	// A different organization must proceed while the holder is alive.
	release, err := lockCredentials(context.Background(), dir, "other-org")
	if err != nil {
		t.Fatal(err)
	}
	release()

	for _, org := range []string{"held-org", "HELD-ORG"} {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if release, err := lockCredentials(ctx, dir, org); !errors.Is(err, context.DeadlineExceeded) {
			if release != nil {
				release()
			}
			t.Fatalf("wait for held lock alias %q = %v, want deadline exceeded", org, err)
		}
	}
	ctx, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err := lockCredentials(ctx, dir, "held-org"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait = %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	ctx, cancelAfterExit := context.WithTimeout(context.Background(), time.Second)
	defer cancelAfterExit()
	release, err = lockCredentials(ctx, dir, "held-org")
	if err != nil {
		t.Fatalf("lock after holder termination: %v", err)
	}
	release()
}

func TestCredentialLockEnvironmentIdentity(t *testing.T) {
	t.Setenv(CredentialStorePathEnv, filepath.Join(t.TempDir(), "private", "credentials.json"))
	t.Setenv("BUILDKITE_NO_KEYRING", "1")
	kr, _ := NewWithCredentialStore(StoreSHM)
	if err := kr.Set("environment-identity-test", "synthetic"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BK_LOCK_PROCESS_DIR", "public-lock")
	t.Setenv("BK_LOCK_PROCESS_PUBLIC", "1")
	first, second := t.TempDir(), t.TempDir()
	t.Setenv("HOME", first)
	t.Setenv("TMPDIR", first)
	t.Setenv("USERPROFILE", first)
	startLockHolder(t)
	// Model an age-based temporary cleaner unlinking this org's eligible lock
	// while its descriptor is held. Persistent account locks are out of scope
	// for that cleaner and must continue to exclude the next process.
	dir, err := credentialLockDirectory()
	if err != nil {
		t.Fatal(err)
	}
	for _, temporary := range []string{"/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp"} {
		if strings.HasPrefix(dir+"/", temporary+"/") {
			path := filepath.Join(dir, fmt.Sprintf("%x.lock", sha256.Sum256([]byte("environment-identity-test"))))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv("HOME", second)
	t.Setenv("TMPDIR", second)
	t.Setenv("USERPROFILE", second)
	t.Setenv("XDG_CONFIG_HOME", second)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	release, err := kr.Lock(ctx, "environment-identity-test")
	if !errors.Is(err, context.DeadlineExceeded) {
		if release != nil {
			release()
		}
		t.Fatalf("processes sharing credentials bypassed coordination with different environments: %v", err)
	}
}

func TestCredentialLockCancelsNestedStorageWait(t *testing.T) {
	t.Setenv(CredentialStorePathEnv, filepath.Join(t.TempDir(), "private", "credentials.json"))
	kr, _ := NewWithCredentialStore(StoreSHM)
	if err := kr.Set("nested-wait", "synthetic"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BK_LOCK_PROCESS_DIR", "storage-lock")
	t.Setenv("BK_LOCK_PROCESS_SHM", "1")
	startLockHolder(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	release, err := kr.Lock(ctx, "nested-wait")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := kr.CheckWritable(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("storage wait ignored credential operation context: %v", err)
	}
}

func startLockHolder(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCredentialLockProcess$", "-test.timeout=15s")
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "locked" {
		t.Fatalf("holder did not acquire lock: %v", scanner.Err())
	}
	return cmd
}

func TestSHMConcurrentProcesses(t *testing.T) {
	if org := os.Getenv("BK_SHM_PROCESS_ORG"); org != "" {
		fmt.Println("ready")
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			t.Fatal(err)
		}
		kr, err := NewWithCredentialStore(StoreSHM)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 12; i++ {
			if err := kr.Set(org, fmt.Sprintf("access-%d", i)); err != nil {
				t.Fatal(err)
			}
			if err := kr.SetRefreshToken(org, fmt.Sprintf("refresh-%d", i)); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	t.Setenv(CredentialStorePathEnv, filepath.Join(t.TempDir(), "private", "credentials.json"))
	var commands []*exec.Cmd
	var gates []io.WriteCloser
	for _, org := range []string{"first-org", "second-org", "third-org"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSHMConcurrentProcesses$", "-test.timeout=15s")
		cmd.Env = append(os.Environ(), "BK_SHM_PROCESS_ORG="+org)
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
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		if scanner := bufio.NewScanner(stdout); !scanner.Scan() || scanner.Text() != "ready" {
			t.Fatal("writer not ready")
		}
		commands = append(commands, cmd)
		gates = append(gates, gate)
	}
	for _, gate := range gates {
		_ = gate.Close()
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	for _, org := range []string{"first-org", "second-org", "third-org"} {
		for service, want := range map[string]string{serviceName: "access-11", refreshServiceName: "refresh-11"} {
			if got, err := shmBackend.Get(service, org); err != nil || got != want {
				t.Fatalf("%s/%s = %q, %v", service, org, got, err)
			}
			if preferred, err := shmBackend.Preferred(service, org); err != nil || !preferred {
				t.Fatalf("lost preference for %s/%s: %v", service, org, err)
			}
		}
	}
}
