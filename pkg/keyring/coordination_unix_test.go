//go:build !windows

package keyring

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// unmappedAccount models a UID missing from /etc/passwd whose NSS lookup
// prints output, then runs the shell commands in tail.
func unmappedAccount(t *testing.T, output, tail string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("static Linux account lookup regression")
	}
	dir := t.TempDir()
	passwd, out, args, getent := filepath.Join(dir, "passwd"), filepath.Join(dir, "out"), filepath.Join(dir, "args"), filepath.Join(dir, "getent")
	script := fmt.Sprintf("#!/bin/sh\necho \"$*|${BK_GETENT_LEAK-}\" > %q\ncat %q\n%s\n", args, out, tail)
	// Runners may mount the temp directory noexec.
	if err := os.WriteFile(getent, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command(getent).Run(); errors.Is(err, os.ErrPermission) {
		t.Skipf("cannot run getent stub: %v", err)
	}
	for path, data := range map[string]string{passwd: "", out: output, getent: script} {
		if err := os.WriteFile(path, []byte(data), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// The lookup must not see the caller's environment.
	t.Setenv("BK_GETENT_LEAK", "leaked")
	oldPasswd, oldGetent := passwdFile, getentPaths
	// A missing first candidate proves the lookup moves on to the next path.
	passwdFile, getentPaths = passwd, []string{filepath.Join(dir, "missing"), getent}
	t.Cleanup(func() {
		passwdFile, getentPaths = oldPasswd, oldGetent
		// Every failure looks the same, so prove the lookup actually ran.
		if got, err := os.ReadFile(args); err != nil || string(got) != "passwd "+strconv.Itoa(os.Getuid())+"|\n" {
			t.Errorf("getent stub not run as expected: %q, %v", got, err)
		}
	})
}

func TestCredentialLockUnmappedUID(t *testing.T) {
	// Not found in NSS either.
	unmappedAccount(t, "", "exit 2")
	t.Setenv(CredentialStorePathEnv, filepath.Join(t.TempDir(), "private", "credentials.json"))
	kr, _ := NewWithCredentialStore(StoreSHM)
	if err := kr.Set("unmapped-test", "synthetic"); err != nil {
		t.Fatal(err)
	}
	for _, home := range []string{t.TempDir(), t.TempDir()} {
		t.Setenv("HOME", home)
		release, err := kr.Lock(context.Background(), "unmapped-test")
		if !errors.Is(err, errCredentialLockHome) || !strings.Contains(err.Error(), "not resolved by getent") {
			if release != nil {
				release()
			}
			t.Fatalf("unmapped UID must fail closed independent of HOME: %v", err)
		}
	}
	if token, err := kr.Get("unmapped-test"); err != nil || token != "synthetic" {
		t.Fatalf("failed coordination changed credentials: %q, %v", token, err)
	}
}

func TestCredentialLockNSSAccount(t *testing.T) {
	uid := strconv.Itoa(os.Getuid())
	// The lock home must be persistent, so use the checkout rather than a
	// temporary directory. Resolving the lock path creates nothing in it.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	entry := func(id string) string { return "ldapuser:x:" + id + ":" + id + "::" + home + ":/bin/sh\n" }

	t.Run("found", func(t *testing.T) {
		for _, temporary := range []string{"/tmp/", "/var/tmp/"} {
			if strings.HasPrefix(home+"/", temporary) {
				t.Skip("checkout is in a temporary directory, which lock homes reject")
			}
		}
		unmappedAccount(t, entry(uid), "")
		t.Setenv("HOME", t.TempDir())
		dir, err := credentialLockDirectory(context.Background())
		if err != nil || dir != filepath.Join(home, ".buildkite-credential-locks") {
			t.Fatalf("NSS account home not used: %q, %v", dir, err)
		}
	})
	for name, output := range map[string]string{
		"other-uid":     entry(uid + "0"),
		"relative-home": "ldapuser:x:" + uid + ":" + uid + "::home:/bin/sh\n",
		"malformed":     "ldapuser:x:" + uid + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			unmappedAccount(t, output, "")
			if _, err := credentialLockDirectory(context.Background()); !errors.Is(err, errCredentialLockHome) {
				t.Fatalf("unusable NSS entry must fail closed: %v", err)
			}
		})
	}
	t.Run("no-getent", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("static Linux account lookup regression")
		}
		dir := t.TempDir()
		passwd := filepath.Join(dir, "passwd")
		if err := os.WriteFile(passwd, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		oldPasswd, oldGetent := passwdFile, getentPaths
		passwdFile, getentPaths = passwd, []string{filepath.Join(dir, "missing")}
		t.Cleanup(func() { passwdFile, getentPaths = oldPasswd, oldGetent })
		_, err := credentialLockDirectory(context.Background())
		if !errors.Is(err, errCredentialLockHome) || !strings.Contains(err.Error(), "getent not found") {
			t.Fatalf("missing getent must fail closed: %v", err)
		}
	})
	t.Run("getent-error", func(t *testing.T) {
		unmappedAccount(t, "", "echo 'unknown database' >&2; exit 1")
		_, err := credentialLockDirectory(context.Background())
		if !errors.Is(err, errCredentialLockHome) || !strings.Contains(err.Error(), "exit status 1: unknown database") {
			t.Fatalf("getent failure must fail closed with its stderr: %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		unmappedAccount(t, entry(uid), "exec sleep 10")
		oldTimeout := getentTimeout
		getentTimeout = time.Second
		t.Cleanup(func() { getentTimeout = oldTimeout })
		_, err := credentialLockDirectory(context.Background())
		if !errors.Is(err, errCredentialLockHome) || !strings.Contains(err.Error(), "timed out after 1s") {
			t.Fatalf("hung lookup must fail closed as a timeout: %v", err)
		}
	})
	t.Run("caller-cancelled", func(t *testing.T) {
		// A slow lookup that the caller abandons reports cancellation, not a
		// missing account. The forked sleep keeps stdout open after the kill,
		// so returning well before it exits proves WaitDelay bounds the lookup.
		unmappedAccount(t, entry(uid), "sleep 10")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		start := time.Now()
		if _, err := credentialLockDirectory(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("caller cancellation not reported: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("lookup outlived its deadline by waiting on the child's output: %v", elapsed)
		}
	})
}

func TestCredentialLockRejectsUnsafePaths(t *testing.T) {
	for _, kind := range []string{"directory-symlink", "public-directory", "public-parent", "file-symlink", "public-file", "hard-link", "fifo", "wrong-owner", "unwritable"} {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "locks")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, fmt.Sprintf("%x.lock", sha256.Sum256([]byte("org"))))
			target := filepath.Join(t.TempDir(), "target")
			if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "directory-symlink":
				err = os.Symlink(dir, dir+"-link")
				dir += "-link"
			case "public-directory":
				err = os.Chmod(dir, 0o777)
			case "public-parent":
				err = os.Chmod(filepath.Dir(dir), 0o777)
			case "file-symlink":
				err = os.Symlink(target, path)
			case "public-file":
				err = os.WriteFile(path, nil, 0o644)
			case "hard-link":
				err = os.Link(target, path)
			case "fifo":
				err = syscall.Mkfifo(path, 0o600)
			case "wrong-owner":
				if os.Getuid() != 0 {
					t.Skip("changing file ownership requires root")
				}
				err = os.Chown(dir, 12345, -1)
				t.Cleanup(func() { _ = os.Chown(dir, 0, -1) })
			case "unwritable":
				// A regular file cannot contain the lock directory, even as root.
				dir = filepath.Join(target, "locks")
			}
			if err != nil {
				t.Fatal(err)
			}
			release, err := lockCredentials(context.Background(), dir, "org")
			if err == nil {
				release()
				t.Fatal("unsafe lock path accepted")
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != "untouched" {
				t.Fatalf("lock changed target: %q, %v", data, err)
			}
		})
	}
}
