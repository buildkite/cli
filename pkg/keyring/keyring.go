// Package keyring provides secure credential storage using the OS keychain.
package keyring

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/buildkite/cli/v3/pkg/oauth"
	"github.com/zalando/go-keyring"
)

const (
	serviceName = "buildkite-cli"
)

var (
	keyringAvailableOnce sync.Once
	keyringAvailable     bool
)

// Keyring provides secure credential storage.
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
		return nil
	}
	return keyring.Set(serviceName, org, token)
}

// Get retrieves a token for the given organization
func (k *Keyring) Get(org string) (string, error) {
	session, err := k.GetSession(org)
	if err != nil {
		return "", err
	}
	return session.AccessToken, nil
}

// SetSession stores an OAuth session for the given organization.
func (k *Keyring) SetSession(org string, session *oauth.Session) error {
	if !k.useKeyring {
		return nil
	}

	encoded, err := json.Marshal(session)
	if err != nil {
		return err
	}

	return keyring.Set(serviceName, org, string(encoded))
}

// GetSession retrieves an OAuth session for the given organization.
// Legacy plaintext tokens are returned as access-token-only sessions.
func (k *Keyring) GetSession(org string) (*oauth.Session, error) {
	if !k.useKeyring {
		return nil, keyring.ErrNotFound
	}

	stored, err := keyring.Get(serviceName, org)
	if err != nil {
		return nil, err
	}

	var session oauth.Session
	if err := json.Unmarshal([]byte(stored), &session); err == nil && session.AccessToken != "" {
		if session.Version == 0 {
			session.Version = oauth.SessionVersion
		}
		return &session, nil
	}

	return &oauth.Session{
		Version:     oauth.SessionVersion,
		AccessToken: stored,
		TokenType:   "Bearer",
	}, nil
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
// and marks it as available so subsequent New() calls use the mock.
func MockForTesting() {
	keyring.MockInit()
	keyringAvailableOnce = sync.Once{}
	keyringAvailableOnce.Do(func() {
		keyringAvailable = true
	})
}

// isKeyringAvailable checks if the system keyring can be used
func isKeyringAvailable() bool {
	keyringAvailableOnce.Do(func() {
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
