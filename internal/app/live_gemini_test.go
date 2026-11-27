package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// noGeminiKeychain stands in for the shared Antigravity keychain item in
// every app test, so no test reads or writes the real login.
type noGeminiKeychain struct{}

func (noGeminiKeychain) Load(context.Context) ([]byte, error) {
	return nil, gemini.ErrOfficialCredentialMissing
}
func (noGeminiKeychain) Save(context.Context, []byte) error { return gemini.ErrKeychainUnavailable }
func (noGeminiKeychain) Remove(context.Context) error       { return nil }

func init() {
	geminiKeychain = func() gemini.Keychain { return noGeminiKeychain{} }
}

func geminiTestModule(t *testing.T) *gemini.Module {
	t.Helper()
	home := t.TempDir()
	mod := newGeminiModule(home, "test")
	if got := mod.Store.Root(); !strings.HasPrefix(got, home) {
		t.Fatalf("store root %q is outside the temporary home", got)
	}
	if _, ok := mod.Store.Official.Keychain.(noGeminiKeychain); !ok {
		t.Fatalf("the module must use the test keychain, got %T", mod.Store.Official.Keychain)
	}
	return mod
}

func saveGeminiAccounts(t *testing.T, mod *gemini.Module, current string, accounts ...gemini.ManagedAccount) {
	t.Helper()
	if err := mod.Store.SaveIndex(gemini.AccountIndex{Version: 1, CurrentAccountID: current, Accounts: accounts}); err != nil {
		t.Fatal(err)
	}
}

func geminiSnapshotJSON(t *testing.T, snap gemini.QuotaSnapshot) json.RawMessage {
	t.Helper()
	data, err := gemini.MarshalQuotaSnapshot(snap)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGeminiStateWithoutAccounts(t *testing.T) {
	l := l10n.New("en")
	mod := geminiTestModule(t)
	st := buildGeminiState(mod, l, func() bool { return false }, true)
	if st.Availability != runway.GeminiNotInstalled || st.LastError != l.Text(l10n.KeyGeminiNotInstalled) {
		t.Fatalf("not installed: %+v", st)
	}
	st = buildGeminiState(mod, l, func() bool { return true }, true)
	if st.Availability != runway.GeminiNotLoggedIn {
		t.Fatalf("installed without login: %v", st.Availability)
	}
	if w := geminiWidget(st); w.Provider != export.ProviderGemini || w.Availability != export.NotLoggedIn {
		t.Fatalf("widget: %+v", w)
	}
}

func TestGeminiStateOrdersQuotaWindowsAndKeepsCredits(t *testing.T) {
	l := l10n.New("en")
	mod := geminiTestModule(t)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	reset := now.Add(3 * time.Hour)
	half := 0.5
	snap := gemini.QuotaSnapshot{
		Tier: gemini.Tier{Kind: gemini.TierPro},
		Buckets: []gemini.QuotaBucket{
			{ID: "x", Family: gemini.FamilyOther, DisplayName: "Other", RemainingFraction: 0.9},
			{ID: "cw", Family: gemini.FamilyClaude, Window: gemini.WindowWeekly, RemainingFraction: 0.8},
			{ID: "gw", Family: gemini.FamilyGemini, Window: gemini.WindowWeekly, RemainingFraction: 0.6},
			{ID: "gemini-argon-wtu-weekly", Family: gemini.FamilyArgon, Group: "Gemini 4 Argon", Window: gemini.WindowWeekly, RemainingFraction: 0.48, ResetAt: &reset},
			{ID: "g5", Family: gemini.FamilyGemini, Window: gemini.WindowFiveHour, RemainingFraction: 0.25, ResetAt: &reset},
		},
		Models:    []gemini.ModelQuota{{ID: "m", DisplayName: "Gemini 3 Pro", RemainingFraction: &half}},
		Credits:   []gemini.Credit{{Type: "GOOGLE_ONE_AI", Amount: 25}},
		UpdatedAt: now,
	}
	saveGeminiAccounts(t, mod, "a", gemini.ManagedAccount{
		ID: "a", Identity: gemini.Identity{Subject: "1", Email: "a@example.com"}, LastQuotaAt: &now,
		CachedQuota: geminiSnapshotJSON(t, snap),
	})
	st := buildGeminiState(mod, l, nil, true)
	if st.Availability != runway.GeminiReady || st.TierKind != string(gemini.TierPro) || st.Plan != "Pro" {
		t.Fatalf("state: %v %q %q", st.Availability, st.TierKind, st.Plan)
	}
	var titles []string
	for _, m := range st.Meters {
		titles = append(titles, m.Title)
	}
	want := []string{"Gemini · 5-hour", "Gemini · Weekly", "Claude · Weekly", "Gemini 4 Argon · Weekly", "Other"}
	if strings.Join(titles, "|") != strings.Join(want, "|") {
		t.Fatalf("meters = %q, want %q", titles, want)
	}
	if st.Meters[0].Source != runway.SourceStandard || st.Meters[2].Source != runway.SourceModelSpecific || st.Meters[0].UsedPercent != 75 {
		t.Fatalf("first meters: %+v %+v", st.Meters[0], st.Meters[2])
	}
	if argon := st.Meters[3]; argon.UsedPercent != 52 || argon.Source != runway.SourceModelSpecific || !argon.ResetsAt.Equal(reset) {
		t.Fatalf("Argon meter = %+v", argon)
	}
	if len(st.AccountQuotas["a"].Meters) != len(st.Meters) || len(geminiWidget(st).Quota) != len(st.Meters) {
		t.Fatal("account and widget must retain the independent Argon quota")
	}
	q := st.AccountQuotas["a"]
	if len(q.Details) != 1 || q.Details[0].Value != "25" || q.Title != "Gemini · 5-hour" {
		t.Fatalf("account quota: %+v", q)
	}
	if st.Accounts.ActiveID != "a" || len(st.Accounts.Sidebar) != 1 || st.Accounts.Sidebar[0].Email != "a@example.com" {
		t.Fatalf("accounts: %+v", st.Accounts)
	}
}

func TestGeminiStateFallsBackToModelFractions(t *testing.T) {
	l := l10n.New("en")
	mod := geminiTestModule(t)
	low, high := 0.1, 0.7
	snap := gemini.QuotaSnapshot{Models: []gemini.ModelQuota{
		{ID: "flash", DisplayName: "Flash", RemainingFraction: &high},
		{ID: "pro", DisplayName: "Pro", Recommended: true, RemainingFraction: &low},
		{ID: "none", DisplayName: "No quota"},
	}}
	saveGeminiAccounts(t, mod, "a", gemini.ManagedAccount{ID: "a", Identity: gemini.Identity{Subject: "1"}, CachedQuota: geminiSnapshotJSON(t, snap)})
	st := buildGeminiState(mod, l, nil, true)
	if len(st.Meters) != 2 || st.Meters[0].Title != "Pro" || st.Meters[0].Source != runway.SourceStandard || st.Meters[1].Source != runway.SourceModelSpecific {
		t.Fatalf("meters: %+v", st.Meters)
	}
}

func TestGeminiAvailabilityFollowsStoredErrors(t *testing.T) {
	snap := &gemini.QuotaSnapshot{}
	for _, tc := range []struct {
		acc  gemini.ManagedAccount
		snap *gemini.QuotaSnapshot
		want runway.GeminiAvailability
	}{
		{gemini.ManagedAccount{}, nil, runway.GeminiLoading},
		{gemini.ManagedAccount{}, snap, runway.GeminiReady},
		{gemini.ManagedAccount{}, &gemini.QuotaSnapshot{Validation: &gemini.Validation{Reason: "verify"}}, runway.GeminiValidationRequired},
		{gemini.ManagedAccount{RequiresReauth: true}, snap, runway.GeminiReauthRequired},
		{gemini.ManagedAccount{LastError: gemini.ErrorCodeAuthenticationNeeded}, nil, runway.GeminiReauthRequired},
		{gemini.ManagedAccount{LastError: gemini.ErrorCodeValidationRequired}, snap, runway.GeminiValidationRequired},
		{gemini.ManagedAccount{LastError: gemini.ErrorCodeSubscriptionRequired}, snap, runway.GeminiSubscriptionRequired},
		{gemini.ManagedAccount{LastError: gemini.ErrorCodeRateLimited}, snap, runway.GeminiRateLimited},
		{gemini.ManagedAccount{LastError: gemini.ErrorCodeQuotaParseFailed}, nil, runway.GeminiQuotaParseFailed},
		{gemini.ManagedAccount{LastError: gemini.ErrorCodeQuotaParseFailed}, snap, runway.GeminiReady},
		{gemini.ManagedAccount{LastError: gemini.ErrorCodeTimeout}, snap, runway.GeminiReady},
		{gemini.ManagedAccount{LastError: gemini.ErrorCodeTimeout}, nil, runway.GeminiFailed},
	} {
		if got := geminiAvailability(tc.acc, tc.snap); got != tc.want {
			t.Errorf("%+v / %v: got %v, want %v", tc.acc, tc.snap != nil, got, tc.want)
		}
	}
}

func TestGeminiValidationKeepsItsPage(t *testing.T) {
	l := l10n.New("en")
	mod := geminiTestModule(t)
	snap := gemini.QuotaSnapshot{Validation: &gemini.Validation{Reason: "verify", URL: "https://accounts.example.test/verify"}}
	saveGeminiAccounts(t, mod, "a", gemini.ManagedAccount{ID: "a", Identity: gemini.Identity{Subject: "1"}, LastError: gemini.ErrorCodeValidationRequired, CachedQuota: geminiSnapshotJSON(t, snap)})
	st := buildGeminiState(mod, l, nil, true)
	if st.Availability != runway.GeminiValidationRequired || st.ValidationURL != snap.Validation.URL {
		t.Fatalf("state: %v %q", st.Availability, st.ValidationURL)
	}
	if geminiWidgetAvailability(st.Availability) != export.NotLoggedIn {
		t.Fatal("a pending verification is not a reading")
	}
}

func TestGeminiRefreshUsesTheTemporaryHomeOnly(t *testing.T) {
	mod := geminiTestModule(t)
	b := newGeminiBridge(mod, filepath.Dir(mod.Store.Root()), store.DefaultPreferences(), nil)
	changed := 0
	b.SetOnChange(func() { changed++ })
	if err := b.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if st := b.State(time.Now()); st.Availability != runway.GeminiNotLoggedIn && st.Availability != runway.GeminiNotInstalled {
		t.Fatalf("availability: %v", st.Availability)
	}
	if changed == 0 {
		t.Fatal("refresh must notify")
	}
	if err := b.RefreshLocal(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// With reads off, a cached reading from before is not shown anywhere, but
// a sign-in problem still is.
func TestGeminiStateHidesQuotaWhileReadsAreOff(t *testing.T) {
	l := l10n.New("en")
	mod := geminiTestModule(t)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	snap := gemini.QuotaSnapshot{Tier: gemini.Tier{Kind: gemini.TierPro}, UpdatedAt: now,
		Buckets: []gemini.QuotaBucket{{ID: "g5", Family: gemini.FamilyGemini, Window: gemini.WindowFiveHour, RemainingFraction: 0.5}}}
	acc := gemini.ManagedAccount{ID: "a", Identity: gemini.Identity{Subject: "1"}, LastQuotaAt: &now, CachedQuota: geminiSnapshotJSON(t, snap)}
	saveGeminiAccounts(t, mod, "a", acc)
	st := buildGeminiState(mod, l, nil, false)
	if st.Availability != runway.GeminiQuotaOff || len(st.Meters) != 0 || st.AccountQuotas != nil || st.Plan != "" {
		t.Fatalf("off: %+v", st)
	}
	if st.LastError != l.Text(l10n.KeyGeminiQuotaOff) || geminiWidget(st).Availability != export.Unavailable {
		t.Fatalf("off text/widget: %q", st.LastError)
	}
	if st.Accounts.ActiveID != "a" || len(st.Accounts.Sidebar) != 1 {
		t.Fatal("accounts stay visible while reads are off")
	}
	if on := buildGeminiState(mod, l, nil, true); on.Availability != runway.GeminiReady || len(on.Meters) != 1 {
		t.Fatalf("on: %+v", on)
	}
	acc.RequiresReauth = true
	saveGeminiAccounts(t, mod, "a", acc)
	if st := buildGeminiState(mod, l, nil, false); st.Availability != runway.GeminiReauthRequired {
		t.Fatalf("reauth while off: %v", st.Availability)
	}
}

// An automatic refresh never reads quota, and with reads off a manual one
// does not either;
// a refusal shows as the notice until the next automatic refresh.
func TestGeminiBridgeReadsQuotaOnlyOnManualRefresh(t *testing.T) {
	mod := geminiTestModule(t)
	mod.Quota = func(context.Context, string) (gemini.QuotaSnapshot, error) {
		t.Error("quota was read")
		return gemini.QuotaSnapshot{}, errors.New("unexpected")
	}
	mod.RunningProcessIDs = func(context.Context) ([]int, error) { return []int{1}, nil }
	if b := newGeminiBridge(mod, filepath.Dir(mod.Store.Root()), store.DefaultPreferences(), nil); mod.QuotaEnabled == nil || !mod.QuotaEnabled() || b == nil {
		t.Fatal("the bridge must wire the switch, on by default")
	}
	off := store.DefaultPreferences()
	off.GeminiQuotaEnabled = false
	b := newGeminiBridge(mod, filepath.Dir(mod.Store.Root()), off, nil)
	if mod.QuotaEnabled() {
		t.Fatal("turning reads off must reach the module")
	}
	if err := b.Refresh(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := b.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	prefs := store.DefaultPreferences()
	prefs.GeminiQuotaEnabled = true
	b.setPreferences(prefs, nil)
	if !mod.QuotaEnabled() {
		t.Fatal("turning reads on must reach the module")
	}
	b.mu.Lock()
	b.refusal = &gemini.QuotaCooldownError{RetryAfter: 90 * time.Second}
	b.mu.Unlock()
	b.reload(false)
	l := l10n.New("en")
	if got := b.State(time.Now()).Notice; got != l.Textf(l10n.KeyGeminiQuotaCooldown, 2) {
		t.Fatalf("notice = %q", got)
	}
	if err := b.Refresh(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if got := b.State(time.Now()).Notice; got != "" {
		t.Fatalf("an automatic refresh must clear the notice: %q", got)
	}
}

func TestGeminiRefusalsAreExplained(t *testing.T) {
	l := l10n.New("en")
	for _, tc := range []struct {
		err  error
		want string
	}{
		{gemini.ErrQuotaDisabled, l.Text(l10n.KeyGeminiQuotaOff)},
		{gemini.ErrQuotaCurrentOnly, l.Text(l10n.KeyGeminiQuotaCurrentOnly)},
		{gemini.ErrClientNotRunning, l.Text(l10n.KeyGeminiQuotaClientNotRunning)},
		{&gemini.QuotaCooldownError{RetryAfter: 14*time.Minute + time.Second}, l.Textf(l10n.KeyGeminiQuotaCooldown, 15)},
		{&gemini.QuotaCooldownError{RetryAfter: time.Second}, l.Textf(l10n.KeyGeminiQuotaCooldown, 1)},
	} {
		if !isGeminiRefusal(tc.err) || geminiRefusalText(l, tc.err) != tc.want || geminiKnownReason(l, tc.err) != tc.want {
			t.Errorf("%v: %q", tc.err, geminiRefusalText(l, tc.err))
		}
	}
	if isGeminiRefusal(&gemini.RateLimitedError{}) || geminiRefusal(gemini.RefreshReport{Outcomes: []gemini.RefreshOutcome{{Err: gemini.ErrQuotaParse}}}) != nil {
		t.Fatal("a sent read is not a refusal")
	}
}

func TestGeminiRefreshMessageCountsFailures(t *testing.T) {
	l := l10n.New("en")
	report := gemini.RefreshReport{Outcomes: []gemini.RefreshOutcome{{AccountID: "a"}, {AccountID: "b", Err: &gemini.RateLimitedError{}}}}
	if msg := geminiRefreshMessage(l, report); msg != accountBatchFailure(l, 1, 1, []error{report.Outcomes[1].Err}) || msg == "" {
		t.Fatalf("message: %q", msg)
	}
	if msg := geminiRefreshMessage(l, gemini.RefreshReport{Outcomes: []gemini.RefreshOutcome{{AccountID: "a"}}}); msg != "" {
		t.Fatalf("success message: %q", msg)
	}
}

func TestGeminiRestartOnlyQuitsAntigravity(t *testing.T) {
	codexQuit, geminiQuit := false, false
	apps := []runningApp{
		{bundleID: "com.openai.codex", path: "/fake/Codex.app", terminate: func() bool { codexQuit = true; return true }, stopped: func() bool { return true }},
		{bundleID: "com.google.Antigravity", path: "/fake/Antigravity.app", terminate: func() bool { geminiQuit = true; return true }, stopped: func() bool { return true }},
	}
	var opened []string
	reopen, err := prepareAppRestart(geminiAppRestart, apps, "", nil, func(path string) error { opened = append(opened, path); return nil })
	if err != nil || reopen == nil || codexQuit || !geminiQuit {
		t.Fatalf("err=%v reopen=%v codexQuit=%v geminiQuit=%v", err, reopen != nil, codexQuit, geminiQuit)
	}
	if err := reopen(); err != nil || len(opened) != 1 || opened[0] != "/fake/Antigravity.app" {
		t.Fatalf("reopen err=%v opened=%q", err, opened)
	}
	stuck := []runningApp{{bundleID: "com.google.antigravity", path: "/fake/Antigravity.app", terminate: func() bool { return false }}}
	if _, err := prepareAppRestart(geminiAppRestart, stuck, "", nil, func(string) error { return nil }); !errors.Is(err, errGeminiQuit) || !errors.Is(err, errRestartGemini) {
		t.Fatalf("stuck quit err=%v", err)
	}
}
