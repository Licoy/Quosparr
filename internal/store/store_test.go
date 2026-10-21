package store

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const (
	currentJSON = `{"language":"french","appearance":"dark","futureKey":{"n":1}}`
	legacyJSON  = `{"language":"korean","showsRecentSessions":true,"oldOnly":[1,2]}`
)

func newStore(current, legacy string) (*Store, *MemoryDomain, *MemoryDomain) {
	cur, leg := NewMemoryDomain(), NewMemoryDomain()
	if current != "" {
		cur.Put(KeyPreferences, []byte(current))
	}
	if legacy != "" {
		leg.Put(KeyPreferences, []byte(legacy))
	}
	return New(cur, leg), cur, leg
}

func TestLoadNothingStoredGivesDefaultsAndWritesNothing(t *testing.T) {
	s, cur, leg := newStore("", "")
	p, src, err := s.LoadPreferences()
	if err != nil || src != SourceDefaults || !reflect.DeepEqual(p, DefaultPreferences()) {
		t.Fatalf("got %+v, %v, %v", p, src, err)
	}
	if cur.Writes() != 0 || leg.Writes() != 0 {
		t.Error("loading nothing must not write")
	}
	// Without a legacy domain at all.
	if _, src, err := New(NewMemoryDomain(), nil).LoadPreferences(); err != nil || src != SourceDefaults {
		t.Errorf("nil legacy: %v %v", src, err)
	}
}

func TestCurrentDomainWinsOverLegacy(t *testing.T) {
	s, cur, leg := newStore(currentJSON, legacyJSON)
	p, src, err := s.LoadPreferences()
	if err != nil || src != SourceCurrent {
		t.Fatalf("got %v, %v", src, err)
	}
	if p.Language != LanguageFrench || p.ShowsRecentSessions {
		t.Errorf("legacy values leaked into the current settings: %+v", p)
	}
	if cur.Writes() != 0 || leg.Writes() != 0 {
		t.Error("no writes expected")
	}
}

func TestLegacyMigration(t *testing.T) {
	s, cur, leg := newStore("", legacyJSON)
	p, src, err := s.LoadPreferences()
	if err != nil || src != SourceLegacy {
		t.Fatalf("got %v, %v", src, err)
	}
	if p.Language != LanguageKorean || !p.ShowsRecentSessions {
		t.Errorf("legacy values not used: %+v", p)
	}
	// The original bytes, including unknown keys, are copied verbatim.
	got, found, _ := cur.Read(KeyPreferences)
	if !found || string(got) != legacyJSON {
		t.Errorf("current domain holds %q", got)
	}
	if cur.Writes() != 1 {
		t.Errorf("current writes = %d, want 1", cur.Writes())
	}
	// The legacy domain is never modified or deleted.
	if left, found, _ := leg.Read(KeyPreferences); !found || string(left) != legacyJSON || leg.Writes() != 0 {
		t.Error("legacy domain was touched")
	}
	// A second load reads the current domain and does not migrate again.
	if _, src, err := s.LoadPreferences(); err != nil || src != SourceCurrent || cur.Writes() != 1 {
		t.Errorf("second load: %v %v writes=%d", src, err, cur.Writes())
	}
}

func TestLegacyAbsentDoesNotMigrate(t *testing.T) {
	cur, leg := NewMemoryDomain(), NewMemoryDomain()
	leg.Put(KeyNetworkProxy, []byte(`{"unrelated":1}`)) // other keys do not count
	_, src, err := New(cur, leg).LoadPreferences()
	if err != nil || src != SourceDefaults || cur.Writes() != 0 {
		t.Errorf("got %v %v writes=%d", src, err, cur.Writes())
	}
}

func TestCorruptCurrentIsAnErrorAndNeverFallsBack(t *testing.T) {
	for _, bad := range []string{`{not json`, `[]`, `null`, `{"language":1}`, `{"refreshIntervalSeconds":1.5}`} {
		s, cur, leg := newStore(bad, legacyJSON) // a valid legacy value must not rescue it
		p, _, err := s.LoadPreferences()
		if !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: err = %v, want ErrCorrupt", bad, err)
		}
		if !reflect.DeepEqual(p, Preferences{}) {
			t.Errorf("%s: returned %+v with an error", bad, p)
		}
		if cur.Writes() != 0 || leg.Writes() != 0 {
			t.Errorf("%s: something was written", bad)
		}
		if got, _, _ := cur.Read(KeyPreferences); string(got) != bad {
			t.Errorf("%s: stored bytes changed to %q", bad, got)
		}
	}
	// An empty value exists too: it is corrupt, not absent.
	cur := NewMemoryDomain()
	cur.Put(KeyPreferences, nil)
	if _, _, err := New(cur, NewMemoryDomain()).LoadPreferences(); !errors.Is(err, ErrCorrupt) {
		t.Errorf("empty value: err = %v", err)
	}
}

func TestCorruptLegacyIsAnErrorAndNothingIsWritten(t *testing.T) {
	s, cur, leg := newStore("", `{"language":1}`)
	_, _, err := s.LoadPreferences()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
	if cur.Writes() != 0 || leg.Writes() != 0 {
		t.Error("nothing may be written when the legacy value is corrupt")
	}
	if _, found, _ := cur.Read(KeyPreferences); found {
		t.Error("the current domain gained a key")
	}
}

// brokenDomain wraps a Domain and misbehaves on demand.
type brokenDomain struct {
	Domain
	failWrite, dropWrite bool
	readErr              error
}

func (d brokenDomain) Write(key string, data []byte) error {
	switch {
	case d.failWrite:
		return errors.New("disk full")
	case d.dropWrite:
		return nil
	}
	return d.Domain.Write(key, data)
}

func (d brokenDomain) Read(key string) ([]byte, bool, error) {
	if d.readErr != nil {
		return nil, false, d.readErr
	}
	return d.Domain.Read(key)
}

func TestMigrationFailuresAreErrors(t *testing.T) {
	leg := NewMemoryDomain()
	leg.Put(KeyPreferences, []byte(legacyJSON))

	failing := brokenDomain{Domain: NewMemoryDomain(), failWrite: true}
	if _, _, err := New(failing, leg).LoadPreferences(); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("write failure: err = %v", err)
	}
	dropping := brokenDomain{Domain: NewMemoryDomain(), dropWrite: true}
	if _, _, err := New(dropping, leg).LoadPreferences(); err == nil || !strings.Contains(err.Error(), "read-back") {
		t.Errorf("read-back mismatch: err = %v", err)
	}
	if _, found, _ := leg.Read(KeyPreferences); !found {
		t.Error("legacy value must survive a failed migration")
	}
	unreadable := brokenDomain{Domain: NewMemoryDomain(), readErr: ErrUnexpectedType}
	if _, _, err := New(unreadable, nil).LoadPreferences(); !errors.Is(err, ErrUnexpectedType) {
		t.Errorf("current read error: err = %v", err)
	}
	if _, _, err := New(NewMemoryDomain(), unreadable).LoadPreferences(); !errors.Is(err, ErrUnexpectedType) {
		t.Errorf("legacy read error: err = %v", err)
	}
}

func TestSavePreferencesKeepsUnknownKeys(t *testing.T) {
	s, cur, _ := newStore(currentJSON, "")
	p, _, err := s.LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	p.ShowsCostSummary = false
	if err := s.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	raw, _, _ := cur.Read(KeyPreferences)
	if !strings.Contains(string(raw), `"futureKey":{"n":1}`) || !strings.Contains(string(raw), `"showsCostSummary":false`) {
		t.Errorf("saved = %s", raw)
	}
	back, src, err := s.LoadPreferences()
	if err != nil || src != SourceCurrent || !reflect.DeepEqual(back, p) {
		t.Errorf("reload = %+v, %v, %v", back, src, err)
	}
}

func TestSaveInvalidPreferencesWritesNothing(t *testing.T) {
	cur := NewMemoryDomain()
	bad := DefaultPreferences()
	bad.Language = "klingon"
	if err := New(cur, nil).SavePreferences(bad); err == nil {
		t.Fatal("expected an error")
	}
	if cur.Writes() != 0 {
		t.Error("an invalid value was written")
	}
}

func TestProxyStore(t *testing.T) {
	cur := NewMemoryDomain()
	s := New(cur, NewMemoryDomain())

	if c, err := s.LoadNetworkProxy(); err != nil || !reflect.DeepEqual(c, DefaultNetworkProxy()) {
		t.Fatalf("absent: %+v %v", c, err)
	}
	want := NetworkProxyConfiguration{Mode: ProxyModeHTTP, Host: "proxy.local", Port: 8080, UsesAuthentication: true, CredentialID: testUUID}
	if err := s.SaveNetworkProxy(want); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadNetworkProxy(); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("reload: %+v %v", got, err)
	}
	// Proxy and general preferences are independent keys.
	if _, found, _ := cur.Read(KeyPreferences); found {
		t.Error("saving the proxy wrote preferences")
	}
}

func TestProxyStoreNeverFallsBackToDirect(t *testing.T) {
	for doc, wantErr := range map[string]error{
		`garbage`:                             ErrProxyInvalidConfiguration,
		`{"mode":"http","host":"h","port":1}`: ErrProxyInvalidConfiguration,
		`{"mode":"http","host":"h","port":0,"usesAuthentication":false}`:                      ErrProxyInvalidPort,
		`{"mode":"http","host":"a b","port":1,"usesAuthentication":false}`:                    ErrProxyInvalidHost,
		`{"mode":"http","host":"h","port":1,"usesAuthentication":true}`:                       ErrProxyCredentialsUnavailable,
		`{"mode":"http","host":"h","port":1,"usesAuthentication":true,"credentialID":"nope"}`: ErrProxyInvalidConfiguration,
	} {
		cur := NewMemoryDomain()
		cur.Put(KeyNetworkProxy, []byte(doc))
		c, err := New(cur, nil).LoadNetworkProxy()
		if !errors.Is(err, wantErr) {
			t.Errorf("%s: err = %v, want %v", doc, err, wantErr)
		}
		if !reflect.DeepEqual(c, NetworkProxyConfiguration{}) {
			t.Errorf("%s: returned %+v with an error", doc, c)
		}
		if cur.Writes() != 0 {
			t.Errorf("%s: wrote", doc)
		}
	}
	unreadable := brokenDomain{Domain: NewMemoryDomain(), readErr: ErrUnexpectedType}
	if _, err := New(unreadable, nil).LoadNetworkProxy(); !errors.Is(err, ErrProxyInvalidConfiguration) || !errors.Is(err, ErrUnexpectedType) {
		t.Errorf("non-data value: err = %v", err)
	}
}

func TestProxyHasNoLegacyMigration(t *testing.T) {
	cur, leg := NewMemoryDomain(), NewMemoryDomain()
	leg.Put(KeyNetworkProxy, []byte(`{"mode":"http","host":"h","port":1,"usesAuthentication":false}`))
	c, err := New(cur, leg).LoadNetworkProxy()
	if err != nil || !reflect.DeepEqual(c, DefaultNetworkProxy()) || cur.Writes() != 0 {
		t.Errorf("got %+v %v writes=%d", c, err, cur.Writes())
	}
}

func TestSaveInvalidProxyWritesNothing(t *testing.T) {
	cur := NewMemoryDomain()
	s := New(cur, nil)
	for _, c := range []NetworkProxyConfiguration{
		{Mode: ProxyModeHTTP, Host: "h", Port: 0},
		{Mode: ProxyModeSOCKS5, Host: "bad host", Port: 1},
		{Mode: ProxyModeHTTP, Host: "h", Port: 1, UsesAuthentication: true},
	} {
		if err := s.SaveNetworkProxy(c); err == nil {
			t.Errorf("%+v saved", c)
		}
	}
	if cur.Writes() != 0 {
		t.Error("an invalid configuration was written")
	}
}
