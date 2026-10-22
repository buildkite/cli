package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bkhttp "github.com/buildkite/cli/v3/internal/http"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
)

func tokenInspectionFactory(t *testing.T, handler http.HandlerFunc) (*factory.Factory, *keyring.Keyring) {
	t.Helper()
	keyring.MockForTesting()
	t.Cleanup(keyring.ResetForTesting)
	t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
	t.Setenv(keyring.CredentialStorePathEnv, filepath.Join(t.TempDir(), "credentials.json"))
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("BUILDKITE_HOST", strings.TrimPrefix(server.URL, "https://"))
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)

	store := keyring.New()
	if err := store.Set("test-org", "old-token"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRefreshToken("test-org", "old-refresh"); err != nil {
		t.Fatal(err)
	}
	f, err := factory.New(factory.WithTransport(server.Client().Transport))
	if err != nil {
		t.Fatal(err)
	}
	return f, store
}

func writeTokenInspection(w http.ResponseWriter, expiresAt *time.Time) {
	w.Header().Set("Content-Type", "application/json")
	var expiry any
	if expiresAt != nil {
		expiry = expiresAt.UTC().Format("2006-01-02 15:04:05 MST")
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"uuid": "synthetic-token", "expires_at": expiry})
}

func TestInspectTokenReturnsFinalResponseCredentialAfterRefresh(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	var inspections, exchanges atomic.Int32
	f, store := tokenInspectionFactory(t, func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v2/access-token":
			inspections.Add(1)
			switch request.Header.Get("Authorization") {
			case "Bearer old-token":
				w.WriteHeader(http.StatusUnauthorized)
			case "Bearer new-token":
				writeTokenInspection(w, &expiresAt)
			default:
				t.Error("unexpected inspection credential")
				w.WriteHeader(http.StatusInternalServerError)
			}
		case "/oauth/token":
			exchanges.Add(1)
			if request.FormValue("refresh_token") != "old-refresh" {
				t.Error("unexpected refresh credential")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh","expires_in":3600}`))
		default:
			t.Errorf("unexpected request path %s", request.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	inspection, err := inspectToken(context.Background(), f)
	if err != nil || inspection.AccessToken != "new-token" || inspection.ExpiresAt == nil || !inspection.ExpiresAt.Equal(expiresAt) || inspection.OrganizationSlug != "test-org" {
		t.Fatalf("inconsistent inspection: expiry=%v organization=%q error=%v", inspection.ExpiresAt, inspection.OrganizationSlug, err)
	}
	stored, err := store.Get("test-org")
	if err != nil || stored != "new-token" || inspections.Load() != 2 || exchanges.Load() != 1 {
		t.Fatalf("unexpected renewal: inspections=%d exchanges=%d error=%v", inspections.Load(), exchanges.Load(), err)
	}
}

func TestInspectTokenReportsAcceptedShortLifetimeWithoutRefresh(t *testing.T) {
	expiresAt := time.Now().UTC().Add(30 * time.Second).Truncate(time.Second)
	var inspections, exchanges atomic.Int32
	f, _ := tokenInspectionFactory(t, func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v2/access-token":
			inspections.Add(1)
			writeTokenInspection(w, &expiresAt)
		case "/oauth/token":
			exchanges.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	inspection, err := inspectToken(context.Background(), f)
	if err != nil || inspection.AccessToken != "old-token" || inspection.ExpiresAt == nil || !inspection.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("incorrect short-lifetime inspection: expiry=%v error=%v", inspection.ExpiresAt, err)
	}
	if inspections.Load() != 1 || exchanges.Load() != 0 {
		t.Fatalf("unexpected requests: inspections=%d exchanges=%d", inspections.Load(), exchanges.Load())
	}
}

func TestInspectTokenKeepsResponseCredentialWhenSourceChanges(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	source := make(chan *bkhttp.RefreshTransport, 1)
	f, _ := tokenInspectionFactory(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/access-token" || request.Header.Get("Authorization") != "Bearer old-token" {
			t.Error("unexpected inspection request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		(<-source).TokenSource.SetToken("newer-token")
		writeTokenInspection(w, &expiresAt)
	})
	refresh, ok := f.HTTPClient.Transport.(*bkhttp.RefreshTransport)
	if !ok {
		t.Fatal("factory did not install its refresh transport")
	}
	source <- refresh

	inspection, err := inspectToken(context.Background(), f)
	if err != nil || inspection.AccessToken != "old-token" || inspection.ExpiresAt == nil || !inspection.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("response snapshot changed: expiry=%v error=%v", inspection.ExpiresAt, err)
	}
	if refresh.TokenSource.Token() != "newer-token" {
		t.Fatal("controlled token-source update did not occur")
	}
}

func TestInspectTokenReportsMissingExpiryWithoutClassifyingIt(t *testing.T) {
	f, _ := tokenInspectionFactory(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/access-token" {
			t.Errorf("unexpected request path %s", request.URL.Path)
		}
		writeTokenInspection(w, nil)
	})

	inspection, err := inspectToken(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(inspection)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	expiry, reported := output["expires_at"]
	if !reported || expiry != nil || output["access_token"] != "old-token" || output["organization_slug"] != "test-org" {
		t.Fatal("inspection did not preserve the missing expiry")
	}
}
