package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

// fakeKeychain is an in-memory keychain. failSave fails the next Save calls;
// corruptSaves makes the next saves store other bytes (read-back mismatch).
type fakeKeychain struct {
	mu           sync.Mutex
	data         []byte
	has          bool
	failSave     int
	corruptSaves int
	saves        int
	loadErr      error
}

func (k *fakeKeychain) Load(context.Context) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.loadErr != nil {
		return nil, k.loadErr
	}
	if !k.has {
		return nil, ErrOfficialCredentialMissing
	}
	return append([]byte(nil), k.data...), nil
}

func (k *fakeKeychain) Save(_ context.Context, data []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.saves++
	if k.failSave > 0 {
		k.failSave--
		return ErrKeychainUnavailable
	}
	k.data, k.has = append([]byte(nil), data...), true
	if k.corruptSaves > 0 {
		k.corruptSaves--
		k.data = []byte(`{"token":{"access_token":"other"}}`)
	}
	return nil
}

func (k *fakeKeychain) Remove(context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.data, k.has = nil, false
	return nil
}

func (k *fakeKeychain) get() ([]byte, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]byte(nil), k.data...), k.has
}

type testEnv struct {
	store    *Store
	keychain *fakeKeychain
	home     string
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	home := t.TempDir()
	kc := &fakeKeychain{}
	store := NewStoreAt(filepath.Join(t.TempDir(), "gemini"), &OfficialCredentials{Keychain: kc, Files: OfficialFiles(home)})
	return testEnv{store: store, keychain: kc, home: home}
}

func idToken(sub, email string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	claims, _ := json.Marshal(map[string]string{"sub": sub, "email": email, "name": "N " + sub})
	return header + "." + base64.RawURLEncoding.EncodeToString(claims) + ".sig"
}

func credJSON(t *testing.T, access, refresh, id string, expiry time.Time) []byte {
	t.Helper()
	data, err := NewCredential(access, refresh, id, expiry).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustCred(t *testing.T, data []byte) Credential {
	t.Helper()
	c, err := ParseCredential(data)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sameJSON(a, b []byte) bool { return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b)) }

func isErr[T error](err error) bool {
	var target T
	return errors.As(err, &target)
}
