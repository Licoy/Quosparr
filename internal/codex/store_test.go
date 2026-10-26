package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRemoveInactiveAccountKeepsOfficialAndOthers(t *testing.T) {
	e := newEnv(t)
	current := sampleAuth(t, sampleOpt{accountID: "current", email: "current@example.com", refresh: "refresh-current"})
	cur := upsert(t, e.store, current, true)
	removed := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "removed", email: "removed@example.com", refresh: "refresh-removed"}), false)
	e.writeOfficial(t, current)
	before := e.officialBytes(t)

	noErr(t, e.store.DeleteAccount(removed.ID))

	if !bytes.Equal(e.officialBytes(t), before) {
		t.Error("deleting a managed account must not touch the official auth.json")
	}
	idx := must(e.store.LoadIndex())
	if len(idx.Accounts) != 1 || idx.Accounts[0].ID != cur.ID || idx.ActiveAccountID != cur.ID {
		t.Fatalf("index after delete: %+v", idx)
	}
	if _, err := os.Stat(e.store.CredentialPath(removed.ID)); !os.IsNotExist(err) {
		t.Error("removed credential still on disk")
	}
	if _, err := os.Stat(e.store.CredentialPath(cur.ID)); err != nil {
		t.Error("current credential is gone")
	}
}

func TestUpsertStoresCredentialsRestricted(t *testing.T) {
	e := newEnv(t)
	a := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "acct-a", email: "a@example.com", refresh: "refresh-a"}), true)
	b := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "acct-b", email: "b@example.com", refresh: "refresh-b"}), false)
	if a.AccountID != "acct-a" || a.ID == b.ID || b.Email != "b@example.com" {
		t.Fatalf("accounts: %+v %+v", a, b)
	}
	idx := must(e.store.LoadIndex())
	if len(idx.Accounts) != 2 || idx.ActiveAccountID != a.ID {
		t.Fatalf("index: %+v", idx)
	}
	loaded := must(e.store.LoadCredential(a.ID))
	if !strings.HasPrefix(loaded.Tokens.RefreshToken, "refresh-a") || strings.Contains(loaded.Redacted(), "refresh-a") {
		t.Fatal("credential not stored or not redacted")
	}
	// Setting active only edits the index, never the official file.
	noErr(t, e.store.SetActiveAccount(b.ID, testNow))
	if must(e.store.LoadIndex()).ActiveAccountID != b.ID {
		t.Fatal("active account not updated")
	}
	if _, err := os.Stat(e.officialPath()); !os.IsNotExist(err) {
		t.Fatal("SetActiveAccount created the official auth.json")
	}
}

func TestFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are not meaningful on Windows")
	}
	e := newEnv(t)
	acc := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "perm", email: "p@example.com", refresh: "refresh-perm"}), true)
	mode := func(path string) os.FileMode { return must(os.Stat(path)).Mode().Perm() }
	checks := []struct {
		what string
		path string
		want os.FileMode
	}{
		{"accounts root", e.store.Root(), 0o700},
		{"index.json", e.store.IndexPath(), 0o600},
		{"account dir", e.store.AccountDir(acc.ID), 0o700},
		{"credential", e.store.CredentialPath(acc.ID), 0o600},
	}
	for _, c := range checks {
		if got := mode(c.path); got != c.want {
			t.Errorf("%s mode = %o, want %o", c.what, got, c.want)
		}
	}
}

func TestInjectedPathsLayout(t *testing.T) {
	e := newEnv(t)
	if e.store.Root() != filepath.Join(e.runwayHome, "accounts") {
		t.Errorf("root = %s", e.store.Root())
	}
	if e.store.OfficialAuthPath() != filepath.Join(e.codexHome, "auth.json") {
		t.Errorf("official = %s", e.store.OfficialAuthPath())
	}
	acc := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "x", email: "x@example.com", refresh: "refresh-x"}), true)
	if e.store.CredentialPath(acc.ID) != filepath.Join(e.runwayHome, "accounts", acc.ID, "auth.json") {
		t.Errorf("credential path = %s", e.store.CredentialPath(acc.ID))
	}
}

func TestIndexNeverContainsTokens(t *testing.T) {
	e := newEnv(t)
	a := sampleAuth(t, sampleOpt{accountID: "tok", email: "t@example.com", refresh: "refresh-secret-value-123"})
	upsert(t, e.store, a, true)
	data := must(os.ReadFile(e.store.IndexPath()))
	for _, secret := range []string{a.Tokens.RefreshToken, a.Tokens.AccessToken, a.Tokens.IDToken} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatal("index.json contains a token")
		}
	}
}

func TestIndexPreservesUnknownFields(t *testing.T) {
	e := newEnv(t)
	acc := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "u", email: "u@example.com", refresh: "refresh-unknown"}), true)
	data := must(os.ReadFile(e.store.IndexPath()))
	var raw map[string]any
	noErr(t, json.Unmarshal(data, &raw))
	raw["futureTopLevel"] = map[string]any{"a": 1}
	raw["accounts"].([]any)[0].(map[string]any)["futureRowField"] = "keep"
	edited := must(json.Marshal(raw))
	noErr(t, os.WriteFile(e.store.IndexPath(), edited, 0o600))

	// Any rewrite through the store must keep both unknown fields.
	noErr(t, e.store.SetActiveAccount(acc.ID, testNow))
	upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "u", email: "u@example.com", refresh: "refresh-unknown-rotated"}), false)

	out := string(must(os.ReadFile(e.store.IndexPath())))
	if !strings.Contains(out, "futureTopLevel") || !strings.Contains(out, "futureRowField") {
		t.Fatalf("unknown fields dropped:\n%s", out)
	}
}

func TestDeduplicatesByIdentityOnReimport(t *testing.T) {
	e := newEnv(t)
	first := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "same", email: "same@example.com", refresh: "rt-1"}), true)
	second := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "same", email: "same@example.com", refresh: "rt-2"}), false)
	if first.ID != second.ID || len(must(e.store.LoadIndex()).Accounts) != 1 {
		t.Fatal("same identity must update one account")
	}
	if !strings.HasPrefix(must(e.store.LoadCredential(first.ID)).Tokens.RefreshToken, "rt-2") {
		t.Fatal("credential not replaced")
	}
}

func TestSameWorkspaceDifferentUsersStaySeparate(t *testing.T) {
	e := newEnv(t)
	first := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "workspace-shared", email: "member-a@example.com", refresh: "refresh-member-a", userID: "user-Aa1"}), true)
	second := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "workspace-shared", email: "member-b@example.com", refresh: "refresh-member-b", userID: "user-Bb2"}), false)
	if first.ID == second.ID || first.UserID != "user-Aa1" || second.UserID != "user-Bb2" {
		t.Fatalf("accounts: %+v %+v", first, second)
	}
	if first.IdentityMarkerUserID() == second.IdentityMarkerUserID() {
		t.Fatal("identity markers should differ")
	}
	if len(must(e.store.LoadIndex()).Accounts) != 2 {
		t.Fatal("expected two accounts")
	}
	if !strings.HasPrefix(must(e.store.LoadCredential(first.ID)).Tokens.RefreshToken, "refresh-member-a") ||
		!strings.HasPrefix(must(e.store.LoadCredential(second.ID)).Tokens.RefreshToken, "refresh-member-b") {
		t.Fatal("credentials crossed")
	}
}

func TestOneUserDifferentWorkspacesStaySeparate(t *testing.T) {
	e := newEnv(t)
	mk := func(ws string) Auth {
		return sampleAuth(t, sampleOpt{accountID: ws, email: "member@example.com", refresh: "refresh-shared-workspaces", userID: "user-shared", claimAccountID: "workspace-stale-claim"})
	}
	first, second := upsert(t, e.store, mk("workspace-one"), false), upsert(t, e.store, mk("workspace-two"), false)
	if first.ID == second.ID || first.UserID != second.UserID || first.AccountID == second.AccountID ||
		first.IdentityMarkerWorkspaceID() == second.IdentityMarkerWorkspaceID() {
		t.Fatalf("accounts: %+v %+v", first, second)
	}
	if must(e.store.LoadCredential(first.ID)).Tokens.AccountID != "workspace-one" ||
		must(e.store.LoadCredential(second.ID)).Tokens.AccountID != "workspace-two" {
		t.Fatal("credentials crossed")
	}
}

func TestSameUserAndWorkspaceUpdatesInPlace(t *testing.T) {
	e := newEnv(t)
	first := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "workspace-one", email: "member@example.com", refresh: "refresh-original-token", userID: "user-one"}), false)
	second := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "workspace-one", email: "renamed@example.com", refresh: "refresh-rotated-token", userID: "user-one"}), false)
	if first.ID != second.ID || len(must(e.store.LoadIndex()).Accounts) != 1 {
		t.Fatal("expected one account")
	}
	if must(e.store.LoadCredential(first.ID)).Tokens.RefreshToken != "refresh-rotated-token" {
		t.Fatal("credential not rotated")
	}
}

func TestSubjectFallbackUpgradesWithoutDuplication(t *testing.T) {
	e := newEnv(t)
	legacyAuth := sampleAuth(t, sampleOpt{accountID: "workspace-one", email: "member@example.com", refresh: "refresh-before-user-id", subject: "legacy-subject"})
	legacy := ManagedAccount{ID: "legacy-subject-row", Email: "member@example.com", UserID: "legacy-subject", AccountID: "workspace-one", DisplayName: "member@example.com", AuthMode: AuthModeOAuth, CreatedAt: testNow}
	noErr(t, e.store.SaveCredential(legacy.ID, legacyAuth))
	idx := NewAccountIndex()
	idx.ActiveAccountID = legacy.ID
	idx.Accounts = []ManagedAccount{legacy}
	noErr(t, e.store.SaveIndex(idx))

	migrated, _ := must(e.store.LoadIndex()).Account(legacy.ID)
	if migrated.UserID != "" || migrated.SubjectID != "legacy-subject" {
		t.Fatalf("hydration: user=%q subject=%q", migrated.UserID, migrated.SubjectID)
	}
	mk := func(refresh, user, subject string) Auth {
		return sampleAuth(t, sampleOpt{accountID: "workspace-one", email: "member@example.com", refresh: refresh, userID: user, subject: subject})
	}
	upgraded := upsert(t, e.store, mk("refresh-after-user-id", "user-one", "legacy-subject"), false)
	if upgraded.ID != legacy.ID || upgraded.UserID != "user-one" || len(must(e.store.LoadIndex()).Accounts) != 1 {
		t.Fatalf("upgrade: %+v", upgraded)
	}
	again := upsert(t, e.store, mk("refresh-without-explicit-user", "", "legacy-subject"), false)
	if again.ID != upgraded.ID || again.UserID != "user-one" || len(must(e.store.LoadIndex()).Accounts) != 1 {
		t.Fatalf("re-import: %+v", again)
	}
	other := upsert(t, e.store, mk("refresh-different-subject", "user-two", "different-subject"), false)
	if other.ID == upgraded.ID || len(must(e.store.LoadIndex()).Accounts) != 2 {
		t.Fatal("a different user and subject must be a new account")
	}
	if must(e.store.LoadCredential(upgraded.ID)).Tokens.RefreshToken != "refresh-without-explicit-user" {
		t.Fatal("original credential overwritten")
	}
	subjectOnly := upsert(t, e.store, mk("refresh-different-subject-only", "", "subject-without-explicit-user"), false)
	if subjectOnly.ID == upgraded.ID || len(must(e.store.LoadIndex()).Accounts) != 3 {
		t.Fatal("a different subject must be a new account")
	}
}

func TestLegacyHydrationKeepsRowAndCredentialInPlace(t *testing.T) {
	e := newEnv(t)
	auth := sampleAuth(t, sampleOpt{accountID: "workspace-selected", email: "legacy@example.com", refresh: "refresh-legacy-token", userID: "legacy-user", workspaceName: "Unrelated Organization Title"})
	legacy := ManagedAccount{ID: "legacy-row-id", SortIndex: 7, Email: "legacy@example.com", AccountID: "stale-claim-workspace", DisplayName: "Legacy", Alias: "Keep me", Note: "Keep note", AuthMode: AuthModeOAuth, CreatedAt: testNow}
	noErr(t, e.store.SaveCredential(legacy.ID, auth))
	idx := NewAccountIndex()
	idx.ActiveAccountID = legacy.ID
	idx.Accounts = []ManagedAccount{legacy}
	noErr(t, e.store.SaveIndex(idx))

	h, _ := must(e.store.LoadIndex()).Account(legacy.ID)
	if h.UserID != "legacy-user" || h.AccountID != "workspace-selected" || h.WorkspaceName != "" ||
		h.Alias != "Keep me" || h.Note != "Keep note" || h.SortIndex != 7 || !h.CreatedAt.Equal(testNow) {
		t.Fatalf("hydrated row: %+v", h)
	}
	updated := upsert(t, e.store, auth, false)
	after := must(e.store.LoadIndex())
	if updated.ID != legacy.ID || after.ActiveAccountID != legacy.ID || len(after.Accounts) != 1 {
		t.Fatalf("after upsert: %+v", after)
	}
	if _, err := os.Stat(e.store.CredentialPath(legacy.ID)); err != nil {
		t.Fatal("credential moved")
	}
}

func TestLegacyHydrationFlagsBrokenCredentials(t *testing.T) {
	tests := []struct {
		name      string
		write     func(t *testing.T, e testEnv, id string)
		wantError string
	}{
		{"missing", func(*testing.T, testEnv, string) {}, "credential_missing"},
		{"invalid", func(t *testing.T, e testEnv, id string) {
			noErr(t, os.MkdirAll(e.store.AccountDir(id), 0o700))
			noErr(t, os.WriteFile(e.store.CredentialPath(id), []byte("not valid auth json"), 0o600))
		}, "invalid_credential"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			idx := NewAccountIndex()
			idx.Accounts = []ManagedAccount{{ID: "broken", DisplayName: "Broken", AuthMode: AuthModeOAuth, CreatedAt: testNow}}
			noErr(t, e.store.SaveIndex(idx))
			tc.write(t, e, "broken")
			got, _ := must(e.store.LoadIndex()).Account("broken")
			if !got.RequiresReauth || got.LastError != tc.wantError {
				t.Fatalf("row: reauth=%v err=%q", got.RequiresReauth, got.LastError)
			}
		})
	}
}

func TestWeakIdentitiesDoNotMergeByWorkspace(t *testing.T) {
	e := newEnv(t)
	weak := func(refresh string) Auth {
		return Auth{AuthMode: "chatgpt", Tokens: Tokens{
			IDToken: jwtOf(t, nil), AccessToken: jwtExp(t, farFuture), RefreshToken: refresh, AccountID: "workspace-only",
		}}
	}
	first, second := weak("weak-refresh-token-first"), weak("weak-refresh-token-second")
	a := upsert(t, e.store, first, false)
	b := upsert(t, e.store, second, false)
	again := upsert(t, e.store, first, false)
	if a.ID == b.ID || again.ID != a.ID || len(must(e.store.LoadIndex()).Accounts) != 2 {
		t.Fatal("weak identities must stay separate")
	}
}

func TestAmbiguousOrganizationsNeverOverwrite(t *testing.T) {
	e := newEnv(t)
	id := jwtOf(t, map[string]any{"email": "member@example.com", "https://api.openai.com/auth": map[string]any{
		"chatgpt_user_id": "user-shared",
		"organizations": orgs(
			map[string]any{"id": "org-first", "is_default": false},
			map[string]any{"id": "org-second", "is_default": false}),
	}})
	first := Auth{AuthMode: "chatgpt", Tokens: Tokens{IDToken: id, AccessToken: jwtExp(t, farFuture), RefreshToken: "ambiguous-refresh-token-one"}}
	second := first
	second.Tokens.RefreshToken = "ambiguous-refresh-token-two"
	a, b := upsert(t, e.store, first, false), upsert(t, e.store, second, false)
	if a.ID == b.ID || len(must(e.store.LoadIndex()).Accounts) != 2 {
		t.Fatal("ambiguous organizations must not merge")
	}
	if must(e.store.LoadCredential(a.ID)).Tokens.RefreshToken != "ambiguous-refresh-token-one" {
		t.Fatal("first credential overwritten")
	}
}

func TestWithIdentityPrefersLivePlan(t *testing.T) {
	pro := sampleAuth(t, sampleOpt{accountID: "acct", email: "a@example.com", refresh: "r1", plan: "pro"})
	acc := NewManagedAccount("", pro, 0, "", "", "", testNow)
	if acc.Tier() != TierPro200 {
		t.Fatalf("tier = %s", acc.Tier())
	}
	free := sampleAuth(t, sampleOpt{accountID: "acct", email: "a@example.com", refresh: "r2", plan: "free"})
	acc = acc.WithIdentity(free, "", testNow)
	if !strings.Contains(strings.ToLower(acc.PlanType), "free") || acc.Tier() != TierFree {
		t.Fatalf("plan = %q tier = %s", acc.PlanType, acc.Tier())
	}
}

func TestWithIdentityKeepsWorkspaceName(t *testing.T) {
	auth := sampleAuth(t, sampleOpt{accountID: "workspace-cached", email: "cached@example.com", refresh: "refresh-cached-workspace", plan: "team", userID: "user-cached"})
	acc := NewManagedAccount("", auth, 0, "", "", "", testNow)
	acc.WorkspaceName = "Cached Workspace"
	r := acc.WithIdentity(auth, "team", testNow)
	if r.WorkspaceName != "Cached Workspace" || !r.DisplaysWorkspaceIdentity() ||
		r.IdentityMarkerUserID() != "user-cached" || r.IdentityMarkerWorkspaceID() != "workspace-cached" {
		t.Fatalf("row: %+v", r)
	}
	personal := NewManagedAccount("", sampleAuth(t, sampleOpt{accountID: "personal-account", email: "personal@example.com", refresh: "refresh-personal-account", plan: "pro", userID: "personal-user"}), 0, "", "", "", testNow)
	if personal.DisplaysWorkspaceIdentity() || personal.IdentityMarkerUserID() != "personal-user" {
		t.Fatalf("personal row: %+v", personal)
	}
}

func TestSidebarOrder(t *testing.T) {
	idx := NewAccountIndex()
	idx.ActiveAccountID = "b"
	idx.Accounts = []ManagedAccount{{ID: "a", SortIndex: 0, DisplayName: "A"}, {ID: "b", SortIndex: 2, DisplayName: "B"}, {ID: "c", SortIndex: 1, DisplayName: "C"}}
	var ids []string
	for _, a := range idx.OrderedForSidebar() {
		ids = append(ids, a.ID)
	}
	if strings.Join(ids, ",") != "b,a,c" {
		t.Fatalf("order = %v", ids)
	}
	idx.ReindexSortOrder([]string{"c", "a", "b"})
	c, _ := idx.Account("c")
	a, _ := idx.Account("a")
	if c.SortIndex != 0 || a.SortIndex != 1 {
		t.Fatalf("sort indexes: c=%d a=%d", c.SortIndex, a.SortIndex)
	}
}

func TestAPIKeyAccount(t *testing.T) {
	e := newEnv(t)
	im := &Importer{Store: e.store, Now: func() time.Time { return testNow }}
	acc := must(im.ImportAPIKey("  sk-test-key-1234567890\n", false))
	if acc.AuthMode != AuthModeAPIKey || acc.Tier() != TierAPI {
		t.Fatalf("account: %+v", acc)
	}
	auth := must(e.store.LoadCredential(acc.ID))
	if !auth.IsAPIKey() || auth.APIKey() != "sk-test-key-1234567890" || strings.Contains(auth.Redacted(), "sk-test") {
		t.Fatal("api key credential wrong")
	}
	if _, err := im.ImportAPIKey("   ", false); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("blank key: %v", err)
	}
}

func TestSessionAccessTokenOnlyUsability(t *testing.T) {
	access := jwtOf(t, map[string]any{"email": "session@example.com", "exp": farFuture,
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "sess-1", "chatgpt_plan_type": "free"}})
	auth := Auth{AuthMode: "chatgpt", Tokens: Tokens{IDToken: access, AccessToken: access, AccountID: "sess-1"}}
	if !auth.IsAccessTokenOnly() || auth.LoginUsability(testNow) != Usable {
		t.Fatal("valid session should be usable")
	}
	expired := jwtOf(t, map[string]any{"exp": 1_700_000_000, "email": "x@y.z"})
	exp := Auth{AuthMode: "chatgpt", Tokens: Tokens{IDToken: expired, AccessToken: expired, AccountID: "sess-1"}}
	if exp.LoginUsability(testNow) != ExpiredAccessWithoutRefresh {
		t.Fatal("expired session must be flagged")
	}
}

func TestSyncFollowsOfficialAuth(t *testing.T) {
	e := newEnv(t)
	e.writeOfficial(t, sampleAuth(t, sampleOpt{accountID: "one", email: "one@example.com", refresh: "r1"}))
	must(e.store.ImportOfficialAuth(true, testNow))
	e.writeOfficial(t, sampleAuth(t, sampleOpt{accountID: "two", email: "two@example.com", refresh: "r2"}))

	idx, status, err := e.store.SyncFromOfficialAuth(testNow)
	noErr(t, err)
	active, _ := idx.Account(idx.ActiveAccountID)
	if status != OfficialSynced || len(idx.Accounts) != 2 || active.AccountID != "two" {
		t.Fatalf("status=%v accounts=%d active=%+v", status, len(idx.Accounts), active)
	}
}

func TestSyncNeverRestoresMissingOrBrokenOfficial(t *testing.T) {
	junk := `{"auth_mode":"chatgpt","tokens":{"access_token":"x","refresh_token":"y"}}`
	tests := []struct {
		name   string
		write  string // "" leaves the official file missing
		status OfficialStatus
	}{
		{"missing", "", OfficialMissing},
		{"corrupt", "not json", OfficialMissing},
		{"unusable tokens", junk, OfficialUnusable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			auth := sampleAuth(t, sampleOpt{accountID: "one", email: "one@example.com", refresh: "refresh-sync-repair-one"})
			acc := upsert(t, e.store, auth, true)
			managedBefore := must(os.ReadFile(e.store.CredentialPath(acc.ID)))
			if tc.write != "" {
				noErr(t, os.MkdirAll(e.codexHome, 0o700))
				noErr(t, os.WriteFile(e.officialPath(), []byte(tc.write), 0o600))
			}
			idx, status, err := e.store.SyncFromOfficialAuth(testNow)
			if tc.name == "corrupt" {
				if !errors.Is(err, ErrInvalidCredential) {
					t.Fatalf("corrupt auth error = %v", err)
				}
			} else {
				noErr(t, err)
			}
			if status != tc.status || idx.ActiveAccountID != "" {
				t.Fatalf("status=%v active=%s", status, idx.ActiveAccountID)
			}
			data, readErr := os.ReadFile(e.officialPath())
			if tc.write == "" {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatalf("sync restored missing auth: %v", readErr)
				}
			} else if readErr != nil || string(data) != tc.write {
				t.Fatal("sync modified official auth")
			}
			if saved := must(e.store.LoadIndex()); saved.ActiveAccountID != "" || len(saved.Accounts) != 1 {
				t.Fatal("sync lost managed account or retained current login")
			}
			if row, _ := idx.Account(acc.ID); row.RequiresReauth {
				t.Fatal("invalid official auth incorrectly flagged a usable managed credential")
			}
			if got := must(os.ReadFile(e.store.CredentialPath(acc.ID))); !bytes.Equal(got, managedBefore) {
				t.Fatal("sync changed managed credential")
			}
		})
	}
}

func TestSyncWithNoUsableManagedAccountFlagsReauth(t *testing.T) {
	e := newEnv(t)
	acc := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "one", email: "one@example.com", refresh: "r1"}), true)
	// Corrupt the managed copy as well.
	broken := Auth{AuthMode: "chatgpt", Tokens: Tokens{AccessToken: "short", RefreshToken: "short"}}
	noErr(t, e.store.SaveCredential(acc.ID, broken))
	junk := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"x","refresh_token":"y"}}`)
	noErr(t, os.MkdirAll(e.codexHome, 0o700))
	noErr(t, os.WriteFile(e.officialPath(), junk, 0o600))

	idx, status, err := e.store.SyncFromOfficialAuth(testNow)
	noErr(t, err)
	row, _ := idx.Account(acc.ID)
	if status != OfficialUnusable || !row.RequiresReauth || !bytes.Equal(e.officialBytes(t), junk) {
		t.Fatalf("status=%v row=%+v", status, row)
	}
	// The persisted index carries the flag too.
	if saved, _ := must(e.store.LoadIndex()).Account(acc.ID); !saved.RequiresReauth {
		t.Fatal("flag not persisted")
	}
}

func TestMarkActiveInvalid(t *testing.T) {
	e := newEnv(t)
	acc := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "one", email: "one@example.com", refresh: "refresh-mark-invalid"}), true)
	idx := must(e.store.LoadIndex())
	noErr(t, e.store.markActiveInvalid(&idx))
	row, _ := must(e.store.LoadIndex()).Account(acc.ID)
	if !row.RequiresReauth || row.LastError != "invalid credential" {
		t.Fatalf("row: %+v", row)
	}
	empty := NewAccountIndex()
	noErr(t, e.store.markActiveInvalid(&empty)) // no active account: no-op, no panic
}

func TestSyncNeverReplacesUsableWithUnusable(t *testing.T) {
	e := newEnv(t)
	good := sampleAuth(t, sampleOpt{accountID: "acct", email: "a@example.com", refresh: "refresh-guard-credential-1"})
	acc := upsert(t, e.store, good, true)
	before := must(e.store.LoadCredential(acc.ID))
	noErr(t, os.MkdirAll(e.codexHome, 0o700))
	noErr(t, os.WriteFile(e.officialPath(), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"x","refresh_token":"y"}}`), 0o600))
	_, _, err := e.store.SyncFromOfficialAuth(testNow)
	noErr(t, err)
	if !must(e.store.LoadCredential(acc.ID)).Equal(before) {
		t.Fatal("managed credential was overwritten by an unusable official snapshot")
	}
}

func TestSyncDifferentUserInSameWorkspacePreservesBoth(t *testing.T) {
	e := newEnv(t)
	firstAuth := sampleAuth(t, sampleOpt{accountID: "workspace-shared", email: "one@example.com", refresh: "refresh-first-member", userID: "user-one"})
	secondAuth := sampleAuth(t, sampleOpt{accountID: "workspace-shared", email: "two@example.com", refresh: "refresh-second-member", userID: "user-two"})
	first := upsert(t, e.store, firstAuth, true)
	e.writeOfficial(t, secondAuth)

	idx, _, err := e.store.SyncFromOfficialAuth(testNow)
	noErr(t, err)
	if len(idx.Accounts) != 2 || idx.ActiveAccountID == first.ID {
		t.Fatalf("index: %+v", idx)
	}
	if must(e.store.LoadCredential(first.ID)).Tokens.RefreshToken != "refresh-first-member" ||
		must(e.store.LoadCredential(idx.ActiveAccountID)).Tokens.RefreshToken != "refresh-second-member" {
		t.Fatal("credentials crossed")
	}
}

func TestSyncKeepsRefreshableManagedCredential(t *testing.T) {
	e := newEnv(t)
	full := sampleAuth(t, sampleOpt{accountID: "acct", email: "a@example.com", refresh: "refresh-full-credential-1"})
	acc := upsert(t, e.store, full, true)
	sessionOnly := full
	sessionOnly.Tokens.RefreshToken = ""
	e.writeOfficial(t, sessionOnly)

	_, status, err := e.store.SyncFromOfficialAuth(testNow)
	noErr(t, err)
	if status != OfficialSynced {
		t.Fatalf("status = %v", status)
	}
	if must(e.store.LoadCredential(acc.ID)).Tokens.RefreshToken != full.Tokens.RefreshToken {
		t.Fatal("a weaker official snapshot replaced the refreshable managed credential")
	}
}

func TestRepairOfficialAuth(t *testing.T) {
	e := newEnv(t)
	auth := sampleAuth(t, sampleOpt{accountID: "acct", email: "a@example.com", refresh: "refresh-repair-credential"})
	acc := upsert(t, e.store, auth, true)
	if _, err := os.Stat(e.officialPath()); !os.IsNotExist(err) {
		t.Fatal("upsert must not create the official file")
	}
	idx, err := e.store.RepairOfficialAuth(testNow)
	noErr(t, err)
	if idx.ActiveAccountID != acc.ID {
		t.Fatalf("active = %s", idx.ActiveAccountID)
	}
	got := must(e.store.LoadOfficialAuth())
	if got.Tokens.RefreshToken != auth.Tokens.RefreshToken {
		t.Fatal("official file not repaired")
	}
}

func TestWriteRefreshedCredentialBoundaries(t *testing.T) {
	setup := func(t *testing.T) (testEnv, ManagedAccount, ManagedAccount, Auth, Auth) {
		e := newEnv(t)
		curAuth := sampleAuth(t, sampleOpt{accountID: "current", email: "c@example.com", refresh: "refresh-current-credential"})
		othAuth := sampleAuth(t, sampleOpt{accountID: "other", email: "o@example.com", refresh: "refresh-other-credential-"})
		cur := upsert(t, e.store, curAuth, true)
		oth := upsert(t, e.store, othAuth, false)
		e.writeOfficial(t, curAuth)
		return e, cur, oth, curAuth, othAuth
	}
	rotate := func(a Auth) Auth { a.Tokens.RefreshToken = "rotated-refresh-token-value-0001"; return a }

	t.Run("non-current writes only the managed copy", func(t *testing.T) {
		e, _, oth, _, othAuth := setup(t)
		before := e.officialBytes(t)
		res := must(e.store.WriteRefreshedCredential(oth.ID, rotate(othAuth), testNow))
		if !res.Managed || res.Official || !bytes.Equal(before, e.officialBytes(t)) {
			t.Fatalf("result %+v; official changed", res)
		}
		if must(e.store.LoadCredential(oth.ID)).Tokens.RefreshToken != "rotated-refresh-token-value-0001" {
			t.Fatal("managed copy not updated")
		}
	})
	t.Run("current writes both", func(t *testing.T) {
		e, cur, _, curAuth, _ := setup(t)
		res := must(e.store.WriteRefreshedCredential(cur.ID, rotate(curAuth), testNow))
		if !res.Managed || !res.Official {
			t.Fatalf("result %+v", res)
		}
		if must(e.store.LoadOfficialAuth()).Tokens.RefreshToken != "rotated-refresh-token-value-0001" ||
			must(e.store.LoadCredential(cur.ID)).Tokens.RefreshToken != "rotated-refresh-token-value-0001" {
			t.Fatal("copies out of sync")
		}
	})
	t.Run("current but official drifted to another login", func(t *testing.T) {
		e, cur, _, curAuth, othAuth := setup(t)
		e.writeOfficial(t, othAuth)
		before := e.officialBytes(t)
		res := must(e.store.WriteRefreshedCredential(cur.ID, rotate(curAuth), testNow))
		if !res.Managed || res.Official || !res.OfficialSkipped || !bytes.Equal(before, e.officialBytes(t)) {
			t.Fatalf("result %+v", res)
		}
	})
	t.Run("unknown account", func(t *testing.T) {
		e, _, _, curAuth, _ := setup(t)
		_, err := e.store.WriteRefreshedCredential("nope", curAuth, testNow)
		var nf *AccountNotFoundError
		if !errors.As(err, &nf) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestOfficialWriteRefusesUnusableAuth(t *testing.T) {
	e := newEnv(t)
	bad := Auth{AuthMode: "chatgpt", Tokens: Tokens{AccessToken: "short", RefreshToken: "short"}}
	if err := e.store.writeOfficial(bad, testNow); !errors.Is(err, ErrNotUsableAsCodexLogin) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(e.officialPath()); !os.IsNotExist(err) {
		t.Fatal("unusable auth reached disk")
	}
}

func TestOfficialWritePreservesUnknownFields(t *testing.T) {
	e := newEnv(t)
	noErr(t, os.MkdirAll(e.codexHome, 0o700))
	existing := `{"auth_mode":"chatgpt","some_future_field":{"x":[1,2]},"tokens":{"access_token":"old","refresh_token":"old","account_id":"acct","extra_token_field":"keep"}}`
	noErr(t, os.WriteFile(e.officialPath(), []byte(existing), 0o600))
	auth := sampleAuth(t, sampleOpt{accountID: "acct", email: "a@example.com", refresh: "refresh-official-write"})
	noErr(t, e.store.writeOfficial(auth, testNow))
	out := string(e.officialBytes(t))
	if !strings.Contains(out, "some_future_field") || !strings.Contains(out, "extra_token_field") {
		t.Fatalf("unknown official fields dropped:\n%s", out)
	}

	// Another login's token-level extras must not leak into this one.
	other := sampleAuth(t, sampleOpt{accountID: "different", email: "d@example.com", refresh: "refresh-official-other"})
	noErr(t, e.store.writeOfficial(other, testNow))
	out = string(e.officialBytes(t))
	if strings.Contains(out, "extra_token_field") || !strings.Contains(out, "some_future_field") {
		t.Fatalf("token extras leaked across logins (or top-level extras lost):\n%s", out)
	}
}

func TestInstallCredentialAsOfficial(t *testing.T) {
	e := newEnv(t)
	auth := sampleAuth(t, sampleOpt{accountID: "acct", email: "a@example.com", refresh: "refresh-install-credential"})
	acc := upsert(t, e.store, auth, false)
	got := must(e.store.InstallCredentialAsOfficial(acc.ID, testNow))
	if got.Tokens.RefreshToken != auth.Tokens.RefreshToken || must(e.store.LoadOfficialAuth()).Tokens.AccountID != "acct" {
		t.Fatal("credential not installed")
	}
	_, err := e.store.InstallCredentialAsOfficial("missing", testNow)
	var nf *AccountNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v", err)
	}
}

func TestPathTraversalIdsStayInsideRoot(t *testing.T) {
	e := newEnv(t)
	for _, id := range []string{"../evil", "a/b", "..", ".", "a\\b", ""} {
		p := e.store.CredentialPath(id)
		rel, err := filepath.Rel(e.store.Root(), p)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if err != nil || len(parts) != 2 || parts[0] == ".." || parts[0] == "." || parts[1] != "auth.json" {
			t.Errorf("id %q escapes: %s", id, p)
		}
	}
}

func TestReorderAndRemoveActive(t *testing.T) {
	e := newEnv(t)
	a := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "a", email: "a@example.com", refresh: "refresh-reorder-a"}), true)
	b := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "b", email: "b@example.com", refresh: "refresh-reorder-b"}), false)
	c := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "c", email: "c@example.com", refresh: "refresh-reorder-c"}), false)
	noErr(t, e.store.Reorder([]string{c.ID, b.ID, a.ID}))
	idx := must(e.store.LoadIndex())
	for want, id := range []string{c.ID, b.ID, a.ID} {
		if row, _ := idx.Account(id); row.SortIndex != want {
			t.Errorf("%s sortIndex = %d, want %d", id, row.SortIndex, want)
		}
	}
	// Deleting the active account promotes the lowest sort index.
	noErr(t, e.store.DeleteAccount(a.ID))
	if got := must(e.store.LoadIndex()).ActiveAccountID; got != c.ID {
		t.Errorf("active after delete = %s, want %s", got, c.ID)
	}
	noErr(t, e.store.DeleteAccount(b.ID))
	noErr(t, e.store.DeleteAccount(c.ID))
	if got := must(e.store.LoadIndex()); got.ActiveAccountID != "" || len(got.Accounts) != 0 {
		t.Errorf("empty library: %+v", got)
	}
}

func TestCachedQuotaMeterRowsAndClamping(t *testing.T) {
	reset := testNow
	w300, w10080 := 300, 10080
	sec := QuotaWindow{UsedPercent: 250}
	acc := ManagedAccount{ID: "q"}.ApplyQuota(QuotaSummary{
		Plan: "plus", UpdatedAt: testNow,
		Primary:    QuotaWindow{UsedPercent: -5, WindowMinutes: &w300, ResetsAt: &reset},
		Secondary:  &sec,
		Additional: []NamedQuotaWindow{{Name: "Spark", Window: QuotaWindow{UsedPercent: 11, WindowMinutes: &w10080}}},
	})
	rows := acc.CachedQuota.MeterRows()
	if len(rows) != 3 || rows[0].ID != "primary" || rows[1].ID != "secondary" || rows[2].ID != "additional-0-Spark" {
		t.Fatalf("rows: %+v", rows)
	}
	if rows[0].UsedPercent != 0 || rows[1].UsedPercent != 100 || *rows[1].WindowMinutes != 10080 {
		t.Fatalf("clamping or default window wrong: %+v", rows)
	}
	if rows[1].RemainingPercent() != 0 || acc.CachedQuota.PrimaryRemainingPercent() != 100 {
		t.Fatal("remaining percent")
	}
}

func TestCachedQuotaKeepsCreditsBalance(t *testing.T) {
	balance := 12.5
	acc := ManagedAccount{ID: "q"}.ApplyQuota(QuotaSummary{UpdatedAt: testNow, CreditsBalance: &balance})
	data, err := json.Marshal(acc.CachedQuota)
	if err != nil {
		t.Fatal(err)
	}
	var back CachedAccountQuota
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.CreditsBalance == nil || *back.CreditsBalance != balance {
		t.Fatalf("balance lost: %s", data)
	}
	// Caches written before the balance was kept read as unknown.
	if err := json.Unmarshal([]byte(`{"primaryUsedPercent":10,"updatedAt":"2026-01-02T03:04:05Z"}`), &back); err != nil || back.CreditsBalance != nil {
		t.Fatalf("old cache: %+v %v", back, err)
	}
	if err := json.Unmarshal([]byte(`{"primaryUsedPercent":10,"creditsBalance":"12"}`), &back); err == nil {
		t.Fatal("non-number balance accepted")
	}
}

func TestErrorMessagesCarryNoSecrets(t *testing.T) {
	errs := []error{
		&AccountNotFoundError{ID: "x"}, &CredentialMissingError{ID: "x"},
		&UnsupportedFormatError{Detail: "d"}, &UnsupportedVersionError{Version: 2},
		&ProviderMismatchError{Expected: ProviderCodex, Actual: ProviderGrok},
	}
	for _, err := range errs {
		if !strings.HasPrefix(err.Error(), "codex: ") {
			t.Errorf("%T message %q", err, err.Error())
		}
	}
}
