package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
	buildkite "github.com/buildkite/go-buildkite/v4"
	oskeyring "github.com/zalando/go-keyring"
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

type authStubGlobals struct{}

func (authStubGlobals) SkipConfirmation() bool { return false }
func (authStubGlobals) DisableInput() bool     { return false }
func (authStubGlobals) IsQuiet() bool          { return false }
func (authStubGlobals) DisablePager() bool     { return false }
func (authStubGlobals) EnableDebug() bool      { return false }

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

	t.Run("requires an available credential store", func(t *testing.T) {
		f := newFactory(t)
		_, err := persistOAuthLogin(f, &stubOAuthTokenStore{}, "test-org", "access-token", "refresh-token")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "requires an available credential store") {
			t.Fatalf("expected credential store availability error, got %v", err)
		}
	})

	t.Run("fails when refresh token cannot be stored", func(t *testing.T) {
		f := newFactory(t)
		store := &stubOAuthTokenStore{available: true, refreshErr: errors.New("boom")}

		_, err := persistOAuthLogin(f, store, "test-org", "access-token", "refresh-token")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to store refresh token in credential store") {
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

func TestLoginCmdValidateDeviceIncompatibleFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     LoginCmd
		wantErr string
	}{
		{
			name:    "device with token",
			cmd:     LoginCmd{Device: true, Token: "token", Org: "buildkite"},
			wantErr: "--device cannot be used with --token",
		},
		{
			name:    "device with org",
			cmd:     LoginCmd{Device: true, Org: "buildkite"},
			wantErr: "--org is not supported with --device; choose an organization on the authorization page",
		},
		{
			name: "device only",
			cmd:  LoginCmd{Device: true},
		},
		{
			name: "token with org",
			cmd:  LoginCmd{Token: "token", Org: "buildkite"},
		},
		{
			name:    "invalid credential store",
			cmd:     LoginCmd{CredentialStore: "disk"},
			wantErr: "unsupported credential store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cmd.validate(nil)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("validate() error = nil, want error")
			}
			if got := err.Error(); !strings.Contains(got, tt.wantErr) {
				t.Fatalf("validate() error = %q, want substring %q", got, tt.wantErr)
			}
		})
	}
}

func TestLoginCmdValidateCredentialStoreEnv(t *testing.T) {
	t.Setenv(keyring.CredentialStoreEnv, "disk")

	err := (&LoginCmd{}).validate(nil)
	if err == nil {
		t.Fatal("validate() error = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "unsupported credential store") {
		t.Fatalf("validate() error = %q, want unsupported credential store", got)
	}
}

func TestLoginCmdRunWithTokenUsesCredentialStoreEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")

	path := filepath.Join(t.TempDir(), "bk-credentials", "credentials.json")
	t.Setenv(keyring.CredentialStoreEnv, keyring.StoreSHM)
	t.Setenv(keyring.CredentialStorePathEnv, path)
	keyring.ResetForTesting()
	t.Cleanup(keyring.ResetForTesting)

	cmd := &LoginCmd{Org: "test-org", Token: "access-token"}
	if err := cmd.Run(nil, authStubGlobals{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	kr, err := keyring.NewWithCredentialStore(keyring.StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", keyring.StoreSHM, err)
	}
	token, err := kr.Get("test-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "access-token" {
		t.Fatalf("stored token = %q, want access-token", token)
	}
}

func TestLoginCmdRunWithTokenUsesCredentialStoreEnvWhenFlagOmitted(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")

	path := filepath.Join(t.TempDir(), "bk-credentials", "credentials.json")
	t.Setenv(keyring.CredentialStoreEnv, keyring.StoreSHM)
	t.Setenv(keyring.CredentialStorePathEnv, path)
	keyring.ResetForTesting()
	t.Cleanup(keyring.ResetForTesting)

	var cli struct {
		Login LoginCmd `cmd:""`
	}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	ctx, err := parser.Parse([]string{"login", "--org", "test-org", "--token", "access-token"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cli.Login.CredentialStore != keyring.StoreAuto {
		t.Fatalf("parsed credential store = %q, want %q", cli.Login.CredentialStore, keyring.StoreAuto)
	}
	if cli.Login.credentialStoreFlagProvided(ctx) {
		t.Fatal("credentialStoreFlagProvided() = true, want false")
	}

	if err := cli.Login.Run(ctx, authStubGlobals{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	kr, err := keyring.NewWithCredentialStore(keyring.StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", keyring.StoreSHM, err)
	}
	token, err := kr.Get("test-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "access-token" {
		t.Fatalf("stored token = %q, want access-token", token)
	}
}

func TestLoginCmdRunWithTokenClearsStaleOAuthRefreshTokens(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")

	path := filepath.Join(t.TempDir(), "bk-credentials", "credentials.json")
	t.Setenv(keyring.CredentialStoreEnv, keyring.StoreSHM)
	t.Setenv(keyring.CredentialStorePathEnv, path)
	keyring.MockForTesting()
	t.Cleanup(keyring.ResetForTesting)

	keyringStore, err := keyring.NewWithCredentialStore(keyring.StoreKeyring)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", keyring.StoreKeyring, err)
	}
	if err := keyringStore.SetRefreshToken("test-org", "old-keyring-refresh-token"); err != nil {
		t.Fatalf("SetRefreshToken() keyring error = %v", err)
	}

	shmStore, err := keyring.NewWithCredentialStore(keyring.StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", keyring.StoreSHM, err)
	}
	cmd := &LoginCmd{Org: "test-org", Token: "access-token"}
	if err := cmd.Run(nil, authStubGlobals{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	token, err := shmStore.Get("test-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "access-token" {
		t.Fatalf("stored token = %q, want access-token", token)
	}
	if _, err := keyringStore.GetRefreshToken("test-org"); !errors.Is(err, oskeyring.ErrNotFound) {
		t.Fatalf("keyring GetRefreshToken() error = %v, want ErrNotFound", err)
	}
}

func TestLoginCmdRunWithTokenCredentialStoreFlagOverridesEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")

	path := filepath.Join(t.TempDir(), "bk-credentials", "credentials.json")
	t.Setenv(keyring.CredentialStoreEnv, "disk")
	t.Setenv(keyring.CredentialStorePathEnv, path)
	keyring.ResetForTesting()
	t.Cleanup(keyring.ResetForTesting)

	var cli struct {
		Login LoginCmd `cmd:""`
	}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	ctx, err := parser.Parse([]string{"login", "--org", "test-org", "--token", "access-token", "--credential-store", "shm"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !cli.Login.credentialStoreFlagProvided(ctx) {
		t.Fatal("credentialStoreFlagProvided() = false, want true")
	}

	if err := cli.Login.Run(ctx, authStubGlobals{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	kr, err := keyring.NewWithCredentialStore(keyring.StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", keyring.StoreSHM, err)
	}
	token, err := kr.Get("test-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "access-token" {
		t.Fatalf("stored token = %q, want access-token", token)
	}
}

func TestLoginCmdApplyCredentialStoreEnvForExplicitFlag(t *testing.T) {
	t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)

	var cli struct {
		Login LoginCmd `cmd:""`
	}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	ctx, err := parser.Parse([]string{"login", "--credential-store", "shm"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	restore, err := cli.Login.applyCredentialStoreEnv(ctx)
	if err != nil {
		t.Fatalf("applyCredentialStoreEnv() error = %v", err)
	}
	if got := os.Getenv(keyring.CredentialStoreEnv); got != keyring.StoreSHM {
		t.Fatalf("%s = %q, want %q", keyring.CredentialStoreEnv, got, keyring.StoreSHM)
	}
	restore()
	if got := os.Getenv(keyring.CredentialStoreEnv); got != keyring.StoreKeyring {
		t.Fatalf("restored %s = %q, want %q", keyring.CredentialStoreEnv, got, keyring.StoreKeyring)
	}
}

func TestLoginCmdRunDeviceFlow(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")

	keyring.MockForTesting()
	t.Cleanup(keyring.ResetForTesting)

	var sawDeviceAuthorization bool
	var sawTokenExchange bool
	var sawOrganizationsList bool

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/oauth/device_authorization":
			sawDeviceAuthorization = true
			if r.Method != "POST" {
				t.Errorf("device authorization method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if got := r.FormValue("client_id"); got != oauth.DefaultClientID {
				t.Errorf("client_id = %q, want %q", got, oauth.DefaultClientID)
			}
			if got := r.FormValue("scope"); got != "read_user read_organizations" {
				t.Errorf("scope = %q, want requested scopes", got)
			}
			_ = json.NewEncoder(w).Encode(oauth.DeviceAuthorizationResponse{
				DeviceCode:              "device-code",
				UserCode:                "ABCD-EFGH",
				VerificationURI:         "https://buildkite.example/device",
				VerificationURIComplete: "https://buildkite.example/device/ABCD-EFGH",
				ExpiresIn:               600,
				Interval:                1,
			})
		case "/oauth/token":
			sawTokenExchange = true
			if r.Method != "POST" {
				t.Errorf("token method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if got := r.FormValue("grant_type"); got != "urn:ietf:params:oauth:grant-type:device_code" {
				t.Errorf("grant_type = %q, want device code grant", got)
			}
			if got := r.FormValue("device_code"); got != "device-code" {
				t.Errorf("device_code = %q, want device-code", got)
			}
			_ = json.NewEncoder(w).Encode(oauth.TokenResponse{
				AccessToken:  "access-token",
				TokenType:    "Bearer",
				Scope:        "read_user read_organizations",
				RefreshToken: "refresh-token",
				ExpiresIn:    3600,
			})
		case "/v2/organizations":
			sawOrganizationsList = true
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Errorf("Authorization = %q, want Bearer access-token", got)
			}
			_ = json.NewEncoder(w).Encode([]buildkite.Organization{{Slug: "test-org"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)

	cmd := &LoginCmd{Device: true, Scopes: "read_user read_organizations"}
	if err := cmd.Run(nil, authStubGlobals{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !sawDeviceAuthorization {
		t.Fatal("device authorization endpoint was not called")
	}
	if !sawTokenExchange {
		t.Fatal("token endpoint was not called")
	}
	if !sawOrganizationsList {
		t.Fatal("organizations endpoint was not called")
	}

	kr := keyring.New()
	token, err := kr.Get("test-org")
	if err != nil {
		t.Fatalf("Get token: %v", err)
	}
	if token != "access-token" {
		t.Fatalf("stored token = %q, want access-token", token)
	}

	refreshToken, err := kr.GetRefreshToken("test-org")
	if err != nil {
		t.Fatalf("GetRefreshToken: %v", err)
	}
	if refreshToken != "refresh-token" {
		t.Fatalf("stored refresh token = %q, want refresh-token", refreshToken)
	}

	configBytes, err := os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "bk.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	config := string(configBytes)
	if !strings.Contains(config, "selected_org: test-org") {
		t.Fatalf("config did not select test-org:\n%s", config)
	}
	if !strings.Contains(config, "test-org:") {
		t.Fatalf("config did not register test-org:\n%s", config)
	}
}
