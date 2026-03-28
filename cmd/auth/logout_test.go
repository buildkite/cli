package auth

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
	"github.com/spf13/afero"
)

func TestLogoutOrgDeletesSiblingOAuthAliases(t *testing.T) {
	keyring.MockForTesting()

	conf := config.New(afero.NewMemMapFs(), nil)
	if err := conf.EnsureOrganization("org-a"); err != nil {
		t.Fatalf("EnsureOrganization org-a returned error: %v", err)
	}
	if err := conf.EnsureOrganization("org-b"); err != nil {
		t.Fatalf("EnsureOrganization org-b returned error: %v", err)
	}
	if err := conf.SelectOrganization("org-a", false); err != nil {
		t.Fatalf("SelectOrganization returned error: %v", err)
	}

	session := &oauth.Session{
		Version:      oauth.SessionVersion,
		Host:         "buildkite.localhost",
		ClientID:     "buildkite-cli",
		AccessToken:  "bkua_access",
		RefreshToken: "bkrt_refresh",
		TokenType:    "Bearer",
	}

	kr := keyring.New()
	if err := kr.SetSession("org-a", session); err != nil {
		t.Fatalf("SetSession org-a returned error: %v", err)
	}
	if err := kr.SetSession("org-b", session); err != nil {
		t.Fatalf("SetSession org-b returned error: %v", err)
	}

	cmd := &LogoutCmd{Org: "org-a"}
	f := &factory.Factory{Config: conf}
	if err := cmd.logoutOrg(f); err != nil {
		t.Fatalf("logoutOrg returned error: %v", err)
	}

	if _, err := kr.GetSession("org-a"); err == nil {
		t.Fatal("expected org-a session to be deleted")
	}
	if _, err := kr.GetSession("org-b"); err == nil {
		t.Fatal("expected org-b sibling session to be deleted")
	}
}
