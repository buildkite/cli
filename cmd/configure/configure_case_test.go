package configure

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/spf13/afero"
)

func TestConfigurePreservesOrganizationCase(t *testing.T) {
	testCases := []struct {
		name        string
		orgInput    string
		expectedOrg string
	}{
		{
			name:        "preserves mixed case organization name",
			orgInput:    "gridX",
			expectedOrg: "gridX",
		},
		{
			name:        "preserves uppercase organization name",
			orgInput:    "ACME",
			expectedOrg: "ACME",
		},
		{
			name:        "preserves lowercase organization name",
			orgInput:    "buildkite",
			expectedOrg: "buildkite",
		},
		{
			name:        "preserves camelCase organization name",
			orgInput:    "myOrg",
			expectedOrg: "myOrg",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keyring.MockForTesting()

			fs := afero.NewMemMapFs()
			conf := config.New(fs, nil)
			f := &factory.Factory{Config: conf}

			token := "bk_test_token_12345"

			err := ConfigureWithCredentials(f, tc.orgInput, token)
			if err != nil {
				t.Fatalf("ConfigureWithCredentials failed: %v", err)
			}

			gotOrg := conf.OrganizationSlug()
			if gotOrg != tc.expectedOrg {
				t.Errorf("expected organization to be %q, got %q", tc.expectedOrg, gotOrg)
			}

			kr := keyring.New()
			gotToken, _ := kr.Get(tc.orgInput)
			if gotToken != token {
				t.Errorf("expected token to be %q, got %q", token, gotToken)
			}
		})
	}
}
