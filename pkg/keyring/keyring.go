// Package keyring provides credential storage using the OS keychain, with an
// optional tmpfs-backed store for headless Linux hosts.
package keyring

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	oskeyring "github.com/zalando/go-keyring"
)

const (
	serviceName        = "buildkite-cli"
	refreshServiceName = "buildkite-cli-refresh"

	// CredentialStoreEnv selects the credential store used by keyring.New.
	// Supported values are "auto", "keyring", and "shm".
	CredentialStoreEnv = "BUILDKITE_CREDENTIAL_STORE"

	// CredentialStorePathEnv overrides the tmpfs credential file path. It is
	// mainly useful for tests and controlled environments.
	CredentialStorePathEnv = "BUILDKITE_CREDENTIAL_STORE_PATH"

	StoreAuto    = "auto"
	StoreKeyring = "keyring"
	StoreSHM     = "shm"
)

var (
	keyringAvailableOnce sync.Once
	keyringAvailable     bool
	keyringMocked        bool
	shmCredentialMu      sync.Mutex

	errCredentialStoreUnavailable = errors.New("credential store unavailable")
)

// Keyring provides credential storage with fallback support.
type Keyring struct {
	store      string
	useKeyring bool
	lastStore  string
	err        error
}

// New creates a new Keyring instance using the default credential store mode.
func New() *Keyring {
	kr, err := NewWithCredentialStore(os.Getenv(CredentialStoreEnv))
	if err != nil {
		return &Keyring{
			store: normalizeCredentialStore(os.Getenv(CredentialStoreEnv)),
			err:   err,
		}
	}
	return kr
}

// NewWithCredentialStore creates a keyring using the requested credential
// store. An empty store uses the default auto mode.
func NewWithCredentialStore(store string) (*Keyring, error) {
	store = normalizeCredentialStore(store)
	if err := ValidateCredentialStore(store); err != nil {
		return nil, err
	}
	return &Keyring{
		store:      store,
		useKeyring: store == StoreKeyring || (store == StoreAuto && isKeyringAvailable()),
	}, nil
}

func ValidateCredentialStore(store string) error {
	switch normalizeCredentialStore(store) {
	case StoreAuto, StoreKeyring, StoreSHM:
		return nil
	default:
		return fmt.Errorf("unsupported credential store %q (expected %s, %s, or %s)", store, StoreAuto, StoreKeyring, StoreSHM)
	}
}

// Set stores a token for the given organization
func (k *Keyring) Set(org, token string) error {
	return k.set(serviceName, org, token)
}

// Get retrieves a token for the given organization
func (k *Keyring) Get(org string) (string, error) {
	return k.get(serviceName, org)
}

// Delete removes a token for the given organization
func (k *Keyring) Delete(org string) error {
	return k.delete(serviceName, org)
}

// SetRefreshToken stores a refresh token for the given organization
func (k *Keyring) SetRefreshToken(org, token string) error {
	return k.set(refreshServiceName, org, token)
}

// GetRefreshToken retrieves a refresh token for the given organization
func (k *Keyring) GetRefreshToken(org string) (string, error) {
	return k.get(refreshServiceName, org)
}

// DeleteRefreshToken removes a refresh token for the given organization
func (k *Keyring) DeleteRefreshToken(org string) error {
	return k.delete(refreshServiceName, org)
}

// IsAvailable returns true if the configured credential store is available.
func (k *Keyring) IsAvailable() bool {
	if k.err != nil {
		return false
	}
	switch k.store {
	case StoreSHM:
		return shmStoreAvailable()
	case StoreKeyring:
		return k.useKeyring
	default:
		if credentialStorageDisabledByEnv() {
			return shmCredentialFileExists()
		}
		return k.useKeyring || shmStoreAvailable()
	}
}

// Description returns a user-facing description of the active credential store.
func (k *Keyring) Description() string {
	if k.lastStore == StoreSHM || k.store == StoreSHM {
		return "the /dev/shm credential store"
	}
	return "the system keychain"
}

func (k *Keyring) set(service, org, token string) error {
	if k.err != nil {
		return k.err
	}
	if org == "" {
		return errors.New("organization cannot be empty")
	}
	if token == "" {
		return errors.New("token cannot be empty")
	}
	if k.store == StoreAuto && !k.canWriteAuto() {
		return nil
	}

	var keyringErr error
	if k.canUseKeyring() {
		if err := oskeyring.Set(service, org, token); err == nil {
			k.lastStore = StoreKeyring
			if err := deleteSHMCredentialIfPresent(service, org); err != nil {
				return err
			}
			return nil
		} else {
			keyringErr = err
			if k.store == StoreKeyring {
				return err
			}
		}
	}

	if k.canUseSHMForWrite() {
		if err := setSHMCredential(service, org, token); err == nil {
			k.lastStore = StoreSHM
			return nil
		} else if keyringErr == nil {
			return err
		}
	}

	if keyringErr != nil {
		return keyringErr
	}
	return errCredentialStoreUnavailable
}

func (k *Keyring) get(service, org string) (string, error) {
	if k.err != nil {
		return "", k.err
	}
	if org == "" {
		return "", oskeyring.ErrNotFound
	}

	if k.store == StoreAuto && preferredSHMStore(service, org) {
		token, err := getSHMCredential(service, org)
		if err == nil {
			k.lastStore = StoreSHM
			return token, nil
		}
		if !errors.Is(err, oskeyring.ErrNotFound) {
			return "", err
		}
	}

	var keyringErr error
	if k.canUseKeyring() {
		token, err := oskeyring.Get(service, org)
		if err == nil {
			k.lastStore = StoreKeyring
			return token, nil
		}
		keyringErr = err
		if k.store == StoreKeyring {
			return "", err
		}
	}

	if k.canUseSHMForRead() {
		token, err := getSHMCredential(service, org)
		if err == nil {
			k.lastStore = StoreSHM
			return token, nil
		}
		if keyringErr == nil || !errors.Is(err, oskeyring.ErrNotFound) {
			return "", err
		}
	}

	if keyringErr != nil {
		return "", keyringErr
	}
	return "", oskeyring.ErrNotFound
}

func (k *Keyring) delete(service, org string) error {
	if k.err != nil {
		return k.err
	}
	var deleteErr error
	if k.canUseKeyring() {
		if err := oskeyring.Delete(service, org); err != nil && !errors.Is(err, oskeyring.ErrNotFound) {
			deleteErr = err
		}
	}
	if k.canUseSHMForRead() {
		if err := deleteSHMCredential(service, org); err != nil && !errors.Is(err, oskeyring.ErrNotFound) {
			return err
		}
	}
	return deleteErr
}

func (k *Keyring) canUseKeyring() bool {
	if k.store == StoreAuto && k.lastStore == StoreSHM {
		return false
	}
	return k.useKeyring && k.store != StoreSHM
}

func (k *Keyring) canWriteAuto() bool {
	if !credentialStorageDisabledByEnv() {
		return k.useKeyring || shmStoreAvailable()
	}
	return shmCredentialFileExists()
}

func (k *Keyring) canUseSHMForRead() bool {
	switch k.store {
	case StoreSHM:
		return true
	case StoreAuto:
		if credentialStorageDisabledByEnv() {
			return shmCredentialFileExists()
		}
		return shmStoreAvailable()
	default:
		return false
	}
}

func (k *Keyring) canUseSHMForWrite() bool {
	switch k.store {
	case StoreSHM:
		return true
	case StoreAuto:
		if credentialStorageDisabledByEnv() {
			return shmCredentialFileExists()
		}
		return shmStoreAvailable()
	default:
		return false
	}
}

// ClearRefreshToken removes any stored OAuth refresh token for an organization.
// This is used when replacing OAuth credentials with a non-OAuth API token.
func ClearRefreshToken(org string) error {
	if org == "" {
		return errors.New("organization cannot be empty")
	}
	if err := deleteReadableKeyringCredential(refreshServiceName, org); err != nil {
		return err
	}
	return deleteReadableSHMCredential(refreshServiceName, org)
}

// MockForTesting replaces the keyring backend with an in-memory store
// and marks it as available so subsequent New() calls use the mock.
func MockForTesting() {
	oskeyring.MockInit()
	keyringMocked = true
	keyringAvailableOnce = sync.Once{}
	keyringAvailableOnce.Do(func() {
		keyringAvailable = true
	})
}

// ResetForTesting resets the availability cache so that the next call to
// New() re-evaluates the environment. Intended for use in tests only.
func ResetForTesting() {
	keyringAvailableOnce = sync.Once{}
	keyringAvailable = false
	keyringMocked = false
}

// isKeyringAvailable checks if the system keyring can be used
func isKeyringAvailable() bool {
	if keyringMocked {
		return true
	}
	keyringAvailableOnce.Do(func() {
		if credentialStorageDisabledByEnv() {
			keyringAvailable = false
			return
		}
		// Assume keyring is available; callers can handle errors
		keyringAvailable = true
	})
	return keyringAvailable
}

func credentialStorageDisabledByEnv() bool {
	if keyringMocked {
		return false
	}
	return os.Getenv("BUILDKITE_NO_KEYRING") != "" ||
		os.Getenv("CI") != "" ||
		os.Getenv("BUILDKITE") != ""
}

func normalizeCredentialStore(store string) string {
	if store == "" {
		return StoreAuto
	}
	return store
}

type shmCredentials struct {
	Services        map[string]map[string]string `json:"services,omitempty"`
	PreferredStores map[string]map[string]string `json:"preferred_stores,omitempty"`
}

func newSHMCredentials() shmCredentials {
	return shmCredentials{
		Services:        make(map[string]map[string]string),
		PreferredStores: make(map[string]map[string]string),
	}
}

func (c *shmCredentials) ensureMaps() {
	if c.Services == nil {
		c.Services = make(map[string]map[string]string)
	}
	if c.PreferredStores == nil {
		c.PreferredStores = make(map[string]map[string]string)
	}
}

func (c *shmCredentials) preferStore(service, org, store string) {
	c.ensureMaps()
	if c.PreferredStores[service] == nil {
		c.PreferredStores[service] = make(map[string]string)
	}
	c.PreferredStores[service][org] = store
}

func (c *shmCredentials) clearPreferredStore(service, org string) bool {
	c.ensureMaps()
	if c.PreferredStores[service] == nil {
		return false
	}
	if _, ok := c.PreferredStores[service][org]; !ok {
		return false
	}
	delete(c.PreferredStores[service], org)
	if len(c.PreferredStores[service]) == 0 {
		delete(c.PreferredStores, service)
	}
	return true
}

func shmCredentialPath() string {
	if path := os.Getenv(CredentialStorePathEnv); path != "" {
		return path
	}
	uid := os.Getuid()
	if uid < 0 {
		uid = 0
	}
	return filepath.Join("/dev/shm", fmt.Sprintf("buildkite-cli-%d", uid), "credentials.json")
}

func shmCredentialLockPath(path string) string {
	return filepath.Join(filepath.Dir(path), ".credentials.lock")
}

func shmStoreAvailable() bool {
	path := shmCredentialPath()
	if os.Getenv(CredentialStorePathEnv) == "" {
		if info, err := os.Stat("/dev/shm"); err != nil || !info.IsDir() {
			return false
		}
	}
	return ensureSHMStoreDir(path) == nil
}

func shmCredentialFileExists() bool {
	path := shmCredentialPath()
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return validateSHMFile(path, info) == nil
}

func setSHMCredential(service, org, token string) error {
	return withSHMCredentialLock(func() error {
		creds, err := loadSHMCredentials()
		if err != nil {
			return err
		}
		if creds.Services[service] == nil {
			creds.Services[service] = make(map[string]string)
		}
		creds.Services[service][org] = token
		creds.preferStore(service, org, StoreSHM)
		return saveSHMCredentials(creds)
	})
}

func getSHMCredential(service, org string) (string, error) {
	creds, err := loadSHMCredentials()
	if err != nil {
		return "", err
	}
	if token := creds.Services[service][org]; token != "" {
		return token, nil
	}
	return "", oskeyring.ErrNotFound
}

func deleteSHMCredential(service, org string) error {
	return withSHMCredentialLock(func() error {
		creds, err := loadSHMCredentials()
		if err != nil {
			return err
		}
		if creds.Services[service] == nil {
			return oskeyring.ErrNotFound
		}
		if _, ok := creds.Services[service][org]; !ok {
			return oskeyring.ErrNotFound
		}
		delete(creds.Services[service], org)
		creds.clearPreferredStore(service, org)
		return saveSHMCredentials(creds)
	})
}

func preferredSHMStore(service, org string) bool {
	preferred, err := preferredSHMStoreWithError(service, org)
	return err == nil && preferred
}

func preferredSHMStoreWithError(service, org string) (bool, error) {
	path := shmCredentialPath()
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if err := validateSHMFile(path, info); err != nil {
		return false, err
	}
	creds, err := readSHMCredentialsFile(path)
	if err != nil {
		return false, err
	}
	return creds.PreferredStores[service][org] == StoreSHM, nil
}

func deleteSHMCredentialIfPresent(service, org string) error {
	preferred, err := preferredSHMStoreWithError(service, org)
	if err != nil || !preferred {
		return nil
	}
	if err := deleteSHMCredential(service, org); err != nil && !errors.Is(err, oskeyring.ErrNotFound) {
		return err
	}
	return nil
}

func deleteReadableKeyringCredential(service, org string) error {
	if _, err := oskeyring.Get(service, org); err != nil {
		return nil
	}
	if err := oskeyring.Delete(service, org); err != nil && !errors.Is(err, oskeyring.ErrNotFound) {
		return err
	}
	return nil
}

func deleteReadableSHMCredential(service, org string) error {
	if !shmCredentialFileExists() {
		return nil
	}
	if _, err := getSHMCredential(service, org); err != nil {
		return nil
	}
	if err := deleteSHMCredential(service, org); err != nil && !errors.Is(err, oskeyring.ErrNotFound) {
		return err
	}
	return nil
}

func withSHMCredentialLock(fn func() error) error {
	shmCredentialMu.Lock()
	defer shmCredentialMu.Unlock()

	path := shmCredentialPath()
	if err := ensureSHMStoreDir(path); err != nil {
		return err
	}

	lockPath := shmCredentialLockPath(path)
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()

	info, err := os.Lstat(lockPath)
	if err != nil {
		return err
	}
	if err := validateSHMFile(lockPath, info); err != nil {
		return err
	}

	if err := lockFile(lock); err != nil {
		return err
	}
	defer func() { _ = unlockFile(lock) }()

	return fn()
}

func loadSHMCredentials() (shmCredentials, error) {
	path := shmCredentialPath()
	if err := ensureSHMStoreDir(path); err != nil {
		return shmCredentials{}, err
	}

	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return newSHMCredentials(), nil
	}
	if err != nil {
		return shmCredentials{}, err
	}
	if err := validateSHMFile(path, info); err != nil {
		return shmCredentials{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return shmCredentials{}, err
	}
	return readSHMCredentials(data)
}

func readSHMCredentialsFile(path string) (shmCredentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return shmCredentials{}, err
	}
	return readSHMCredentials(data)
}

func readSHMCredentials(data []byte) (shmCredentials, error) {
	if len(data) == 0 {
		return newSHMCredentials(), nil
	}

	creds := newSHMCredentials()
	if err := json.Unmarshal(data, &creds); err != nil {
		return shmCredentials{}, err
	}
	creds.ensureMaps()
	return creds, nil
}

func saveSHMCredentials(creds shmCredentials) error {
	path := shmCredentialPath()
	if err := ensureSHMStoreDir(path); err != nil {
		return err
	}
	creds.ensureMaps()

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func ensureSHMStoreDir(path string) error {
	dir := filepath.Dir(path)
	createdDir := false

	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		createdDir = true
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("credential store directory %s cannot be a symlink", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("credential store path %s is not a directory", dir)
	}
	if err := validateCurrentUserOwner(dir, info); err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if createdDir || os.Getenv(CredentialStorePathEnv) == "" {
			if err := os.Chmod(dir, 0o700); err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("credential store directory %s must not be accessible by group or others", dir)
	}
	return nil
}

func validateSHMFile(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("credential store file %s cannot be a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("credential store file %s is not a regular file", path)
	}
	if err := validateCurrentUserOwner(path, info); err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func validateCurrentUserOwner(path string, info os.FileInfo) error {
	uid := os.Getuid()
	if uid < 0 {
		return nil
	}
	ownerUID, ok := fileOwnerUID(info)
	if !ok {
		return nil
	}
	if ownerUID != uint32(uid) {
		return fmt.Errorf("credential store path %s is owned by uid %d, not current uid %d", path, ownerUID, uid)
	}
	return nil
}
