package config

import (
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const (
	// KeychainServiceName is the service name used when storing tokens in the system keychain
	KeychainServiceName = "com.buildkite.cli"

	// EnvVarTokenStorage allows users to override token storage mechanism
	// Valid values: "keychain" (default), "file"
	EnvVarTokenStorage = "BUILDKITE_TOKEN_STORAGE"
)

// TokenStorage defines the interface for storing and retrieving API tokens
type TokenStorage interface {
	Get(org string) (string, error)
	Set(org, token string) error
	Delete(org string) error
	List() ([]string, error)
}

// KeychainTokenStorage stores tokens in the system keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service)
type KeychainTokenStorage struct {
	serviceName string
}

// NewKeychainTokenStorage creates a new keychain-based token storage
func NewKeychainTokenStorage() *KeychainTokenStorage {
	return &KeychainTokenStorage{
		serviceName: KeychainServiceName,
	}
}

// Get retrieves a token for the given organization from the keychain
func (k *KeychainTokenStorage) Get(org string) (string, error) {
	token, err := keyring.Get(k.serviceName, org)
	if err == keyring.ErrNotFound {
		return "", fmt.Errorf("no token found for organization %q", org)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get token from keychain: %w", err)
	}
	return token, nil
}

// Set stores a token for the given organization in the keychain
func (k *KeychainTokenStorage) Set(org, token string) error {
	if err := keyring.Set(k.serviceName, org, token); err != nil {
		return fmt.Errorf("failed to set token in keychain: %w", err)
	}
	return nil
}

// Delete removes a token for the given organization from the keychain
func (k *KeychainTokenStorage) Delete(org string) error {
	if err := keyring.Delete(k.serviceName, org); err != nil {
		return fmt.Errorf("failed to delete token from keychain: %w", err)
	}
	return nil
}

// List returns all organizations that have tokens stored in the keychain
// Note: This is not directly supported by the keychain API, so we'll return an empty list
// and rely on the file-based config for listing organizations
func (k *KeychainTokenStorage) List() ([]string, error) {
	return []string{}, nil
}

// FileTokenStorage stores tokens in the configuration file (legacy/fallback mode)
type FileTokenStorage struct {
	conf *Config
}

// NewFileTokenStorage creates a new file-based token storage
func NewFileTokenStorage(conf *Config) *FileTokenStorage {
	return &FileTokenStorage{
		conf: conf,
	}
}

// Get retrieves a token for the given organization from the config file
func (f *FileTokenStorage) Get(org string) (string, error) {
	key := fmt.Sprintf("organizations.%s.api_token", org)
	token := f.conf.userConfig.GetString(key)
	if token == "" {
		return "", fmt.Errorf("no token found for organization %q", org)
	}
	return token, nil
}

// Set stores a token for the given organization in the config file
func (f *FileTokenStorage) Set(org, token string) error {
	key := fmt.Sprintf("organizations.%s.api_token", org)
	f.conf.userConfig.Set(key, token)
	return f.conf.userConfig.WriteConfig()
}

// Delete removes a token for the given organization from the config file
func (f *FileTokenStorage) Delete(org string) error {
	key := fmt.Sprintf("organizations.%s.api_token", org)
	f.conf.userConfig.Set(key, nil)
	return f.conf.userConfig.WriteConfig()
}

// List returns all organizations that have tokens stored in the config file
func (f *FileTokenStorage) List() ([]string, error) {
	orgsMap := f.conf.userConfig.GetStringMap("organizations")
	orgs := make([]string, 0, len(orgsMap))
	for org := range orgsMap {
		// Check if this org actually has a token
		key := fmt.Sprintf("organizations.%s.api_token", org)
		if f.conf.userConfig.GetString(key) != "" {
			orgs = append(orgs, org)
		}
	}
	return orgs, nil
}

// getTokenStorageBackend determines which token storage backend to use based on environment variables
func getTokenStorageBackend() string {
	backend := os.Getenv(EnvVarTokenStorage)
	if backend == "" {
		return "keychain" // Default to keychain
	}
	return backend
}

// ShouldUseKeychain returns true if keychain storage should be used
func ShouldUseKeychain() bool {
	return getTokenStorageBackend() == "keychain"
}

// Deprecated: use ShouldUseKeychain instead
func shouldUseKeychain() bool {
	return ShouldUseKeychain()
}
