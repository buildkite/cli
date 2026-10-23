package keyring

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var credentialLocks sync.Map // lock path -> buffered channel

// Lock serializes credential mutation for an organization, including refresh
// exchanges. Callers must release it before returning and must not acquire it
// recursively. Individual storage operations retain their short file locks.
func (k *Keyring) Lock(ctx context.Context, org string) (func(), error) {
	dir, err := credentialLockDirectory()
	if err != nil {
		return nil, fmt.Errorf("find credential lock directory: %w", err)
	}
	// Auto and explicit stores can share credentials, regardless of repository,
	// config, API endpoints, or environment-selected storage paths.
	release, err := lockCredentials(ctx, dir, org)
	if err != nil {
		return nil, err
	}
	// Backend preference may have changed while this process was waiting.
	k.lastStore.Store("")
	k.lockContext.Store(&ctx)
	return func() {
		k.lockContext.Store(nil)
		release()
	}, nil
}

func (k *Keyring) credentialContext() context.Context {
	if ctx := k.lockContext.Load(); ctx != nil {
		return *ctx
	}
	return context.Background()
}

func lockCredentials(ctx context.Context, dir, org string) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if org == "" {
		return nil, errors.New("organization cannot be empty")
	}
	if err := ensureLockDirectory(dir); err != nil {
		return nil, err
	}
	// Windows keyring credential names are case-insensitive. Coordinate casing
	// aliases across all backends without changing their stored keys.
	path := filepath.Join(dir, fmt.Sprintf("%x.lock", sha256.Sum256([]byte(strings.ToLower(org)))))
	// OS file locks do not establish Go memory ordering for callers sharing
	// objects in this process. Keep an equivalent cancellable local guard.
	value, _ := credentialLocks.LoadOrStore(path, make(chan struct{}, 1))
	local := value.(chan struct{})
	select {
	case local <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	file, err := openLockFile(path)
	if err != nil {
		<-local
		return nil, err
	}
	if err := waitForLock(ctx, file); err != nil {
		_ = file.Close()
		<-local
		return nil, fmt.Errorf("lock credentials: %w", err)
	}
	// Never remove lock files: unlinking would allow waiters to lock a different
	// inode from a newly started process. Closing also releases on process exit.
	return func() { _ = file.Close(); <-local }, nil
}

func waitForLock(ctx context.Context, file *os.File) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		locked, err := tryLockFile(file)
		if err != nil || locked {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
