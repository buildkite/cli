// Package config provides commands for managing CLI configuration
package config

import (
	"fmt"
	"slices"
)

// ConfigKey represents a valid configuration key
type ConfigKey string

const (
	KeySelectedOrg  ConfigKey = "selected_org"
	KeyOutputFormat ConfigKey = "output_format"
	KeyNoPager      ConfigKey = "no_pager"
	KeyQuiet        ConfigKey = "quiet"
	KeyNoInput      ConfigKey = "no_input"
	KeyPager        ConfigKey = "pager"
)

// AllKeys returns all valid configuration keys
func AllKeys() []ConfigKey {
	return []ConfigKey{
		KeySelectedOrg,
		KeyOutputFormat,
		KeyNoPager,
		KeyQuiet,
		KeyNoInput,
		KeyPager,
	}
}

// ValidateKey checks if a key is valid
func ValidateKey(key string) (ConfigKey, error) {
	k := ConfigKey(key)
	if slices.Contains(AllKeys(), k) {
		return k, nil
	}
	return "", fmt.Errorf("unknown config key: %s\nvalid keys: %v", key, AllKeys())
}

// IsLocalOnly returns true if the key can only be set in user config
func (k ConfigKey) IsLocalOnly() bool {
	return false
}

// IsUserOnly returns true if the key can only be set in user config
func (k ConfigKey) IsUserOnly() bool {
	switch k {
	case KeyNoInput, KeyPager:
		return true
	default:
		return false
	}
}

// IsBool returns true if the key is a boolean value
func (k ConfigKey) IsBool() bool {
	switch k {
	case KeyNoPager, KeyQuiet, KeyNoInput:
		return true
	default:
		return false
	}
}

// ValidValues returns valid values for enum keys, or nil if any value is valid
func (k ConfigKey) ValidValues() []string {
	switch k {
	case KeyOutputFormat:
		return []string{"json", "yaml", "text"}
	case KeyNoPager, KeyQuiet, KeyNoInput:
		return []string{"true", "false"}
	default:
		return nil
	}
}
