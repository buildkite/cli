package keyring

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	oskeyring "github.com/zalando/go-keyring"
)

// setEnv sets an environment variable for the duration of the test and
// restores the original value (or unsets it) via t.Cleanup.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	original, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("failed to set env %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(key, original)
		} else {
			os.Unsetenv(key)
		}
		// Reset the once so the next test starts fresh.
		ResetForTesting()
	})
	// Reset now so this test sees the new env value.
	ResetForTesting()
}

func shmCredentialPathForTest(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "bk-credentials", "credentials.json")
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod temp dir: %v", err)
	}
	return dir
}

func TestIsKeyringAvailable(t *testing.T) {
	// These tests manipulate package-level state (sync.Once) so must not run
	// in parallel with each other.

	t.Run("disabled by BUILDKITE_NO_KEYRING", func(t *testing.T) {
		setEnv(t, "BUILDKITE_NO_KEYRING", "1")
		setEnv(t, "CI", "")
		setEnv(t, "BUILDKITE", "")
		setEnv(t, CredentialStoreEnv, "")
		setEnv(t, CredentialStorePathEnv, "")

		kr := New()
		if kr.IsAvailable() {
			t.Error("expected keyring to be unavailable when BUILDKITE_NO_KEYRING is set")
		}
	})

	t.Run("disabled by CI", func(t *testing.T) {
		setEnv(t, "CI", "true")
		setEnv(t, "BUILDKITE_NO_KEYRING", "")
		setEnv(t, "BUILDKITE", "")
		setEnv(t, CredentialStoreEnv, "")
		setEnv(t, CredentialStorePathEnv, "")

		kr := New()
		if kr.IsAvailable() {
			t.Error("expected keyring to be unavailable when CI is set")
		}
	})

	t.Run("disabled by BUILDKITE", func(t *testing.T) {
		setEnv(t, "BUILDKITE", "true")
		setEnv(t, "BUILDKITE_NO_KEYRING", "")
		setEnv(t, "CI", "")
		setEnv(t, CredentialStoreEnv, "")
		setEnv(t, CredentialStorePathEnv, "")

		kr := New()
		if kr.IsAvailable() {
			t.Error("expected keyring to be unavailable when BUILDKITE is set")
		}
	})
}

func TestNoKeyringGet(t *testing.T) {
	setEnv(t, "BUILDKITE_NO_KEYRING", "1")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, CredentialStorePathEnv, "")

	kr := New()
	token, err := kr.Get("my-org")
	if token != "" {
		t.Errorf("Get() returned non-empty token with keyring disabled, got %q", token)
	}
	if err == nil {
		t.Error("Get() expected ErrNotFound when keyring is disabled, got nil")
	}
}

func TestNoKeyringSet(t *testing.T) {
	setEnv(t, "BUILDKITE_NO_KEYRING", "1")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, CredentialStorePathEnv, "")

	kr := New()
	if err := kr.Set("my-org", "token-123"); err != nil {
		t.Errorf("Set() returned unexpected error with keyring disabled: %v", err)
	}
}

func TestNoKeyringSetDoesNotCreateSHMStore(t *testing.T) {
	path := shmCredentialPathForTest(t)
	setEnv(t, CredentialStorePathEnv, path)
	setEnv(t, "CI", "true")
	setEnv(t, "BUILDKITE_NO_KEYRING", "")
	setEnv(t, "BUILDKITE", "")
	setEnv(t, CredentialStoreEnv, "")

	kr := New()
	if kr.IsAvailable() {
		t.Fatal("expected auto credential store to be unavailable without an existing shm store")
	}
	if err := kr.Set("my-org", "token-123"); err != nil {
		t.Errorf("Set() returned unexpected error with keyring disabled: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credential file exists after auto Set() with keyring disabled, err=%v", err)
	}
}

func TestNoKeyringDelete(t *testing.T) {
	setEnv(t, "BUILDKITE_NO_KEYRING", "1")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, CredentialStorePathEnv, "")

	kr := New()
	if err := kr.Delete("my-org"); err != nil {
		t.Errorf("Delete() returned unexpected error with keyring disabled: %v", err)
	}
}

func TestMockForTestingBypassesCredentialStorageDisabledEnv(t *testing.T) {
	setEnv(t, "BUILDKITE", "true")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE_NO_KEYRING", "")
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, CredentialStorePathEnv, "")
	MockForTesting()
	t.Cleanup(ResetForTesting)

	kr := New()
	if !kr.IsAvailable() {
		t.Fatal("expected mocked keyring to be available when BUILDKITE is set")
	}

	if err := kr.Set("my-org", "token-123"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	token, err := kr.Get("my-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "token-123" {
		t.Fatalf("Get() = %q, want token-123", token)
	}
}

func TestValidateCredentialStore(t *testing.T) {
	t.Parallel()

	for _, store := range []string{"", StoreAuto, StoreKeyring, StoreSHM} {
		if err := ValidateCredentialStore(store); err != nil {
			t.Fatalf("ValidateCredentialStore(%q) error = %v", store, err)
		}
	}

	if err := ValidateCredentialStore("disk"); err == nil {
		t.Fatal("ValidateCredentialStore(\"disk\") error = nil, want error")
	}
}

func TestNewInvalidCredentialStoreEnvDoesNotFallbackToKeyring(t *testing.T) {
	setEnv(t, CredentialStoreEnv, "disk")
	setEnv(t, CredentialStorePathEnv, "")
	MockForTesting()
	t.Cleanup(ResetForTesting)

	kr := New()
	if kr.IsAvailable() {
		t.Fatal("New() with invalid credential store env reported available")
	}

	if err := kr.Set("my-org", "token-123"); err == nil || !strings.Contains(err.Error(), "unsupported credential store") {
		t.Fatalf("Set() error = %v, want unsupported credential store", err)
	}
	if _, err := kr.Get("my-org"); err == nil || !strings.Contains(err.Error(), "unsupported credential store") {
		t.Fatalf("Get() error = %v, want unsupported credential store", err)
	}
}

func TestExplicitKeyringCredentialStoreIgnoresCIDisable(t *testing.T) {
	setEnv(t, "CI", "true")
	setEnv(t, "BUILDKITE", "")
	setEnv(t, "BUILDKITE_NO_KEYRING", "")
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, CredentialStorePathEnv, "")

	kr, err := NewWithCredentialStore(StoreKeyring)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", StoreKeyring, err)
	}
	if !kr.IsAvailable() {
		t.Fatal("explicit keyring store reported unavailable under CI")
	}

	setEnv(t, CredentialStoreEnv, StoreKeyring)
	kr = New()
	if !kr.IsAvailable() {
		t.Fatal("env-selected keyring store reported unavailable under CI")
	}
}

func TestSHMCredentialStore(t *testing.T) {
	path := shmCredentialPathForTest(t)
	setEnv(t, CredentialStorePathEnv, path)

	kr, err := NewWithCredentialStore(StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", StoreSHM, err)
	}
	if !kr.IsAvailable() {
		t.Fatal("expected shm credential store to be available")
	}

	if err := kr.Set("my-org", "access-token"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := kr.SetRefreshToken("my-org", "refresh-token"); err != nil {
		t.Fatalf("SetRefreshToken() error = %v", err)
	}
	if got := kr.Description(); got != "the /dev/shm credential store" {
		t.Fatalf("Description() = %q, want shm description", got)
	}

	token, err := kr.Get("my-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "access-token" {
		t.Fatalf("Get() = %q, want access-token", token)
	}

	refreshToken, err := kr.GetRefreshToken("my-org")
	if err != nil {
		t.Fatalf("GetRefreshToken() error = %v", err)
	}
	if refreshToken != "refresh-token" {
		t.Fatalf("GetRefreshToken() = %q, want refresh-token", refreshToken)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat credential dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("credential dir mode = %#o, want 0700", got)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat credential file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("credential file mode = %#o, want 0600", got)
	}

	if err := kr.Delete("my-org"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := kr.Get("my-org"); !errors.Is(err, oskeyring.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestSHMCredentialStoreRejectsUnsafeExistingEnvDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on Windows")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod credential dir: %v", err)
	}
	path := filepath.Join(dir, "credentials.json")
	setEnv(t, CredentialStorePathEnv, path)

	kr, err := NewWithCredentialStore(StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", StoreSHM, err)
	}

	err = kr.Set("my-org", "access-token")
	if err == nil {
		t.Fatal("Set() error = nil, want unsafe directory rejection")
	}
	if !strings.Contains(err.Error(), "must not be accessible by group or others") {
		t.Fatalf("Set() error = %v, want unsafe directory rejection", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat credential dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("credential dir mode = %#o, want unchanged 0755", got)
	}
}

func TestSHMCredentialStoreSerializesConcurrentWrites(t *testing.T) {
	path := shmCredentialPathForTest(t)
	setEnv(t, CredentialStorePathEnv, path)

	kr, err := NewWithCredentialStore(StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", StoreSHM, err)
	}

	const writeCount = 50
	var wg sync.WaitGroup
	errCh := make(chan error, writeCount)
	for i := range writeCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			org := fmt.Sprintf("org-%02d", i)
			token := fmt.Sprintf("token-%02d", i)
			errCh <- kr.Set(org, token)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	for i := range writeCount {
		org := fmt.Sprintf("org-%02d", i)
		want := fmt.Sprintf("token-%02d", i)
		got, err := kr.Get(org)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", org, err)
		}
		if got != want {
			t.Fatalf("Get(%q) = %q, want %q", org, got, want)
		}
	}
}

func TestSHMCredentialStoreRejectsSymlinkFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink handling differs on Windows")
	}

	dir := privateTempDir(t)
	path := filepath.Join(dir, "credentials.json")
	target := filepath.Join(dir, "target.json")

	if err := os.WriteFile(target, []byte(`{"services":{}}`), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	setEnv(t, CredentialStorePathEnv, path)

	kr, err := NewWithCredentialStore(StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", StoreSHM, err)
	}

	err = kr.Set("my-org", "token")
	if err == nil {
		t.Fatal("Set() error = nil, want symlink rejection")
	}
	if !strings.Contains(err.Error(), "cannot be a symlink") {
		t.Fatalf("Set() error = %v, want symlink rejection", err)
	}
}

func TestAutoCredentialStoreReadsSHMFallback(t *testing.T) {
	path := shmCredentialPathForTest(t)
	setEnv(t, CredentialStorePathEnv, path)
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, "BUILDKITE_NO_KEYRING", "")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")

	shmStore, err := NewWithCredentialStore(StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", StoreSHM, err)
	}
	if err := shmStore.Set("my-org", "access-token"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := shmStore.SetRefreshToken("my-org", "refresh-token"); err != nil {
		t.Fatalf("SetRefreshToken() error = %v", err)
	}

	MockForTesting()
	t.Cleanup(ResetForTesting)

	kr := New()
	token, err := kr.Get("my-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "access-token" {
		t.Fatalf("Get() = %q, want access-token", token)
	}

	refreshToken, err := kr.GetRefreshToken("my-org")
	if err != nil {
		t.Fatalf("GetRefreshToken() error = %v", err)
	}
	if refreshToken != "refresh-token" {
		t.Fatalf("GetRefreshToken() = %q, want refresh-token", refreshToken)
	}
}

func TestAutoCredentialStorePrefersMarkedSHMOverKeyring(t *testing.T) {
	path := shmCredentialPathForTest(t)
	setEnv(t, CredentialStorePathEnv, path)
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, "BUILDKITE_NO_KEYRING", "")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")

	shmStore, err := NewWithCredentialStore(StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", StoreSHM, err)
	}
	if err := shmStore.Set("my-org", "new-token"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	MockForTesting()
	t.Cleanup(ResetForTesting)
	if err := oskeyring.Set(serviceName, "my-org", "old-token"); err != nil {
		t.Fatalf("seed keyring token: %v", err)
	}

	kr := New()
	token, err := kr.Get("my-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "new-token" {
		t.Fatalf("Get() = %q, want new-token", token)
	}
}

func TestAutoCredentialStoreKeyringWriteDeletesSHMCredential(t *testing.T) {
	path := shmCredentialPathForTest(t)
	setEnv(t, CredentialStorePathEnv, path)
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, "BUILDKITE_NO_KEYRING", "")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")

	shmStore, err := NewWithCredentialStore(StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", StoreSHM, err)
	}
	if err := shmStore.Set("my-org", "shm-token"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	MockForTesting()
	t.Cleanup(ResetForTesting)

	kr := New()
	if err := kr.Set("my-org", "keyring-token"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	fresh := New()
	token, err := fresh.Get("my-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "keyring-token" {
		t.Fatalf("Get() = %q, want keyring-token", token)
	}
	if _, err := shmStore.Get("my-org"); !errors.Is(err, oskeyring.ErrNotFound) {
		t.Fatalf("forced shm Get() after keyring Set() error = %v, want ErrNotFound", err)
	}
}

func TestKeyringWriteIgnoresMalformedSHMCleanup(t *testing.T) {
	path := shmCredentialPathForTest(t)
	setEnv(t, CredentialStorePathEnv, path)
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, "BUILDKITE_NO_KEYRING", "")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create credential dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write malformed credential file: %v", err)
	}

	MockForTesting()
	t.Cleanup(ResetForTesting)

	kr := New()
	if err := kr.Set("my-org", "keyring-token"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	token, err := kr.Get("my-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "keyring-token" {
		t.Fatalf("Get() = %q, want keyring-token", token)
	}
}

func TestAutoCredentialStoreUsesExistingSHMWhenDisabledByEnv(t *testing.T) {
	path := shmCredentialPathForTest(t)
	setEnv(t, CredentialStorePathEnv, path)
	setEnv(t, CredentialStoreEnv, "")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")
	setEnv(t, "BUILDKITE_NO_KEYRING", "")

	shmStore, err := NewWithCredentialStore(StoreSHM)
	if err != nil {
		t.Fatalf("NewWithCredentialStore(%q) error = %v", StoreSHM, err)
	}
	if err := shmStore.Set("my-org", "access-token"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := shmStore.SetRefreshToken("my-org", "refresh-token"); err != nil {
		t.Fatalf("SetRefreshToken() error = %v", err)
	}

	setEnv(t, "CI", "true")
	kr := New()
	if !kr.IsAvailable() {
		t.Fatal("expected auto credential store to use existing shm store when CI disables new storage")
	}

	token, err := kr.Get("my-org")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if token != "access-token" {
		t.Fatalf("Get() = %q, want access-token", token)
	}

	refreshToken, err := kr.GetRefreshToken("my-org")
	if err != nil {
		t.Fatalf("GetRefreshToken() error = %v", err)
	}
	if refreshToken != "refresh-token" {
		t.Fatalf("GetRefreshToken() = %q, want refresh-token", refreshToken)
	}

	if err := kr.Set("my-org", "rotated-access-token"); err != nil {
		t.Fatalf("Set() under disabled env error = %v", err)
	}
	if err := kr.SetRefreshToken("my-org", "rotated-refresh-token"); err != nil {
		t.Fatalf("SetRefreshToken() under disabled env error = %v", err)
	}
	if token, err := shmStore.Get("my-org"); err != nil || token != "rotated-access-token" {
		t.Fatalf("forced shm Get() after auto Set() = %q, %v; want rotated-access-token, nil", token, err)
	}
	if token, err := shmStore.GetRefreshToken("my-org"); err != nil || token != "rotated-refresh-token" {
		t.Fatalf("forced shm GetRefreshToken() after auto SetRefreshToken() = %q, %v; want rotated-refresh-token, nil", token, err)
	}

	if err := kr.Delete("my-org"); err != nil {
		t.Fatalf("Delete() under disabled env error = %v", err)
	}
	if err := kr.DeleteRefreshToken("my-org"); err != nil {
		t.Fatalf("DeleteRefreshToken() under disabled env error = %v", err)
	}
	if _, err := shmStore.Get("my-org"); !errors.Is(err, oskeyring.ErrNotFound) {
		t.Fatalf("forced shm Get() after auto Delete() error = %v, want ErrNotFound", err)
	}
	if _, err := shmStore.GetRefreshToken("my-org"); !errors.Is(err, oskeyring.ErrNotFound) {
		t.Fatalf("forced shm GetRefreshToken() after auto DeleteRefreshToken() error = %v, want ErrNotFound", err)
	}
}
