// Package keyring provides secure credential storage using the OS keychain.
// It falls back to file-based storage when the keychain is unavailable (e.g., in CI environments).
package keyring

import (
	"os"
	"sync"

	"github.com/zalando/go-keyring"
)

const (
	serviceName        = "buildkite-cli"
	refreshServiceName = "buildkite-cli-refresh"
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

// SetRefreshToken stores a refresh token for the given organization
func (k *Keyring) SetRefreshToken(org, token string) error {
	if !k.useKeyring {
		return nil
	}
	return keyring.Set(refreshServiceName, org, token)
}

// GetRefreshToken retrieves a refresh token for the given organization
func (k *Keyring) GetRefreshToken(org string) (string, error) {
	if !k.useKeyring {
		return "", keyring.ErrNotFound
	}
	return keyring.Get(refreshServiceName, org)
}

// DeleteRefreshToken removes a refresh token for the given organization
func (k *Keyring) DeleteRefreshToken(org string) error {
	if !k.useKeyring {
		return nil
	}
	return keyring.Delete(refreshServiceName, org)
}

// IsAvailable returns true if the system keyring is available
func (k *Keyring) IsAvailable() bool {
	return k.useKeyring
}

// MockForTesting replaces the keyring backend with an in-memory store
// and marks it as available so subsequent New() calls use the mock.
func MockForTesting() {
	keyring.MockInit()
	keyringAvailableOnce = sync.Once{}
	keyringAvailableOnce.Do(func() {
		keyringAvailable = true
	})
}

// ResetForTesting resets the availability cache so that the next call to
// New() re-evaluates the environment. Intended for use in tests only.
func ResetForTesting() {
	keyringAvailableOnce = sync.Once{}
	keyringAvailable = false
}

// isKeyringAvailable checks if the system keyring can be used
func isKeyringAvailable() bool {
	keyringAvailableOnce.Do(func() {
		// Disable keyring if explicitly opted out
		if os.Getenv("BUILDKITE_NO_KEYRING") != "" {
			keyringAvailable = false
			return
		}

		// Disable keyring in CI environments
		if os.Getenv("CI") != "" || os.Getenv("BUILDKITE") != "" {
			keyringAvailable = false
			return
		}

		// Assume keyring is available; callers can handle errors
		keyringAvailable = true
	})
	return keyringAvailable
}
