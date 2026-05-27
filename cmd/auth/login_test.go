package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

type stubOAuthTokenStore struct {
	available  bool
	setErr     error
	refreshErr error
	access     map[string]string
	refresh    map[string]string
}

func (s *stubOAuthTokenStore) IsAvailable() bool {
	return s.available
}

func (s *stubOAuthTokenStore) Set(org, token string) error {
	if s.setErr != nil {
		return s.setErr
	}
	if s.access == nil {
		s.access = make(map[string]string)
	}
	s.access[org] = token
	return nil
}

func (s *stubOAuthTokenStore) SetRefreshToken(org, token string) error {
	if s.refreshErr != nil {
		return s.refreshErr
	}
	if s.refresh == nil {
		s.refresh = make(map[string]string)
	}
	s.refresh[org] = token
	return nil
}

func TestOrganizationIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		org      string
		wantSlug string
		wantUUID string
	}{
		{
			name:     "slug",
			org:      "buildkite",
			wantSlug: "buildkite",
			wantUUID: "",
		},
		{
			name:     "uuid",
			org:      "018f2f7e-7e99-7d77-b4d3-a95cb01805f4",
			wantSlug: "",
			wantUUID: "018f2f7e-7e99-7d77-b4d3-a95cb01805f4",
		},
		{
			name:     "uppercase uuid",
			org:      "018F2F7E-7E99-7D77-B4D3-A95CB01805F4",
			wantSlug: "",
			wantUUID: "018F2F7E-7E99-7D77-B4D3-A95CB01805F4",
		},
		{
			name:     "uuid-like slug without hyphens",
			org:      "018f2f7e7e997d77b4d3a95cb01805f4",
			wantSlug: "018f2f7e7e997d77b4d3a95cb01805f4",
			wantUUID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotSlug, gotUUID := organizationIdentifier(tt.org)
			if gotSlug != tt.wantSlug || gotUUID != tt.wantUUID {
				t.Fatalf("organizationIdentifier(%q) = (%q, %q), want (%q, %q)", tt.org, gotSlug, gotUUID, tt.wantSlug, tt.wantUUID)
			}
		})
	}
}

func TestPersistOAuthLogin(t *testing.T) {
	newFactory := func(t *testing.T) *factory.Factory {
		t.Helper()
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Chdir(t.TempDir())

		f, err := factory.New()
		if err != nil {
			t.Fatalf("factory.New() error = %v", err)
		}
		return f
	}

	t.Run("requires an available keychain", func(t *testing.T) {
		f := newFactory(t)
		_, err := persistOAuthLogin(f, &stubOAuthTokenStore{}, "test-org", "access-token", "refresh-token")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "requires an available system keychain") {
			t.Fatalf("expected keychain availability error, got %v", err)
		}
	})

	t.Run("fails when refresh token cannot be stored", func(t *testing.T) {
		f := newFactory(t)
		store := &stubOAuthTokenStore{available: true, refreshErr: errors.New("boom")}

		_, err := persistOAuthLogin(f, store, "test-org", "access-token", "refresh-token")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to store refresh token in keychain") {
			t.Fatalf("expected refresh token storage error, got %v", err)
		}
		if got := store.access["test-org"]; got != "" {
			t.Fatalf("expected access token not to be stored after refresh-token failure, got %q", got)
		}
	})

	t.Run("reports automatic refresh only when refresh token is stored", func(t *testing.T) {
		f := newFactory(t)
		store := &stubOAuthTokenStore{available: true}

		autoRefresh, err := persistOAuthLogin(f, store, "test-org", "access-token", "refresh-token")
		if err != nil {
			t.Fatalf("persistOAuthLogin() error = %v", err)
		}
		if !autoRefresh {
			t.Fatal("expected autoRefresh to be true")
		}
		if got := store.refresh["test-org"]; got != "refresh-token" {
			t.Fatalf("expected refresh token to be stored, got %q", got)
		}
	})
}
