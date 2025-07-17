package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBuildIdentifier(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		defaultOrg  string
		wantOrg     string
		wantPipe    string
		wantBuild   string
		wantErr     bool
		errContains string
	}{
		// URL formats
		{
			name:      "valid build URL",
			input:     "https://buildkite.com/myorg/mypipeline/builds/123",
			wantOrg:   "myorg",
			wantPipe:  "mypipeline",
			wantBuild: "123",
		},
		{
			name:        "invalid build URL - bad format",
			input:       "https://buildkite.com/invalid/url",
			wantErr:     true,
			errContains: "invalid build URL format",
		},
		{
			name:        "invalid build URL - malformed",
			input:       "http://[::1]:namedport",
			wantErr:     true,
			errContains: "invalid build URL",
		},
		// Slash formats
		{
			name:      "org/pipeline/number format",
			input:     "myorg/mypipeline/456",
			wantOrg:   "myorg",
			wantPipe:  "mypipeline",
			wantBuild: "456",
		},
		{
			name:       "pipeline/number format with default org",
			input:      "mypipeline/789",
			defaultOrg: "default-org",
			wantOrg:    "default-org",
			wantPipe:   "mypipeline",
			wantBuild:  "789",
		},
		{
			name:       "single slash only",
			input:      "/",
			defaultOrg: "default-org",
			wantOrg:    "default-org",
			wantPipe:   "",
			wantBuild:  "",
		},
		// Number only
		{
			name:      "build number only",
			input:     "42",
			wantOrg:   "",
			wantPipe:  "",
			wantBuild: "42",
		},
		{
			name:      "build number with leading zeros",
			input:     "007",
			wantOrg:   "",
			wantPipe:  "",
			wantBuild: "007",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, pipeline, buildNumber, err := parseBuildIdentifier(tt.input, tt.defaultOrg)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantOrg, org, "org mismatch")
				assert.Equal(t, tt.wantPipe, pipeline, "pipeline mismatch")
				assert.Equal(t, tt.wantBuild, buildNumber, "build number mismatch")
			}
		})
	}
}
