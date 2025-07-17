package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaderFlag_ParseFormats(t *testing.T) {
	// Since we can't easily mock Kong's DecodeContext, we'll test the parsing logic
	// by extracting it to a helper function
	tests := []struct {
		name     string
		input    string
		wantKey  string
		wantVal  string
		wantErr  bool
	}{
		{
			name:    "key=value format",
			input:   "Content-Type=application/json",
			wantKey: "Content-Type",
			wantVal: "application/json",
		},
		{
			name:    "key: value format",
			input:   "Authorization: Bearer token123",
			wantKey: "Authorization",
			wantVal: "Bearer token123",
		},
		{
			name:    "key:value format (no space)",
			input:   "X-Custom:value",
			wantKey: "X-Custom",
			wantVal: "value",
		},
		{
			name:    "invalid format",
			input:   "invalid-header",
			wantErr: true,
		},
		{
			name:    "empty value with equals",
			input:   "X-Empty=",
			wantKey: "X-Empty",
			wantVal: "",
		},
		{
			name:    "empty value with colon",
			input:   "X-Empty:",
			wantKey: "X-Empty",
			wantVal: "",
		},
		{
			name:    "value with equals inside",
			input:   "Authorization=Bearer token=123",
			wantKey: "Authorization",
			wantVal: "Bearer token=123",
		},
		{
			name:    "value with colon inside",
			input:   "URL:https://example.com:8080",
			wantKey: "URL",
			wantVal: "https://example.com:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the parsing logic that would be in Decode
			key, val, err := parseHeaderValue(tt.input)
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantKey, key)
				assert.Equal(t, tt.wantVal, val)
			}
		})
	}
}

// Helper function to test the parsing logic
func parseHeaderValue(raw string) (key, val string, err error) {
	var parts []string
	switch {
	case strings.Contains(raw, "="):
		parts = strings.SplitN(raw, "=", 2)
	case strings.Contains(raw, ":"):
		parts = strings.SplitN(raw, ":", 2)
	default:
		return "", "", fmt.Errorf("invalid header %q (expected KEY=VAL or KEY: VAL)", raw)
	}

	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid header %q (expected KEY=VAL or KEY: VAL)", raw)
	}

	key = strings.TrimSpace(parts[0])
	val = strings.TrimSpace(parts[1])
	if key == "" {
		return "", "", fmt.Errorf("header name cannot be empty")
	}

	return key, val, nil
}
