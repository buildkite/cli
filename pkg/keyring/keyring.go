// Package keyring provides secure credential storage using the OS keychain.
// It falls back to file-based storage when the keychain is unavailable (e.g., in CI environments).
package keyring

import (
	"crypto/rand"
	"fmt"
	"os"
	"sync"

	"github.com/zalando/go-keyring"
)

const (
	serviceName = "buildkite-cli"
)

var (
	keyringAvailableOnce sync.Once
	keyringAvailable     bool
)

// Keyring provides secure credential storage with fallback support
type Keyring struct {
	useKeyring bool
}

// New creates a new Keyring instance.
// It automatically detects if the system keyring is available.
func New() *Keyring {
	return &Keyring{
		useKeyring: isKeyringAvailable(),
	}
}

// Set stores a token for the given organization
func (k *Keyring) Set(org, token string) error {
	if !k.useKeyring {
		return nil // Fallback handled by config file
	}
	return keyring.Set(serviceName, org, token)
}

// Get retrieves a token for the given organization
func (k *Keyring) Get(org string) (string, error) {
	if !k.useKeyring {
		return "", keyring.ErrNotFound
	}
	return keyring.Get(serviceName, org)
}

// Delete removes a token for the given organization
func (k *Keyring) Delete(org string) error {
	if !k.useKeyring {
		return nil
	}
	return keyring.Delete(serviceName, org)
}

// IsAvailable returns true if the system keyring is available
func (k *Keyring) IsAvailable() bool {
	return k.useKeyring
}

// MockForTesting replaces the keyring backend with an in-memory store
// and resets the availability cache so subsequent New() calls reflect
// the mock. 
func MockForTesting() {
	keyring.MockInit()
	keyringAvailableOnce = sync.Once{}
	keyringAvailable = false
}

// isKeyringAvailable checks if the system keyring can be used
func isKeyringAvailable() bool {
	keyringAvailableOnce.Do(func() {
		// Disable keyring in CI environments
		if os.Getenv("CI") != "" || os.Getenv("BUILDKITE") != "" {
			keyringAvailable = false
			return
		}

		// Test if keyring is functional by attempting a dummy operation.
		// Use a random suffix so a failed delete won't leave a colliding entry.
		var buf [4]byte
		_, _ = rand.Read(buf[:])
		testKey := fmt.Sprintf("buildkite-cli-keyring-test-%x", buf)
		err := keyring.Set(serviceName, testKey, "test")
		if err != nil {
			keyringAvailable = false
			return
		}
		_ = keyring.Delete(serviceName, testKey)
		keyringAvailable = true
	})
	return keyringAvailable
}
