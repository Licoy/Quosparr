package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

func TestCodexPasteReportsCreatedAndUpdated(t *testing.T) {
	home := t.TempDir()
	st := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "library"))
	svc := runway.New(runway.Options{Sources: runway.Sources{LoadIndex: st.LoadIndex}})
	t.Cleanup(svc.Close)
	rt := &liveApp{codex: st, svc: svc}
	l := l10n.New(l10n.English)
	text := `{"OPENAI_API_KEY":"synthetic-import-result"}`
	for _, expected := range []string{"Added 1 account(s), updated 0 account(s)", "Added 0 account(s), updated 1 account(s)"} {
		message, ok := rt.pasteCodex(l, text)
		if !ok || message != expected {
			t.Fatalf("unexpected import outcome: ok=%v message=%q", ok, message)
		}
	}
}

func TestCodexPasteReloadFailureKeepsSavedAccounts(t *testing.T) {
	home := t.TempDir()
	st := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "library"))
	if _, err := st.Upsert(codex.APIKeyAuth("synthetic-existing"), codex.UpsertOptions{}); err != nil {
		t.Fatal(err)
	}
	svc := runway.New(runway.Options{Sources: runway.Sources{LoadIndex: func() (codex.AccountIndex, error) { return codex.AccountIndex{}, errors.New("private-index-path") }}})
	t.Cleanup(svc.Close)
	rt := &liveApp{codex: st, svc: svc}
	message, ok := rt.pasteCodex(l10n.New(l10n.English), `{"OPENAI_API_KEY":"synthetic-second"}`)
	if ok || !strings.Contains(message, "Changes were saved, but the account list could not be reloaded") || strings.Contains(message, "private-index-path") {
		t.Fatalf("reload failure hidden or exposed: ok=%v message=%q", ok, message)
	}
	idx, err := st.LoadIndex()
	if err != nil || len(idx.Accounts) != 2 {
		t.Fatal("reload failure lost saved accounts")
	}
}

// Exercise the real callback, token exchange, store write, list reload and UI notice.
func TestCodexLoginNoticeUsesSavedIdentityAndReloadOutcome(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	home := t.TempDir()
	st := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "library"))
	reloadFails := false
	svc := runway.New(runway.Options{Sources: runway.Sources{LoadIndex: func() (codex.AccountIndex, error) {
		if reloadFails {
			return codex.AccountIndex{}, &os.PathError{Op: "read", Path: "private-index-path", Err: os.ErrPermission}
		}
		return st.LoadIndex()
	}}})
	t.Cleanup(svc.Close)
	rt := &liveApp{codex: st, svc: svc}
	for _, step := range []struct {
		user       string
		created    bool
		failReload bool
		count      int
	}{
		{"user-a", true, false, 1}, {"user-b", true, false, 2}, {"user-b", false, false, 2}, {"user-b", false, true, 2},
	} {
		reloadFails = step.failReload
		payload, err := json.Marshal(map[string]any{"email": step.user + "@example.invalid", "https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "shared-workspace", "chatgpt_user_id": step.user}})
		if err != nil {
			t.Fatal(err)
		}
		jwt := "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".synthetic"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": jwt, "access_token": "synthetic-access", "refresh_token": "synthetic-refresh"})
		}))
		flow := codexLoginFlow{store: st, port: oauthTestPort(t), login: codexapi.OAuthLogin{HTTP: server.Client(), TokenURL: server.URL}, openBrowser: func(_ context.Context, raw string) error {
			auth, err := url.Parse(raw)
			if err != nil {
				return err
			}
			response, err := http.Get(auth.Query().Get("redirect_uri") + "?code=synthetic-code&state=" + url.QueryEscape(auth.Query().Get("state")))
			if err == nil {
				response.Body.Close()
			}
			return err
		}}
		u.notice, u.noticeOpen = "", false
		u.startCodexLoginFlow(f, rt, flow)
		deadline := time.Now().Add(3 * time.Second)
		for f.busy && time.Now().Before(deadline) {
			e.q.drain()
			time.Sleep(time.Millisecond)
		}
		server.Close()
		if f.busy || !u.noticeOpen {
			t.Fatal("login did not complete with a notice")
		}
		expected := tx(u.L(), xCodexImportResult, 0, 1)
		if step.created {
			expected = tx(u.L(), xCodexImportResult, 1, 0)
		}
		if step.failReload {
			expected = tx(u.L(), xCodexSavedReloadFailed)
		}
		if !strings.Contains(u.notice, expected) || strings.Contains(u.notice, "private-index-path") {
			t.Fatalf("wrong login notice: %q", u.notice)
		}
		idx, err := st.LoadIndex()
		if err != nil || len(idx.Accounts) != step.count {
			t.Fatal("login changed the wrong stored identity")
		}
		if !step.failReload && len(svc.Snapshot().Accounts.Sidebar) != step.count {
			t.Fatal("saved identities not shown in account list")
		}
	}
}

func TestCodexSelectionKeepsPartialFailuresWhenReloadFails(t *testing.T) {
	home := t.TempDir()
	st := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "library"))
	svc := runway.New(runway.Options{Sources: runway.Sources{LoadIndex: func() (codex.AccountIndex, error) { return codex.AccountIndex{}, os.ErrPermission }}})
	t.Cleanup(svc.Close)
	rt := &liveApp{codex: st, svc: svc}
	selection := &accountSelection{codex: []codex.ImportCandidate{{Preview: codex.PreviewItem{ID: "valid"}, Auth: selectionAuthFixture()}}, failed: 1, causes: []error{os.ErrNotExist}}
	l := l10n.New(l10n.English)
	message, ok := rt.commitAccountSelection(l, selection, []string{"valid"}, accountKindCodex)
	if ok {
		t.Fatal("partial failed reload reported success")
	}
	for _, expected := range []string{tx(l, xAccountBatchResult, 1, 1), accountErrorReason(l, os.ErrNotExist), tx(l, xCodexSavedReloadFailed), accountErrorReason(l, os.ErrPermission)} {
		if !strings.Contains(message, expected) {
			t.Fatalf("lost partial/reload failure: %q", message)
		}
	}
}

func TestCodexImportReloadRunsWithoutSuccessfulEntries(t *testing.T) {
	home := t.TempDir()
	st := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "library"))
	loads := 0
	svc := runway.New(runway.Options{Sources: runway.Sources{LoadIndex: func() (codex.AccountIndex, error) {
		loads++
		return codex.AccountIndex{}, os.ErrPermission
	}}})
	t.Cleanup(svc.Close)
	loads = 0
	rt := &liveApp{codex: st, svc: svc}
	l := l10n.New(l10n.English)
	message, ok := rt.pasteCodex(l, `{}`)
	if ok || loads != 1 || strings.Contains(message, tx(l, xCodexSavedReloadFailed)) {
		t.Fatal("failed imports must reload without claiming changes were saved")
	}
	for _, expected := range []string{tx(l, xAccountBatchResult, 0, 1), l.Text(l10n.KeyAccountsReloadFailed), accountErrorReason(l, os.ErrPermission)} {
		if !strings.Contains(message, expected) {
			t.Fatalf("lost import or reload failure: %q", message)
		}
	}
}

func TestCodexAPIApplyStillInvalidatesAndRefreshesAfterReloadFailure(t *testing.T) {
	home := t.TempDir()
	st := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "library"))
	account, err := st.Upsert(codex.APIKeyAuth("synthetic-api-in-use"), codex.UpsertOptions{MakeActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&codex.Switcher{Store: st}).SwitchTo(context.Background(), account.ID); err != nil {
		t.Fatal(err)
	}
	refreshed := make(chan struct{}, 1)
	svc := runway.New(runway.Options{Preferences: quietLivePreferences(), Sources: runway.Sources{
		LoadIndex: func() (codex.AccountIndex, error) {
			return codex.AccountIndex{}, &os.PathError{Op: "read", Path: "private-reload-path", Err: os.ErrPermission}
		},
		LoadValidAuth: func(context.Context, bool, *codex.Auth) (codex.Auth, error) {
			refreshed <- struct{}{}
			return codex.Auth{}, errors.New("synthetic-stop-refresh")
		},
	}})
	t.Cleanup(svc.Close)
	rt := &liveApp{codex: st, svc: svc}
	err = rt.updateCodexAPIAccount(context.Background(), account.ID, CodexAPIInput{APIKey: "synthetic-api-replaced", Apply: true})
	var reload *codexReloadError
	if !errors.As(err, &reload) {
		t.Fatal("reload failure was not returned after apply")
	}
	if svc.Snapshot().Accounts.Runtime.CheckedAt.IsZero() {
		t.Fatal("apply skipped runtime invalidation after reload failure")
	}
	select {
	case <-refreshed:
	case <-time.After(time.Second):
		t.Fatal("apply skipped credential refresh after reload failure")
	}
	official, err := st.LoadOfficialAuth()
	if err != nil || official.APIKey() != "synthetic-api-replaced" {
		t.Fatal("apply did not persist edited official auth")
	}
	l := l10n.New(l10n.English)
	message := apiErrorText(l, reload)
	if !strings.Contains(message, tx(l, xCodexSavedReloadFailed)) || strings.Contains(message, "private-reload-path") {
		t.Fatal("API reload failure notice lost saved outcome or leaked details")
	}
}
