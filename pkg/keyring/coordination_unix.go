//go:build !windows

package keyring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var errCredentialLockHome = errors.New("credential coordination requires a persistent home directory from the OS account database")

// Variables so tests can substitute the account sources. getent is found at
// fixed system paths, not via PATH, so the lock location does not depend on the
// environment. The last entry is the NixOS system profile.
var (
	passwdFile    = "/etc/passwd"
	getentPaths   = []string{"/usr/bin/getent", "/run/current-system/sw/bin/getent"}
	getentTimeout = 10 * time.Second
)

func credentialLockDirectory(ctx context.Context) (string, error) {
	var home string
	if runtime.GOOS == "darwin" {
		// Darwin resolves the account through libc, including non-cgo builds.
		u, err := user.Current()
		if err != nil {
			return "", err
		}
		home = u.HomeDir
	} else {
		// Static Linux os/user falls back to HOME for unmapped UIDs. Resolve the
		// account directly instead; without one, fail before changing credentials.
		data, err := os.ReadFile(passwdFile)
		if err != nil {
			return "", err
		}
		uid := strconv.Itoa(os.Getuid())
		home = passwdHome(data, uid)
		if home == "" {
			// Directory-managed accounts (LDAP, SSSD) are only visible through
			// NSS, which a static binary cannot call. Ask getent instead.
			getent := ""
			for _, path := range getentPaths {
				// An absolute path is checked directly; PATH is not consulted.
				if _, err := exec.LookPath(path); err == nil {
					getent = path
					break
				}
			}
			if getent == "" {
				return "", fmt.Errorf("%w: getent not found at %s", errCredentialLockHome, strings.Join(getentPaths, ", "))
			}
			lookupCtx, cancel := context.WithTimeout(ctx, getentTimeout)
			defer cancel()
			cmd := exec.CommandContext(lookupCtx, getent, "passwd", uid)
			// An empty environment keeps LD_PRELOAD and similar from changing
			// the answer; NSS lookups do not need one.
			cmd.Env = []string{}
			cmd.WaitDelay = time.Second
			out, err := cmd.Output()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			if lookupCtx.Err() != nil {
				return "", fmt.Errorf("%w: getent passwd %s timed out after %s", errCredentialLockHome, uid, getentTimeout)
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
				// getent also exits 2 when the directory service is unreachable.
				return "", fmt.Errorf("%w: uid %s not resolved by getent passwd (account unknown or directory unavailable)", errCredentialLockHome, uid)
			}
			if err != nil {
				if exitErr != nil && len(bytes.TrimSpace(exitErr.Stderr)) > 0 {
					err = fmt.Errorf("%w: %s", err, bytes.TrimSpace(exitErr.Stderr))
				}
				return "", fmt.Errorf("%w: getent passwd %s: %v", errCredentialLockHome, uid, err)
			}
			home = passwdHome(out, uid)
		}
	}
	if !filepath.IsAbs(home) {
		return "", errCredentialLockHome
	}
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", err
	}
	// Normal temporary-file cleanup can unlink even a held lockfile.
	for _, temporary := range []string{"/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp"} {
		if strings.HasPrefix(home+"/", temporary+"/") {
			return "", errCredentialLockHome
		}
	}
	return filepath.Join(home, ".buildkite-credential-locks"), nil
}

// passwdHome returns the home directory of the passwd(5) entry for uid.
func passwdHome(data []byte, uid string) string {
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) == 7 && fields[2] == uid {
			return fields[5]
		}
	}
	return ""
}

func ensureLockDirectory(dir string) error {
	parent := filepath.Dir(dir)
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("credential lock parent must not be writable by others: %s", parent)
	}
	if err := validateCurrentUserOwner(parent, info); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err = os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("credential lock directory must be a private directory: %s", dir)
	}
	return validateCurrentUserOwner(dir, info)
}

func openLockFile(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Nlink != 1 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			err = fmt.Errorf("credential lock must be a private, regular file with one link: %s", path)
		} else {
			err = validateCurrentUserOwner(path, info)
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func tryLockFile(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}
