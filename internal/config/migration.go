package config

import (
	"fmt"
)

// MigrateTokensToKeychain migrates all API tokens from the config file to the keychain
// Returns the number of tokens migrated and any error encountered
func (conf *Config) MigrateTokensToKeychain() (int, error) {
	if !shouldUseKeychain() {
		return 0, fmt.Errorf("keychain storage is not enabled")
	}

	// Get all organizations from the config file
	orgsMap := conf.userConfig.GetStringMap("organizations")
	if len(orgsMap) == 0 {
		return 0, nil // No organizations to migrate
	}

	migrated := 0
	keychainStorage := NewKeychainTokenStorage()

	for org := range orgsMap {
		// Get token from file
		key := fmt.Sprintf("organizations.%s.api_token", org)
		token := conf.userConfig.GetString(key)

		if token == "" {
			continue // No token for this org
		}

		// Store in keychain
		if err := keychainStorage.Set(org, token); err != nil {
			return migrated, fmt.Errorf("failed to migrate token for %q: %w", org, err)
		}

		migrated++
	}

	return migrated, nil
}

// RemoveTokensFromFile removes all API tokens from the config file after successful migration
// This should only be called after MigrateTokensToKeychain succeeds
func (conf *Config) RemoveTokensFromFile() error {
	orgsMap := conf.userConfig.GetStringMap("organizations")
	if len(orgsMap) == 0 {
		return nil
	}

	for org := range orgsMap {
		key := fmt.Sprintf("organizations.%s.api_token", org)
		if conf.userConfig.GetString(key) != "" {
			// Set to empty string to remove the token
			conf.userConfig.Set(key, "")
		}
	}

	return conf.userConfig.WriteConfig()
}

// HasFileTokens checks if there are any tokens stored in the config file
func (conf *Config) HasFileTokens() bool {
	orgsMap := conf.userConfig.GetStringMap("organizations")
	for org := range orgsMap {
		key := fmt.Sprintf("organizations.%s.api_token", org)
		if conf.userConfig.GetString(key) != "" {
			return true
		}
	}
	return false
}
