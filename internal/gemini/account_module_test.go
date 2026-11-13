package gemini

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type quotaFake struct {
	mu     sync.Mutex
	tokens []string
	errs   []error
}

func (q *quotaFake) fetch(_ context.Context, token string) (QuotaSnapshot, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tokens = append(q.tokens, token)
	if len(q.errs) > 0 {
		err := q.errs[0]
		q.errs = q.errs[1:]
		if err != nil {
			return QuotaSnapshot{}, err
		}
	}
	return QuotaSnapshot{Tier: Tier{Kind: TierPro, ID: "g1-pro-tier"}, Source: SourceSummary,
		Buckets: []QuotaBucket{{ID: "gemini:5h", Family: FamilyGemini, Window: WindowFiveHour, RemainingFraction: 0.5}}, UpdatedAt: testNow}, nil
}

type moduleEnv struct {
	testEnv
	module *Module
	oauth  *oauthServer
	quota  *quotaFake
}

func newModuleEnv(t *testing.T) moduleEnv {
	t.Helper()
	env := newTestEnv(t)
	o := newOAuthServer(t)
	q := &quotaFake{}
	m := &Module{
		Store: env.store, OAuth: o.client(), Quota: q.fetch,
		RunningProcessIDs: func(context.Context) ([]int, error) { return nil, nil },
		Now:               func() time.Time { return testNow },
	}
	return moduleEnv{testEnv: env, module: m, oauth: o, quota: q}
}

func (e moduleEnv) setOfficial(data []byte) { e.keychain.data, e.keychain.has = data, true }

func (e moduleEnv) login(t *testing.T, sub, email, refresh string) ManagedAccount {
	t.Helper()
	data := credJSON(t, "acc-"+sub, refresh, idToken(sub, email), testNow.Add(time.Hour))
	acc, err := e.module.ImportBrowserLogin(context.Background(), LoginResult{Credential: data, Identity: Identity{Subject: sub, Email: email}})
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

func TestLoadAdoptsOfficialLoginAndFollowsExternalChanges(t *testing.T) {
	e := newModuleEnv(t)
	ctx := context.Background()
	e.setOfficial(credJSON(t, "a1", "r1", idToken("s1", "one@example.com"), testNow.Add(time.Hour)))
	state, err := e.module.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Official.Kind != OfficialAuthenticated || len(state.Accounts) != 1 || state.CurrentAccountID != (Identity{Subject: "s1"}).StableID() {
		t.Fatalf("state = %+v", state)
	}
	// Another account signs in in Antigravity, without an id_token: userinfo names it.
	e.oauth.userinfo = `{"id":"s2","email":"two@example.com"}`
	e.setOfficial(credJSON(t, "a2", "r2", "", testNow.Add(time.Hour)))
	state, err = e.module.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Accounts) != 2 || state.CurrentAccountID != (Identity{Subject: "s2"}).StableID() || !state.OfficialIdentityChangedExternally {
		t.Fatalf("external change = %+v", state)
	}
	if state, _ = e.module.Load(ctx); state.OfficialIdentityChangedExternally {
		t.Fatal("an external change is reported once")
	}
	// Logged out: current cleared, accounts kept.
	e.keychain.has = false
	if state, _ = e.module.Load(ctx); state.CurrentAccountID != "" || state.Official.Kind != OfficialMissing || len(state.Accounts) != 2 {
		t.Fatalf("logged out = %+v", state)
	}
}

func TestBrowserLoginBootstrapsEmptyLibraryOnly(t *testing.T) {
	e := newModuleEnv(t)
	first := e.login(t, "s1", "one@example.com", "r1")
	kc, has := e.keychain.get()
	if !has || mustCred(t, kc).RefreshToken != "r1" {
		t.Fatal("first login must become the official one")
	}
	idx, _ := e.store.LoadIndex()
	if idx.CurrentAccountID != first.ID {
		t.Fatal("first login is current")
	}
	e.login(t, "s2", "two@example.com", "r2")
	if kc, _ := e.keychain.get(); mustCred(t, kc).RefreshToken != "r1" {
		t.Fatal("a second login must not touch the official login")
	}
}

func TestSwitchWritesKeychainAndExistingFilesAndKeepsReplacedLogin(t *testing.T) {
	e := newModuleEnv(t)
	ctx := context.Background()
	files := OfficialFiles(e.home)
	officialOld := credJSON(t, "a0", "r0", idToken("s0", "zero@example.com"), testNow.Add(time.Hour))
	e.setOfficial(officialOld)
	writeFile(t, files[1], officialOld)
	target := e.login(t, "s1", "one@example.com", "r1")

	e.module.RunningProcessIDs = func(context.Context) ([]int, error) { return []int{42}, nil }
	if _, err := e.module.Apply(ctx, Command{Kind: CommandMakeCurrent, ID: target.ID}); !isErr[*ProcessesRunningError](err) {
		t.Fatalf("running: %v", err)
	}
	state, err := e.module.Apply(ctx, Command{Kind: CommandMakeCurrent, ID: target.ID, AllowWhileRunning: true})
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentAccountID != target.ID || len(state.Accounts) != 2 {
		t.Fatalf("state = %+v", state)
	}
	if kc, _ := e.keychain.get(); mustCred(t, kc).RefreshToken != "r1" {
		t.Fatal("keychain not switched")
	}
	if got, _ := os.ReadFile(files[1]); mustCred(t, got).RefreshToken != "r1" {
		t.Fatal("existing CLI file not switched")
	}
	if _, err := os.Stat(files[0]); !os.IsNotExist(err) {
		t.Fatal("absent file must not be created")
	}
	old := (Identity{Subject: "s0"}).StableID()
	if data, err := e.store.LoadCredentialData(old); err != nil || mustCred(t, data).RefreshToken != "r0" {
		t.Fatal("replaced login must stay in the library")
	}
}

func TestSwitchFailureRestoresOfficialAndIndex(t *testing.T) {
	e := newModuleEnv(t)
	ctx := context.Background()
	officialOld := credJSON(t, "a0", "r0", idToken("s0", "zero@example.com"), testNow.Add(time.Hour))
	e.setOfficial(officialOld)
	if _, err := e.module.Load(ctx); err != nil {
		t.Fatal(err)
	}
	target := e.login(t, "s1", "one@example.com", "r1")
	before, _ := e.store.LoadIndex()

	e.keychain.corruptSaves = 1
	if _, err := e.module.Apply(ctx, Command{Kind: CommandMakeCurrent, ID: target.ID}); err == nil {
		t.Fatal("read-back mismatch must fail")
	}
	if kc, _ := e.keychain.get(); !sameJSON(kc, officialOld) {
		t.Fatal("keychain not rolled back")
	}
	after, _ := e.store.LoadIndex()
	if after.CurrentAccountID != before.CurrentAccountID {
		t.Fatal("index current changed")
	}

	e.keychain.failSave = 1
	if _, err := e.module.Apply(ctx, Command{Kind: CommandMakeCurrent, ID: target.ID}); !errors.Is(err, ErrKeychainUnavailable) {
		t.Fatalf("save failure: %v", err)
	}
	if kc, _ := e.keychain.get(); !sameJSON(kc, officialOld) {
		t.Fatal("keychain changed after a failed save")
	}
}

func TestRefreshCurrentRetriesAfter401AndSyncsOfficial(t *testing.T) {
	e := newModuleEnv(t)
	ctx := context.Background()
	files := OfficialFiles(e.home)
	official := credJSON(t, "a0", "r0", idToken("s0", "zero@example.com"), testNow.Add(time.Hour))
	e.setOfficial(official)
	writeFile(t, files[0], official)
	e.quota.errs = []error{ErrUnauthorized, nil}
	report := e.module.Refresh(ctx, RefreshTarget{Kind: RefreshCurrent})
	if len(report.Outcomes) != 1 || report.Outcomes[0].Err != nil || report.Outcomes[0].Snapshot == nil {
		t.Fatalf("report = %+v", report.Outcomes)
	}
	if e.oauth.refreshCalls != 1 || e.quota.tokens[0] != "a0" || e.quota.tokens[1] != "fresh-1" {
		t.Fatalf("refresh calls %d tokens %v", e.oauth.refreshCalls, e.quota.tokens)
	}
	kc, _ := e.keychain.get()
	if c := mustCred(t, kc); c.AccessToken != "fresh-1" || c.RefreshToken != "r0" || c.IDToken == "" {
		t.Fatal("refreshed token must be written back to the keychain")
	}
	if got, _ := os.ReadFile(files[0]); mustCred(t, got).AccessToken != "fresh-1" {
		t.Fatal("refreshed token must be written to the existing file")
	}
	idx, _ := e.store.LoadIndex()
	acc, _ := idx.Account(idx.CurrentAccountID)
	if snap, err := ParseQuotaSnapshot(acc.CachedQuota); err != nil || snap.Tier.Kind != TierPro || acc.LastQuotaAt == nil {
		t.Fatalf("cached quota: %v", err)
	}

	e.quota.errs = []error{ErrUnauthorized, ErrUnauthorized}
	report = e.module.Refresh(ctx, RefreshTarget{Kind: RefreshCurrent})
	if err := report.Outcomes[0].Err; !errors.Is(err, ErrAuthenticationRequired) || !report.Outcomes[0].RetainedStaleSnapshot {
		t.Fatalf("second 401: %v", err)
	}
	idx, _ = e.store.LoadIndex()
	if acc, _ := idx.Account(idx.CurrentAccountID); !acc.RequiresReauth || acc.LastError != ErrorCodeAuthenticationNeeded {
		t.Fatalf("account = %+v", acc)
	}
}

func TestRefreshNonCurrentTouchesOnlyManagedCopy(t *testing.T) {
	e := newModuleEnv(t)
	ctx := context.Background()
	official := credJSON(t, "a0", "r0", idToken("s0", "zero@example.com"), testNow.Add(time.Hour))
	e.setOfficial(official)
	if _, err := e.module.Load(ctx); err != nil {
		t.Fatal(err)
	}
	expired := credJSON(t, "old", "r1", idToken("s1", "one@example.com"), testNow.Add(5*time.Minute))
	acc, err := e.store.Upsert(expired, Identity{Subject: "s1", Email: "one@example.com"}, UpsertOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	saves := e.keychain.saves
	report := e.module.Refresh(ctx, RefreshTarget{Kind: RefreshAccount, ID: acc.ID})
	if report.Outcomes[0].Err != nil {
		t.Fatal(report.Outcomes[0].Err)
	}
	if e.keychain.saves != saves {
		t.Fatal("non-current refresh wrote the official login")
	}
	if data, _ := e.store.LoadCredentialData(acc.ID); mustCred(t, data).AccessToken != "fresh-1" {
		t.Fatal("managed copy not refreshed")
	}

	e.oauth.tokenStatus, e.oauth.tokenBody = 400, `{"error":"invalid_grant"}`
	writeExpired := credJSON(t, "old", "r1", idToken("s1", "one@example.com"), testNow.Add(-time.Minute))
	if err := e.store.SaveCredentialData(acc.ID, writeExpired); err != nil {
		t.Fatal(err)
	}
	report = e.module.Refresh(ctx, RefreshTarget{Kind: RefreshAccount, ID: acc.ID})
	if !errors.Is(report.Outcomes[0].Err, ErrAuthenticationRequired) {
		t.Fatalf("invalid_grant: %v", report.Outcomes[0].Err)
	}
	idx, _ := e.store.LoadIndex()
	if a, _ := idx.Account(acc.ID); !a.RequiresReauth || a.LastError != ErrorCodeAuthenticationNeeded {
		t.Fatalf("flags = %+v", a)
	}
}

func TestRefreshRateLimitAndValidation(t *testing.T) {
	e := newModuleEnv(t)
	ctx := context.Background()
	e.setOfficial(credJSON(t, "a0", "r0", idToken("s0", "zero@example.com"), testNow.Add(time.Hour)))
	e.quota.errs = []error{&RateLimitedError{}}
	report := e.module.Refresh(ctx, RefreshTarget{Kind: RefreshCurrent})
	if !isErr[*RateLimitedError](report.Outcomes[0].Err) {
		t.Fatalf("429: %v", report.Outcomes[0].Err)
	}
	calls := len(e.quota.tokens)
	report = e.module.Refresh(ctx, RefreshTarget{Kind: RefreshCurrent})
	var rl *RateLimitedError
	if !errors.As(report.Outcomes[0].Err, &rl) || rl.RetryAfter != RateLimitBackoffStart || len(e.quota.tokens) != calls {
		t.Fatal("backoff must hold the account back without a request")
	}

	e.module.throttle.Succeeded(report.Outcomes[0].AccountID)
	e.quota.errs = []error{nil, &ValidationRequiredError{Reason: reasonValidationRequired, URL: "https://accounts.google.com/v"}}
	_ = e.module.Refresh(ctx, RefreshTarget{Kind: RefreshCurrent})
	report = e.module.Refresh(ctx, RefreshTarget{Kind: RefreshCurrent})
	if !isErr[*ValidationRequiredError](report.Outcomes[0].Err) || !report.Outcomes[0].RetainedStaleSnapshot {
		t.Fatalf("validation: %+v", report.Outcomes[0])
	}
	idx, _ := e.store.LoadIndex()
	acc, _ := idx.Account(idx.CurrentAccountID)
	snap, err := ParseQuotaSnapshot(acc.CachedQuota)
	if err != nil || snap.Validation == nil || snap.Validation.URL != "https://accounts.google.com/v" || len(snap.Buckets) != 1 {
		t.Fatalf("snapshot = %+v %v", snap, err)
	}
	if acc.LastError != ErrorCodeValidationRequired || acc.RequiresReauth {
		t.Fatalf("account = %+v", acc)
	}
}

func TestRemoveAliasReorder(t *testing.T) {
	e := newModuleEnv(t)
	ctx := context.Background()
	first := e.login(t, "s1", "one@example.com", "r1")
	second := e.login(t, "s2", "two@example.com", "r2")
	if _, err := e.module.Apply(ctx, Command{Kind: CommandRemove, ID: first.ID}); !isErr[*CurrentAccountCannotBeRemovedError](err) {
		t.Fatalf("remove current: %v", err)
	}
	state, err := e.module.Apply(ctx, Command{Kind: CommandSetAlias, ID: second.ID, Alias: "  Work  "})
	if err != nil {
		t.Fatal(err)
	}
	state, err = e.module.Apply(ctx, Command{Kind: CommandReorder, IDs: []string{second.ID}})
	if err != nil || state.Accounts[0].ID != second.ID || state.Accounts[0].Alias != "Work" {
		t.Fatalf("reorder: %+v %v", state.Accounts, err)
	}
	state, err = e.module.Apply(ctx, Command{Kind: CommandRemove, ID: second.ID})
	if err != nil || len(state.Accounts) != 1 {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(e.store.AccountDir(second.ID)); !os.IsNotExist(err) {
		t.Fatal("credential directory kept")
	}
}

func TestStoreFilesArePrivate(t *testing.T) {
	e := newModuleEnv(t)
	acc := e.login(t, "s1", "one@example.com", "r1")
	for _, path := range []string{e.store.IndexPath(), e.store.CredentialPath(acc.ID)} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 && filepath.Separator == '/' {
			t.Fatalf("%s mode %v", filepath.Base(path), info.Mode().Perm())
		}
	}
	idx, _ := os.ReadFile(e.store.IndexPath())
	for _, secret := range []string{"acc-s1", "r1\""} {
		if strings.Contains(string(idx), secret) {
			t.Fatal("index must never hold tokens")
		}
	}
}

func TestPasteImportAndPackRoundTrip(t *testing.T) {
	e := newModuleEnv(t)
	ctx := context.Background()
	text := string(credJSON(t, "a1", "r1", idToken("s1", "one@example.com"), testNow.Add(time.Hour)))
	result, state, err := e.module.ImportPastedText(ctx, text)
	if err != nil || result.SuccessCount() != 1 || state.CurrentAccountID == "" {
		t.Fatalf("paste: %+v %v", result, err)
	}
	if _, has := e.keychain.get(); !has {
		t.Fatal("pasting into an empty library bootstraps the official login")
	}
	exported, err := e.module.ExportAccounts([]string{result.Succeeded[0].ID})
	if err != nil || exported.ExportedCount != 1 {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pack.json")
	if _, err := (Exporter{Store: e.store}).Write([]string{result.Succeeded[0].ID}, path, testNow); err != nil {
		t.Fatal(err)
	}

	other := newModuleEnv(t)
	preview, err := other.module.PreviewImportFiles(ctx, []string{path})
	if err != nil || len(preview.Items) != 1 || !preview.SourceIsPack || preview.RoutedProvider != ProviderGemini {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if preview.Items[0].Preview.Detail != "one@example.com" {
		t.Fatalf("detail = %q", preview.Items[0].Preview.Detail)
	}
	batch, _, err := other.module.ImportPreviewSelection(ctx, preview.Items, []string{preview.Items[0].ID()})
	if err != nil || batch.SuccessCount() != 1 {
		t.Fatalf("commit: %+v %v", batch, err)
	}
	if _, has := other.keychain.get(); has {
		t.Fatal("pack import never writes the official login")
	}

	grokPack := filepath.Join(t.TempDir(), "grok.json")
	writeFile(t, grokPack, []byte(`{"format":"quosparr-accounts","version":1,"provider":"grok","accounts":[]}`))
	if p, _ := other.module.PreviewImportFiles(ctx, []string{grokPack}); p.RoutedProvider != "grok" || len(p.Items) != 0 {
		t.Fatalf("routing: %+v", p)
	}
}
