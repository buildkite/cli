package config

import (
	"testing"
)

func TestValidateKey(t *testing.T) {
	t.Parallel()

	t.Run("valid keys", func(t *testing.T) {
		t.Parallel()

		validKeys := []string{
			"selected_org",
			"output_format",
			"no_pager",
			"quiet",
			"no_input",
			"pager",
		}

		for _, key := range validKeys {
			t.Run(key, func(t *testing.T) {
				got, err := ValidateKey(key)
				if err != nil {
					t.Errorf("ValidateKey(%q) returned error: %v", key, err)
				}
				if string(got) != key {
					t.Errorf("ValidateKey(%q) = %q, want %q", key, got, key)
				}
			})
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		t.Parallel()

		_, err := ValidateKey("invalid_key")
		if err == nil {
			t.Error("ValidateKey(\"invalid_key\") expected error, got nil")
		}
	})
}

func TestConfigKeyIsBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key    ConfigKey
		isBool bool
	}{
		{KeyNoPager, true},
		{KeyQuiet, true},
		{KeyNoInput, true},
		{KeyOutputFormat, false},
		{KeySelectedOrg, false},
		{KeyPager, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.key), func(t *testing.T) {
			t.Parallel()
			if got := tt.key.IsBool(); got != tt.isBool {
				t.Errorf("%s.IsBool() = %v, want %v", tt.key, got, tt.isBool)
			}
		})
	}
}

func TestConfigKeyIsUserOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key        ConfigKey
		isUserOnly bool
	}{
		{KeyNoInput, true},
		{KeyPager, true},
		{KeyNoPager, false},
		{KeyQuiet, false},
		{KeyOutputFormat, false},
		{KeySelectedOrg, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.key), func(t *testing.T) {
			t.Parallel()
			if got := tt.key.IsUserOnly(); got != tt.isUserOnly {
				t.Errorf("%s.IsUserOnly() = %v, want %v", tt.key, got, tt.isUserOnly)
			}
		})
	}
}

func TestConfigKeyValidValues(t *testing.T) {
	t.Parallel()

	t.Run("output_format has valid values", func(t *testing.T) {
		t.Parallel()

		values := KeyOutputFormat.ValidValues()
		if values == nil {
			t.Fatal("expected valid values for output_format")
		}
		expected := []string{"json", "yaml", "text"}
		if len(values) != len(expected) {
			t.Errorf("got %d values, want %d", len(values), len(expected))
		}
	})

	t.Run("boolean keys have true/false", func(t *testing.T) {
		t.Parallel()

		for _, key := range []ConfigKey{KeyNoPager, KeyQuiet, KeyNoInput} {
			values := key.ValidValues()
			if values == nil {
				t.Errorf("%s.ValidValues() = nil, want [true, false]", key)
				continue
			}
			if len(values) != 2 || values[0] != "true" || values[1] != "false" {
				t.Errorf("%s.ValidValues() = %v, want [true, false]", key, values)
			}
		}
	})

	t.Run("pager has no valid values constraint", func(t *testing.T) {
		t.Parallel()

		if values := KeyPager.ValidValues(); values != nil {
			t.Errorf("KeyPager.ValidValues() = %v, want nil", values)
		}
	})
}
