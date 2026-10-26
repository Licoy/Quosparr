package claude

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func coordinatedRig(t *testing.T) (*Module, string) {
	t.Helper()
	home := t.TempDir()
	s := NewStoreAt(filepath.Join(home, "accounts"), filepath.Join(home, "config"), home)
	m := NewModule(s)
	m.Now = func() time.Time { return time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC) }
	return m, seedAccount(t, s, "target", "target-org", false, m.now().Add(time.Hour))
}

func TestSwitchWithAfterRestoresAbsentLogin(t *testing.T) {
	m, id := coordinatedRig(t)
	fail := errors.New("desktop did not start")
	err := m.SwitchWithAfter(context.Background(), id, false, func() error {
		idx, e := m.Store.LoadIndex()
		if e != nil {
			return e
		}
		if idx.CurrentAccountID != id {
			t.Fatal("callback before CLI commit")
		}
		return fail
	})
	if !errors.Is(err, fail) || errors.Is(err, ErrSwitchRollback) {
		t.Fatal(err)
	}
	if _, e := m.Store.LoadOfficial(context.Background()); !errors.Is(e, ErrNotLoggedIn) {
		t.Fatal(e)
	}
	if _, e := os.Stat(m.configPath()); !errors.Is(e, os.ErrNotExist) {
		t.Fatal(e)
	}
	idx, e := m.Store.LoadIndex()
	if e != nil || idx.CurrentAccountID != "" {
		t.Fatal("current marker not restored", e)
	}
}

func TestSwitchWithAfterRestoresAPILogin(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	r.writeSettings(t, `{"theme":"dark","env":{"FOO":"bar"}}`)
	id := r.loginOAuth(t, "test-original-access", "original")
	x := r.addAPI(t, "", keyX, APIProfile{Model: "model", OpusModel: "opus"})
	if err := r.mod.Switch(ctx, x.ID, false); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(r.config, ".credentials.json"), r.mod.configPath(), r.mod.Store.SettingsPath()}
	before := make([][]byte, len(paths))
	for i, p := range paths {
		before[i] = fileBytes(t, p)
	}
	fail := errors.New("metadata save failed")
	if err := r.mod.SwitchWithAfter(ctx, id, false, func() error { return fail }); !errors.Is(err, fail) || errors.Is(err, ErrSwitchRollback) {
		t.Fatal(err)
	}
	for i, p := range paths {
		if !bytes.Equal(fileBytes(t, p), before[i]) {
			t.Fatal("snapshot not restored", filepath.Base(p))
		}
	}
	idx, err := r.mod.Store.LoadIndex()
	if err != nil || idx.CurrentAccountID != x.ID {
		t.Fatal("API current not restored", err)
	}
}

func TestSwitchWithAfterProtectsExternalChange(t *testing.T) {
	for _, kind := range []string{"official", "identity", "settings", "current"} {
		t.Run(kind, func(t *testing.T) {
			m, id := coordinatedRig(t)
			ctx := context.Background()
			other := []byte(`{"external":true}`)
			fail := errors.New("launch failed")
			err := m.SwitchWithAfter(ctx, id, false, func() error {
				var e error
				switch kind {
				case "official":
					e = m.Store.SaveOfficial(ctx, mustManaged(t, "external-access", "external-refresh", m.now().Add(time.Hour)))
				case "identity":
					e = os.WriteFile(m.configPath(), other, 0o600)
				case "settings":
					e = os.WriteFile(m.Store.SettingsPath(), other, 0o600)
				case "current":
					e = m.Store.SetCurrentAccountID("", m.now())
				}
				if e != nil {
					t.Fatal(e)
				}
				return fail
			})
			if !errors.Is(err, ErrSwitchRollback) || !errors.Is(err, fail) {
				t.Fatal(err)
			}
			if kind == "settings" && !bytes.Equal(fileBytes(t, m.Store.SettingsPath()), other) {
				t.Fatal("external settings overwritten")
			}

			if kind == "identity" && !bytes.Equal(fileBytes(t, m.configPath()), other) {
				t.Fatal("external identity overwritten")
			}
		})
	}
}

func TestSwitchWithAfterKeepsRotatedTarget(t *testing.T) {
	m, id := coordinatedRig(t)
	seedAccount(t, m.Store, "target", "target-org", false, m.now().Add(-time.Hour))
	srv := tokenServer(t)
	defer srv.Close()
	m.Tokens = RefreshClient{HTTP: srv.Client(), Primary: srv.URL + "/token", Fallback: srv.URL + "/unused"}
	fail := errors.New("launch failed")
	if err := m.SwitchWithAfter(context.Background(), id, false, func() error { return fail }); !errors.Is(err, fail) || errors.Is(err, ErrSwitchRollback) {
		t.Fatal(err)
	}
	if readCred(t, m.Store, id).OAuth().RefreshToken != "test-refresh-token-2" {
		t.Fatal("rotated refresh token lost")
	}
}

func TestSwitchWithAfterSuccessAndGuards(t *testing.T) {
	m, id := coordinatedRig(t)
	called := false
	m.RunningProcessIDs = func(context.Context) ([]int, error) { return []int{123}, nil }
	if err := m.SwitchWithAfter(context.Background(), id, false, func() error { called = true; return nil }); err == nil || called {
		t.Fatal("running guard bypassed")
	}
	m.RunningProcessIDs = nil
	if err := m.SwitchWithAfter(context.Background(), id, false, func() error { called = true; return nil }); err != nil || !called {
		t.Fatal(err)
	}
	idx, e := m.Store.LoadIndex()
	if e != nil || idx.CurrentAccountID != id {
		t.Fatal(e)
	}
	if err := m.SwitchWithAfter(context.Background(), id, false, nil); err == nil {
		t.Fatal("nil callback accepted")
	}
	r := newAPIRig(t)
	api := r.addAPI(t, "", keyX, APIProfile{})
	if err := r.mod.SwitchWithAfter(context.Background(), api.ID, false, func() error { return nil }); !errors.Is(err, ErrAPIAccount) {
		t.Fatal(err)
	}
}

func TestSwitchWithAfterRestoresRealPreviousOAuth(t *testing.T) {
	m, target := coordinatedRig(t)
	ctx := context.Background()
	previous := seedAccount(t, m.Store, "previous", "previous-org", true, m.now().Add(time.Hour))
	original := mustManaged(t, "actually-current-access", "actually-current-refresh", m.now().Add(time.Hour))
	if err := m.Store.SaveOfficial(ctx, original); err != nil {
		t.Fatal(err)
	}
	identity := []byte(`{"projects":{"keep":true},"oauthAccount":{"accountUuid":"previous","organizationUuid":"previous-org"}}`)
	if err := os.WriteFile(m.configPath(), identity, 0o600); err != nil {
		t.Fatal(err)
	}
	fail := errors.New("launch failed")
	if err := m.SwitchWithAfter(ctx, target, false, func() error { return fail }); !errors.Is(err, fail) || errors.Is(err, ErrSwitchRollback) {
		t.Fatal(err)
	}
	restored, err := m.Store.LoadOfficial(ctx)
	if err != nil || !bytes.Equal(restored, original) {
		t.Fatal("did not restore actual official bytes", err)
	}
	if !bytes.Equal(fileBytes(t, m.configPath()), identity) {
		t.Fatal("identity changed")
	}
	idx, err := m.Store.LoadIndex()
	if err != nil || idx.CurrentAccountID != previous {
		t.Fatal("current not restored", err)
	}
	if readCred(t, m.Store, previous).OAuth().RefreshToken != "actually-current-refresh" {
		t.Fatal("actual current managed token lost")
	}
}

type coordinatedFailStore struct {
	FileStore
	saves   int
	failure error
}

func (s *coordinatedFailStore) Save(ctx context.Context, b []byte) error {
	s.saves++
	if s.saves == 2 {
		return s.failure
	}
	return s.FileStore.Save(ctx, b)
}

func TestSwitchWithAfterReportsRollbackSaveFailure(t *testing.T) {
	m, id := coordinatedRig(t)
	ctx := context.Background()
	if err := m.Store.SaveOfficial(ctx, mustManaged(t, "original-access", "original-refresh", m.now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("fake restore store unavailable")
	m.Store.Official = &coordinatedFailStore{FileStore: FileStore{Path: CredentialsFilePath(m.Store.ConfigDir())}, failure: failure}
	launch := errors.New("launch failed")
	err := m.SwitchWithAfter(ctx, id, false, func() error { return launch })
	if !errors.Is(err, ErrSwitchRollback) || !errors.Is(err, failure) || !errors.Is(err, launch) {
		t.Fatal(err)
	}
}

func TestSwitchWithAfterSerializesCallbacks(t *testing.T) {
	m, id := coordinatedRig(t)
	entered, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- m.SwitchWithAfter(context.Background(), id, false, func() error { close(entered); <-release; return nil })
	}()
	<-entered
	second := make(chan error, 1)
	go func() { second <- m.SwitchWithAfter(context.Background(), id, false, func() error { return nil }) }()
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
}

type partialCoordinatedStore struct {
	FileStore
	first   bool
	unknown []byte
	failure error
}

func (s *partialCoordinatedStore) Save(ctx context.Context, b []byte) error {
	if !s.first {
		return s.FileStore.Save(ctx, b)
	}
	s.first = false
	if s.unknown != nil {
		b = s.unknown
	}
	if err := s.FileStore.Save(ctx, b); err != nil {
		return err
	}
	return s.failure
}

func TestSwitchWithAfterHandlesSaveErrorAfterPartialWrite(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(strconv.FormatBool(unknown), func(t *testing.T) {
			m, id := coordinatedRig(t)
			failure := errors.New("fake credential write verification failed")
			partial := &partialCoordinatedStore{FileStore: FileStore{Path: CredentialsFilePath(m.Store.ConfigDir())}, first: true, failure: failure}
			if unknown {
				partial.unknown = mustManaged(t, "unknown-access", "unknown-refresh", m.now().Add(time.Hour))
			}
			m.Store.Official = partial
			called := false
			err := m.SwitchWithAfter(context.Background(), id, false, func() error { called = true; return nil })
			if !errors.Is(err, failure) || called {
				t.Fatal(err)
			}
			if unknown {
				if !errors.Is(err, ErrSwitchRollback) {
					t.Fatal("unrecognized partial save lacked rollback classification", err)
				}
				b, e := partial.Load(context.Background())
				if e != nil || !bytes.Equal(b, partial.unknown) {
					t.Fatal("unrecognized state overwritten", e)
				}
			} else {
				if errors.Is(err, ErrSwitchRollback) {
					t.Fatal(err)
				}
				if _, e := partial.Load(context.Background()); !errors.Is(e, ErrNotLoggedIn) {
					t.Fatal("partial save not restored", e)
				}
			}
		})
	}
}
