//go:build !windows

package keyring

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestCredentialLockUnmappedUID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("static Linux account lookup regression")
	}
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) == 7 && fields[2] == strconv.Itoa(os.Getuid()) {
			t.Skip("run under an unmapped numeric UID to exercise fail-closed behavior")
		}
	}
	t.Setenv(CredentialStorePathEnv, filepath.Join(t.TempDir(), "private", "credentials.json"))
	kr, _ := NewWithCredentialStore(StoreSHM)
	if err := kr.Set("unmapped-test", "synthetic"); err != nil {
		t.Fatal(err)
	}
	for _, home := range []string{t.TempDir(), t.TempDir()} {
		t.Setenv("HOME", home)
		release, err := kr.Lock(context.Background(), "unmapped-test")
		if !errors.Is(err, errCredentialLockHome) {
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
