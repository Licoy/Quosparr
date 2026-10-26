package runway

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// ---- accounts ---------------------------------------------------------------

// accountFixture is two managed accounts in a temp store; account A is the
// active one and the fake's login.
type accountFixture struct {
	st    *codex.Store
	a, b  codex.ManagedAccount
	gens  *codexapi.Generations
	mu    sync.Mutex
	polls func(ids []string) []codexapi.QuotaRefreshResult
	// recordedAtPoll is what RecordAuth had seen when polling started.
	recordedAtPoll []string
}

func newAccountFixture(t *testing.T, f *fake) *accountFixture {
	t.Helper()
	af := &accountFixture{st: tempStore(t), gens: &codexapi.Generations{}}
	authB := testAuth(t, "acct-b", "b@example.com", "refresh-b")
	var err error
	if af.b, err = af.st.Upsert(authB, codex.UpsertOptions{Now: t0}); err != nil {
		t.Fatal(err)
	}
	if af.a, err = af.st.Upsert(f.auth, codex.UpsertOptions{MakeActive: true, Now: t0}); err != nil {
		t.Fatal(err)
	}
	return af
}

func (af *accountFixture) apply(f *fake) func(*Options) {
	return func(o *Options) {
		o.Sources.LoadIndex = af.st.LoadIndex
		o.Sources.SaveAccount = af.st.UpdateMetadata
		o.Sources.Generations = af.gens
		o.Sources.RefreshAccounts = func(context.Context, string) []codexapi.QuotaRefreshResult {
			af.mu.Lock()
			af.recordedAtPoll = f.recordedIDs()
			polls := af.polls
			af.mu.Unlock()
			if polls == nil {
				return nil
			}
			idx, _ := af.st.LoadIndex()
			var ids []string
			for _, a := range idx.Accounts {
				ids = append(ids, a.ID)
			}
			return polls(ids)
		}
	}
}

func (af *accountFixture) setPolls(fn func(ids []string) []codexapi.QuotaRefreshResult) {
	af.mu.Lock()
	af.polls = fn
	af.mu.Unlock()
}

func (af *accountFixture) account(t *testing.T, s *Service, id string) codex.ManagedAccount {
	t.Helper()
	for _, a := range s.Snapshot().Accounts.Sidebar {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("account %s not in the sidebar", id)
	return codex.ManagedAccount{}
}

func TestFullRefreshRecordsCurrentAuthBeforePollingAccounts(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	s, _ := newService(t, f, af.apply(f))
	refreshAll(t, s)
	af.mu.Lock()
	seen := af.recordedAtPoll
	af.mu.Unlock()
	if len(seen) == 0 || seen[0] != "acct-a" {
		t.Errorf("the credential was not recorded before the poll: %v", seen)
	}
	if got := s.Snapshot().Accounts.ActiveID; got != af.a.ID {
		t.Errorf("active %q", got)
	}
}

func TestPolledAccountQuotaIsStoredOnItsRow(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	s, _ := newService(t, f, af.apply(f))
	af.setPolls(func(ids []string) []codexapi.QuotaRefreshResult {
		var out []codexapi.QuotaRefreshResult
		for _, id := range ids {
			q := f.quota("polled", 42)
			out = append(out, codexapi.QuotaRefreshResult{AccountID: id, Generation: af.gens.Begin(id), Quota: &q, WorkspaceName: "Team " + id})
		}
		return out
	})
	wait(t, s.RefreshAllAccountQuotas())
	settle(t, s)
	row := af.account(t, s, af.b.ID)
	if row.CachedQuota == nil || row.CachedQuota.PrimaryUsedPercent != 42 || row.WorkspaceName != "Team "+af.b.ID {
		t.Errorf("row not updated: %+v", row)
	}
	if s.Snapshot().Refresh.AccountQuotas || len(s.Snapshot().Accounts.RefreshingIDs) != 0 {
		t.Error("polling flag left on")
	}
}

func TestStalePolledAccountResultIsDropped(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	s, _ := newService(t, f, af.apply(f))
	af.setPolls(func(ids []string) []codexapi.QuotaRefreshResult {
		q := f.quota("stale", 77)
		old := af.gens.Begin(af.b.ID)
		af.gens.Begin(af.b.ID) // a newer refresh superseded it
		return []codexapi.QuotaRefreshResult{{AccountID: af.b.ID, Generation: old, Quota: &q}}
	})
	wait(t, s.RefreshAllAccountQuotas())
	settle(t, s)
	if row := af.account(t, s, af.b.ID); row.CachedQuota != nil && row.CachedQuota.PrimaryUsedPercent == 77 {
		t.Error("superseded result was applied")
	}
}

func TestAccountListAuthFailureOnlyAffectsTheActiveAccount(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	s, _ := newService(t, f, af.apply(f))
	refreshAll(t, s)
	reauth := func(want string) func([]string) []codexapi.QuotaRefreshResult {
		return func(ids []string) []codexapi.QuotaRefreshResult {
			return []codexapi.QuotaRefreshResult{{
				AccountID: want, Generation: af.gens.Begin(want),
				Err: codexapi.ErrAuthenticationRequired, NeedsReauth: true,
			}}
		}
	}

	af.setPolls(reauth(af.b.ID))
	wait(t, s.RefreshAllAccountQuotas())
	settle(t, s)
	c := s.Snapshot().Codex
	if c.Quota == nil || !c.Authenticated {
		t.Fatal("another account's failure signed the active account out")
	}
	if !af.account(t, s, af.b.ID).RequiresReauth {
		t.Error("the failing account is not marked for re-authentication")
	}

	af.setPolls(reauth(af.a.ID))
	wait(t, s.RefreshAllAccountQuotas())
	settle(t, s)
	c = s.Snapshot().Codex
	if c.Quota != nil || c.Authenticated || c.StatusText != en.Text(l10n.KeyStatusLogin) {
		t.Errorf("active account failure not applied: %+v", c)
	}
}

func TestSwitchAccountClearsAndReloads(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	authB := testAuth(t, "acct-b", "b@example.com", "refresh-b")
	g := newGate()
	g.off.Store(true)
	f.quotaFn = func(_ context.Context, a codexapi.Auth) (codexapi.QuotaSnapshot, error) {
		if a.AccountID == "acct-b" {
			g.wait()
		}
		return f.quota("plan-"+a.AccountID, 5), nil
	}
	s, _ := newService(t, f, func(o *Options) {
		af.apply(f)(o)
		o.Sources.SwitchAccount = func(_ context.Context, id string) (codex.SwitchResult, error) {
			if err := af.st.SetActiveAccount(id, t0); err != nil {
				return codex.SwitchResult{}, err
			}
			f.setAuth(authB, nil)
			return codex.SwitchResult{Auth: authB, Account: codex.ManagedAccount{ID: id, PlanType: "pro"}}, nil
		}
	})
	refreshAll(t, s)
	if s.Snapshot().Codex.Quota.Plan != "plan-acct-a" {
		t.Fatal("setup")
	}
	g.off.Store(false)
	epoch := s.Epoch()
	if err := s.SwitchAccount(context.Background(), af.b.ID); err != nil {
		t.Fatal(err)
	}
	g.waitStarted(t)
	snap := s.Snapshot()
	if s.Epoch() == epoch || snap.Codex.Quota != nil {
		t.Errorf("previous account's data still shown: %+v", snap.Codex.Quota)
	}
	if snap.Accounts.ActiveID != af.b.ID || snap.Accounts.Switching || snap.Accounts.Message != "" {
		t.Errorf("accounts view: %+v", snap.Accounts)
	}
	if !snap.Codex.Authenticated {
		t.Error("switched account must be authenticated")
	}
	g.open()
	settle(t, s)
	if q := s.Snapshot().Codex.Quota; q == nil || q.Plan != "plan-acct-b" {
		t.Errorf("new account's quota: %+v", q)
	}
}

func TestSwitchFailureMapsToUserText(t *testing.T) {
	cases := []struct {
		err  error
		want l10n.Key
	}{
		{codex.ErrReauthRequired, l10n.KeyAccountsNeedsReauth},
		{codex.ErrCredentialStoreUnsupported, l10n.KeyCodexCredentialStoreUnsupported},
		{codex.ErrCredentialConfigInvalid, l10n.KeyCodexCredentialConfigInvalid},
		{codexapi.ErrAuthenticationRequired, l10n.KeyAccountsNeedsReauth},
		{codex.ErrExpiredAccessWithoutRefresh, l10n.KeyAccountsSwitchSessionExpired},
		{codex.ErrNotUsableAsCodexLogin, l10n.KeyAccountsSwitchInvalidCredential},
	}
	for _, tc := range cases {
		f := newFake(t)
		s, _ := newService(t, f, func(o *Options) {
			o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) {
				return codex.SwitchResult{}, tc.err
			}
		})
		if err := s.SwitchAccount(context.Background(), "x"); !errors.Is(err, tc.err) {
			t.Errorf("err %v", err)
		}
		c := s.Snapshot()
		if c.Codex.LastError != en.Text(tc.want) || c.Accounts.Switching || c.Accounts.Message != "" {
			t.Errorf("%v: %q switching=%v", tc.err, c.Codex.LastError, c.Accounts.Switching)
		}
	}
	f := newFake(t)
	s, _ := newService(t, f, func(o *Options) {
		o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) {
			return codex.SwitchResult{}, errors.New("disk on fire")
		}
	})
	_ = s.SwitchAccount(context.Background(), "x")
	if got := s.Snapshot().Codex.LastError; !strings.HasPrefix(got, en.Text(l10n.KeyAccountsSwitchFailed)) || !strings.Contains(got, "disk on fire") {
		t.Errorf("generic failure: %q", got)
	}
}

func TestConcurrentSwitchIsRefused(t *testing.T) {
	f := newFake(t)
	g := newGate()
	authB := testAuth(t, "acct-b", "b@example.com", "refresh-b")
	s, _ := newService(t, f, func(o *Options) {
		o.Sources.SwitchAccount = func(_ context.Context, id string) (codex.SwitchResult, error) {
			g.wait()
			f.setAuth(authB, nil)
			return codex.SwitchResult{Auth: authB, Account: codex.ManagedAccount{ID: id}}, nil
		}
	})
	done := make(chan error, 1)
	go func() { done <- s.SwitchAccount(context.Background(), "b") }()
	g.waitStarted(t)
	if !s.Snapshot().Accounts.Switching {
		t.Error("switching flag not shown")
	}
	if err := s.SwitchAccount(context.Background(), "b"); !errors.Is(err, ErrSwitchBusy) {
		t.Errorf("second switch: %v", err)
	}
	g.open()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	settle(t, s)
}

func TestSwitchWithoutSwitcherFails(t *testing.T) {
	s, _ := newService(t, newFake(t), nil)
	if err := s.SwitchAccount(context.Background(), "b"); err == nil {
		t.Error("expected an error")
	}
}

// ---- scheduling ----------------------------------------------------------------

func intervalPrefs(sec int) func(*Options) {
	return func(o *Options) { o.Preferences.RefreshIntervalSeconds = sec }
}

func TestTickStartsRefreshWhenTheIntervalElapses(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, intervalPrefs(300))
	refreshAll(t, s)
	if f.count("quota") != 1 {
		t.Fatal("setup")
	}
	s.Tick(t0.Add(299 * time.Second))
	settle(t, s)
	if f.count("quota") != 1 {
		t.Error("refreshed before the interval elapsed")
	}
	now := f.clk.Advance(300 * time.Second)
	s.Tick(now)
	settle(t, s)
	if f.count("quota") != 2 {
		t.Errorf("quota fetched %d times, want 2", f.count("quota"))
	}
	// The next deadline is measured from the completion of the second one.
	s.Tick(now.Add(299 * time.Second))
	settle(t, s)
	if f.count("quota") != 2 {
		t.Error("refreshed again too early")
	}
}

func TestTickNeverStartsASecondRefresh(t *testing.T) {
	f := newFake(t)
	g := newGate()
	f.quotaFn = func(context.Context, codexapi.Auth) (codexapi.QuotaSnapshot, error) {
		g.wait()
		return f.quota("p", 1), nil
	}
	s, _ := newService(t, f, intervalPrefs(300))
	first := s.Refresh(usage.RefreshForce)
	g.waitStarted(t)
	for i := 1; i <= 5; i++ {
		s.Tick(t0.Add(time.Duration(i*400) * time.Second))
	}
	g.open()
	wait(t, first)
	settle(t, s)
	if f.count("quota") != 1 {
		t.Errorf("quota fetched %d times during one refresh", f.count("quota"))
	}
}

func TestIdleTickDoesNotRepublish(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, intervalPrefs(3600))
	refreshAll(t, s)
	now := f.clk.Now()
	s.Tick(now)
	seq := s.Snapshot().Seq
	s.Tick(now)
	if got := s.Snapshot().Seq; got != seq {
		t.Errorf("an unchanged tick published (%d -> %d)", seq, got)
	}
	s.Tick(now.Add(5 * time.Minute)) // the countdown text changes
	if s.Snapshot().Seq == seq {
		t.Error("a changed countdown was not published")
	}
}

func TestPassedQuotaResetTriggersRefresh(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, intervalPrefs(100_000))
	refreshAll(t, s)
	reset := s.Snapshot().Codex.Quota.Primary.ResetsAt

	s.Tick(reset.Add(500 * time.Millisecond))
	settle(t, s)
	if f.count("quota") != 1 {
		t.Error("refreshed before the reset passed a full second")
	}
	now := f.clk.Advance(2*time.Hour + 2*time.Second)
	s.Tick(now)
	settle(t, s)
	if f.count("quota") != 2 {
		t.Errorf("quota fetched %d times, want 2", f.count("quota"))
	}
	if q := s.Snapshot().Codex.Quota; q == nil || !q.Primary.ResetsAt.After(now) {
		t.Errorf("new quota %+v", q)
	}
	s.Tick(now.Add(time.Second))
	settle(t, s)
	if f.count("quota") != 2 {
		t.Error("the same reset triggered twice")
	}
}

func TestPassedResetIsNotRetriggeredWhenItStaysInThePast(t *testing.T) {
	f := newFake(t)
	f.quotaFn = func(context.Context, codexapi.Auth) (codexapi.QuotaSnapshot, error) {
		q := f.quota("p", 1)
		q.Primary.ResetsAt = t0.Add(time.Hour) // the server keeps reporting the old reset
		return q, nil
	}
	s, _ := newService(t, f, intervalPrefs(100_000))
	refreshAll(t, s)
	now := f.clk.Advance(time.Hour + 5*time.Second)
	for i := 0; i < 4; i++ {
		s.Tick(now.Add(time.Duration(i) * time.Second))
		settle(t, s)
	}
	if n := f.count("quota"); n != 2 {
		t.Errorf("quota fetched %d times, want exactly one reset-triggered refresh", n)
	}
}

func TestIntervalChangeRearmsFromLastCompletion(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, intervalPrefs(600))
	refreshAll(t, s)
	now := f.clk.Advance(100 * time.Second)
	s.Tick(now)
	settle(t, s)
	if f.count("quota") != 1 {
		t.Fatal("refreshed early")
	}
	p := s.Preferences()
	p.RefreshIntervalSeconds = 60
	s.SetPreferences(p)
	s.Tick(now)
	settle(t, s)
	if f.count("quota") != 2 {
		t.Errorf("shorter interval ignored: %d fetches", f.count("quota"))
	}
}

func TestStartTicksOnItsOwn(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, func(o *Options) {
		o.TickInterval = 5 * time.Millisecond
		o.Preferences.RefreshIntervalSeconds = 1
	})
	s.Start(context.Background())
	for i := 0; i < 400 && f.count("quota") < 2; i++ {
		if i == 20 {
			f.clk.Advance(2 * time.Second)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f.count("quota") < 2 {
		t.Error("the tick loop never started a scheduled refresh")
	}
}

// ---- providers ------------------------------------------------------------------

type fakeGrok struct {
	mu       sync.Mutex
	state    GrokState
	refresh  int
	local    int
	onChange func()
}

func (g *fakeGrok) State(time.Time) GrokState { g.mu.Lock(); defer g.mu.Unlock(); return g.state }
func (g *fakeGrok) Refresh(context.Context) error {
	g.mu.Lock()
	g.refresh++
	cb := g.onChange
	g.mu.Unlock()
	if cb != nil {
		cb()
	}
	return nil
}
func (g *fakeGrok) RefreshLocal(context.Context) error {
	g.mu.Lock()
	g.local++
	g.mu.Unlock()
	return nil
}
func (g *fakeGrok) SetOnChange(fn func()) { g.mu.Lock(); g.onChange = fn; g.mu.Unlock() }
func (g *fakeGrok) count() int            { g.mu.Lock(); defer g.mu.Unlock(); return g.refresh }
func (g *fakeGrok) localCount() int       { g.mu.Lock(); defer g.mu.Unlock(); return g.local }

type fakeResetToday struct {
	mu       sync.Mutex
	state    ResetTodayState
	last     time.Time
	refresh  int
	forced   []bool
	pending  []alerts.Alert
	onChange func()
	now      func() time.Time
}

func (r *fakeResetToday) State(time.Time) ResetTodayState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}
func (r *fakeResetToday) Refresh(_ context.Context, force bool) error {
	r.mu.Lock()
	r.refresh++
	r.forced = append(r.forced, force)
	r.last = r.now()
	r.mu.Unlock()
	return nil
}
func (r *fakeResetToday) LastFetch() time.Time { r.mu.Lock(); defer r.mu.Unlock(); return r.last }
func (r *fakeResetToday) PendingAlerts(time.Time) []alerts.Alert {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending
}
func (r *fakeResetToday) SetOnChange(fn func()) { r.mu.Lock(); r.onChange = fn; r.mu.Unlock() }
func (r *fakeResetToday) count() int            { r.mu.Lock(); defer r.mu.Unlock(); return r.refresh }

// The provider strip shows every provider, so the Grok panel still runs the
// full Codex refresh.
func TestGrokPanelRefreshCoversEveryProvider(t *testing.T) {
	f := newFake(t)
	g := &fakeGrok{state: GrokState{Availability: GrokReady, Plan: "heavy"}}
	s, _ := newService(t, f, func(o *Options) {
		o.Grok = g
		o.Preferences.SelectedProvider = store.ProviderGrok
	})
	wait(t, s.Refresh(usage.RefreshForce))
	settle(t, s)
	if g.count() != 1 {
		t.Errorf("grok refreshed %d times", g.count())
	}
	if f.count("quota") != 1 || f.count("credits") != 1 {
		t.Errorf("Codex quota %d, credits %d", f.count("quota"), f.count("credits"))
	}
	if s.Snapshot().Grok.Plan != "heavy" || s.Snapshot().Provider != store.ProviderGrok {
		t.Errorf("grok state missing: %+v", s.Snapshot().Grok)
	}
}

type fakeClaude struct {
	mu     sync.Mutex
	state  ClaudeState
	forced []bool
	local  int
}

func (c *fakeClaude) State(time.Time) ClaudeState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}
func (c *fakeClaude) Refresh(_ context.Context, force bool) error {
	c.mu.Lock()
	c.forced = append(c.forced, force)
	c.mu.Unlock()
	return nil
}
func (c *fakeClaude) RefreshLocal(context.Context) error {
	c.mu.Lock()
	c.local++
	c.mu.Unlock()
	return nil
}
func (c *fakeClaude) SetOnChange(func()) {}
func (c *fakeClaude) localCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.local
}
func (c *fakeClaude) calls() []bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]bool(nil), c.forced...)
}

func TestOnlyManualRefreshForcesClaude(t *testing.T) {
	f := newFake(t)
	c := &fakeClaude{}
	s, _ := newService(t, f, func(o *Options) { o.Claude = c })
	wait(t, s.Refresh(usage.RefreshForce))
	settle(t, s)
	s.Tick(f.clk.Advance(time.Duration(s.Preferences().RefreshIntervalSeconds) * time.Second))
	settle(t, s)
	wait(t, s.RefreshQuota())
	settle(t, s)
	if got := c.calls(); !reflect.DeepEqual(got, []bool{true, false, true}) {
		t.Fatalf("forced = %v", got)
	}
}

func TestManualQuotaRefreshRestartsTheInterval(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, nil)
	refreshAll(t, s)
	interval := time.Duration(s.Preferences().RefreshIntervalSeconds) * time.Second
	s.Tick(f.clk.Advance(interval / 2))
	settle(t, s)
	wait(t, s.RefreshQuota())
	settle(t, s)
	quota := f.count("quota")
	s.Tick(f.clk.Advance(interval/2 + time.Second))
	settle(t, s)
	if f.count("quota") != quota {
		t.Fatal("the old deadline still fired after a manual refresh")
	}
	s.Tick(f.clk.Advance(interval / 2))
	settle(t, s)
	if f.count("quota") != quota+1 {
		t.Fatal("the restarted interval did not fire")
	}
}

func TestBusySectionDoesNotHoldBackScheduledRefresh(t *testing.T) {
	f := newFake(t)
	gate := newGate()
	defer gate.open()
	var armed atomic.Bool
	s, _ := newService(t, f, func(o *Options) {
		o.Sources.DryRunSessions = func(ctx context.Context) (sessions.Report, error) {
			if armed.Load() {
				gate.waitCtx(ctx)
			}
			return sessions.Report{}, nil
		}
	})
	refreshAll(t, s)
	quota := f.count("quota")
	armed.Store(true)
	s.RefreshSessionReport(true)
	gate.waitStarted(t)
	s.Tick(f.clk.Advance(time.Duration(s.Preferences().RefreshIntervalSeconds) * time.Second))
	deadline := time.Now().Add(5 * time.Second)
	for f.count("quota") == quota {
		if time.Now().After(deadline) {
			t.Fatal("a busy section held back the scheduled quota refresh")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestProviderChangeRepublishes(t *testing.T) {
	f := newFake(t)
	g := &fakeGrok{state: GrokState{Availability: GrokLoading}}
	s, _ := newService(t, f, func(o *Options) { o.Grok = g })
	seq := s.Snapshot().Seq
	g.mu.Lock()
	g.state = GrokState{Availability: GrokReady, Plan: "new"}
	cb := g.onChange
	g.mu.Unlock()
	cb()
	snap := s.Snapshot()
	if snap.Seq == seq || snap.Grok.Plan != "new" {
		t.Errorf("change not published: %+v", snap.Grok)
	}
}

func TestTickRefreshesResetTodayOnItsOwnInterval(t *testing.T) {
	f := newFake(t)
	rt := &fakeResetToday{now: f.clk.Now, pending: []alerts.Alert{{ID: "rt-1", Kind: alerts.KindQuota}}}
	s, r := newService(t, f, func(o *Options) {
		o.ResetToday = rt
		o.Preferences.RateLimitResetTodayRefreshIntervalSeconds = 600
		o.Preferences.RateLimitResetTodayAlertsEnabled = true
	})
	s.Tick(f.clk.Now())
	settle(t, s)
	if rt.count() != 1 {
		t.Fatalf("first tick refreshed %d times", rt.count())
	}
	s.Tick(f.clk.Advance(599 * time.Second))
	settle(t, s)
	if rt.count() != 1 {
		t.Error("refreshed before the interval")
	}
	s.Tick(f.clk.Advance(2 * time.Second))
	settle(t, s)
	if rt.count() != 2 {
		t.Errorf("not refreshed after the interval: %d", rt.count())
	}
	if r.alertCount() != 1 {
		t.Errorf("pending alerts must be delivered once (deduped): %d", r.alertCount())
	}
	s.RefreshResetToday(true)
	settle(t, s)
	if rt.forced[len(rt.forced)-1] != true {
		t.Error("force flag not passed on")
	}
	// Alerts alone keep it polling, whichever provider the panel shows.
	p := s.Preferences()
	p.ShowsRateLimitResetToday = false
	p.SelectedProvider = store.ProviderClaude
	s.SetPreferences(p)
	before := rt.count()
	s.Tick(f.clk.Advance(time.Hour))
	settle(t, s)
	if rt.count() != before+1 {
		t.Error("reset-today alerts stopped polling on another provider")
	}
	// Hidden card, alerts off, no widget: nothing to refresh.
	p.RateLimitResetTodayAlertsEnabled = false
	s.SetPreferences(p)
	before = rt.count()
	s.Tick(f.clk.Advance(time.Hour))
	settle(t, s)
	if rt.count() != before {
		t.Error("an unused reset-today feed was refreshed")
	}
}

// ---- export, alerts, widgets -------------------------------------------------------

func TestStatusJSONExportHasNoCredentials(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, func(o *Options) { o.Preferences.ExportsStatusJSON = true })
	var statusPath string
	s2, _ := newService(t, f, func(o *Options) { o.Preferences.ExportsStatusJSON = true; statusPath = o.StatusPath })
	showPanel(s2)
	_ = s
	refreshAll(t, s2)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("status.json is not JSON: %v", err)
	}
	if doc["quota"] == nil || doc["cost"] == nil {
		t.Errorf("status.json misses quota/cost: %s", data)
	}
	f.mu.Lock()
	secrets := []string{f.auth.Tokens.AccessToken, f.auth.Tokens.RefreshToken, f.auth.Tokens.IDToken}
	f.mu.Unlock()
	for _, sec := range secrets {
		if sec != "" && strings.Contains(string(data), sec) {
			t.Fatal("status.json contains a credential")
		}
	}
}

func TestStatusJSONExportOffWritesNothing(t *testing.T) {
	f := newFake(t)
	var path string
	s, _ := newService(t, f, func(o *Options) { path = o.StatusPath })
	refreshAll(t, s)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("status.json written although the preference is off: %v", err)
	}
}

func TestQuotaAndCreditAlertsAreDedupedAcrossRefreshes(t *testing.T) {
	f := newFake(t)
	f.quotaFn = func(context.Context, codexapi.Auth) (codexapi.QuotaSnapshot, error) {
		q := f.quota("p", 95)
		q.Primary.ResetsAt = t0.Add(2 * time.Hour)
		return q, nil
	}
	f.creditsFn = func(context.Context, codexapi.Auth) (codexapi.ResetCreditsSnapshot, error) {
		return codexapi.ResetCreditsSnapshot{AvailableCount: 1, UpdatedAt: t0, Credits: []codexapi.ResetCredit{
			{ID: "soon", Status: "available", ExpiresAt: t0.Add(3 * 24 * time.Hour), RemainingSeconds: 3 * 86400},
		}}, nil
	}
	var path string
	s, r := newService(t, f, func(o *Options) {
		path = o.AlertsPath
		o.Preferences.QuotaAlertsEnabled, o.Preferences.ResetCreditAlertsEnabled = true, true
	})
	refreshAll(t, s)
	if got := r.alertCount(); got != 2 {
		t.Fatalf("first refresh: %d alerts, want a quota and a credit alert", got)
	}
	refreshAll(t, s)
	if got := r.alertCount(); got != 2 {
		t.Errorf("second refresh repeated alerts: %d", got)
	}
	if data, err := os.ReadFile(alerts.QuotaPath(path)); err != nil || !strings.Contains(string(data), "highest") {
		t.Errorf("quota alert state: %v %q", err, data)
	}
	// A new service sharing the file does not repeat them either.
	s2, r2 := newService(t, f, func(o *Options) {
		o.AlertsPath = path
		o.Preferences.QuotaAlertsEnabled, o.Preferences.ResetCreditAlertsEnabled = true, true
	})
	refreshAll(t, s2)
	if r2.alertCount() != 0 {
		t.Errorf("restart repeated alerts: %d", r2.alertCount())
	}
}

func TestDisabledAlertsStaySilent(t *testing.T) {
	f := newFake(t)
	f.quotaFn = func(context.Context, codexapi.Auth) (codexapi.QuotaSnapshot, error) { return f.quota("p", 100), nil }
	s, r := newService(t, f, nil)
	refreshAll(t, s)
	if r.alertCount() != 0 {
		t.Errorf("%d alerts although alerts are off", r.alertCount())
	}
}

func TestWidgetSnapshotIsWrittenForActiveWidgets(t *testing.T) {
	f := newFake(t)
	var mu sync.Mutex
	var written []export.Snapshot
	s, _ := newService(t, f, func(o *Options) {
		o.WidgetSink = func(sn export.Snapshot) error {
			mu.Lock()
			written = append(written, sn)
			mu.Unlock()
			return nil
		}
	})
	s.SetWidgetRequirements(export.RequireProviderQuota)
	settle(t, s)
	mu.Lock()
	n := len(written)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("widget snapshots written: %d", n)
	}
	var codexBlock *export.ProviderSnapshot
	for i := range written[0].Providers {
		if written[0].Providers[i].Provider == export.ProviderCodex {
			codexBlock = &written[0].Providers[i]
		}
	}
	if codexBlock == nil || codexBlock.Availability != export.Available || len(codexBlock.Quota) == 0 {
		t.Fatalf("codex block: %+v", codexBlock)
	}
	if got := s.WidgetSnapshot(f.clk.Now()); len(got.Providers) != 3 {
		t.Errorf("providers: %d", len(got.Providers))
	}
}

func TestWidgetCadenceFollowsItsOwnInterval(t *testing.T) {
	f := newFake(t)
	var mu sync.Mutex
	writes := 0
	s, _ := newService(t, f, func(o *Options) {
		o.Preferences.RefreshIntervalSeconds = 100_000
		o.Preferences.WidgetRefreshIntervalSeconds = 600
		o.WidgetSink = func(export.Snapshot) error { mu.Lock(); writes++; mu.Unlock(); return nil }
	})
	s.SetWidgetRequirements(export.RequireProviderQuota)
	settle(t, s)
	s.Tick(f.clk.Advance(599 * time.Second))
	settle(t, s)
	mu.Lock()
	got := writes
	mu.Unlock()
	if got != 1 {
		t.Fatalf("early tick wrote a widget: %d", got)
	}
	s.Tick(f.clk.Advance(2 * time.Second))
	settle(t, s)
	mu.Lock()
	got = writes
	mu.Unlock()
	if got != 2 {
		t.Errorf("widget not rewritten after its interval: %d", got)
	}
}

func TestNoWidgetSinkNoWrites(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, nil)
	s.SetWidgetRequirements(export.RequireAll)
	settle(t, s)
	if f.count("quota") != 1 {
		t.Errorf("widget requirements must still collect data: %d", f.count("quota"))
	}
}

// ---- preferences ---------------------------------------------------------------------

func TestLanguageChangeRelabelsTheSnapshot(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, nil)
	before := refreshAll(t, s).Codex
	p := s.Preferences()
	p.Language = store.LanguageSimplifiedChinese
	s.SetPreferences(p)
	after := s.Snapshot()
	if after.Language != l10n.SimplifiedChinese {
		t.Errorf("language %v", after.Language)
	}
	if len(after.Codex.QuotaLines) == 0 || after.Codex.QuotaLines[0].Title == before.QuotaLines[0].Title {
		t.Errorf("quota lines were not relabelled: %+v", after.Codex.QuotaLines)
	}
	if after.Codex.Quota == nil || after.Codex.Quota.Plan != before.Quota.Plan {
		t.Error("data changed with the language")
	}
}

func TestEnablingCostAndHeatmapFetchesThem(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, func(o *Options) {
		o.Preferences.ShowsCostSummary, o.Preferences.ShowsTokenUsageHeatmap = false, false
	})
	refreshAll(t, s)
	if f.count("scan") != 0 || f.count("profile") != 0 {
		t.Fatalf("hidden sections were scanned: %v", f.calls)
	}
	p := s.Preferences()
	p.ShowsCostSummary, p.ShowsTokenUsageHeatmap = true, true
	s.SetPreferences(p)
	settle(t, s)
	if f.count("scan") == 0 || f.count("profile") == 0 {
		t.Errorf("newly enabled sections were not fetched: %v", f.calls)
	}
	if c := s.Snapshot().Codex; c.Cost == nil || len(c.Heatmap.AllDevices) == 0 {
		t.Errorf("sections empty after enabling: cost=%v", c.Cost)
	}
}

func TestCostRangeChangeRescansTheSelectedRange(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, func(o *Options) { o.Preferences.APICostSummaryRange = store.CostRangeToday })
	refreshAll(t, s)
	scans := f.count("scan")
	p := s.Preferences()
	p.APICostSummaryRange = store.CostRangeThisMonth
	s.SetPreferences(p)
	settle(t, s)
	if f.count("scan") <= scans {
		t.Error("a new range must be scanned")
	}
	if c := s.Snapshot().Codex; c.DisplayedRange != store.CostRangeThisMonth || c.DisplayedCost == nil {
		t.Errorf("displayed range %v", c.DisplayedRange)
	}
}

func TestSelectProviderOnlyChangesTheView(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, nil)
	refreshAll(t, s)
	s.SelectProvider(store.ProviderGrok)
	if s.Snapshot().Provider != store.ProviderGrok || s.Snapshot().Codex.Quota == nil {
		t.Error("provider switch must keep the Codex data")
	}
}

// ---- sessions ------------------------------------------------------------------------

func TestSessionReportRepairAndRecentSessions(t *testing.T) {
	f := newFake(t)
	var mu sync.Mutex
	repaired := false
	s, _ := newService(t, f, func(o *Options) {
		o.Preferences.ShowsRecentSessions = true
		o.Sources.DryRunSessions = func(context.Context) (sessions.Report, error) {
			mu.Lock()
			defer mu.Unlock()
			if repaired {
				return sessions.Report{}, nil
			}
			return sessions.Report{MissingIndexIDs: []string{"a", "b"}, PlannedEntries: 4}, nil
		}
		o.Sources.RepairSessions = func(context.Context) (sessions.Report, error) {
			mu.Lock()
			repaired = true
			mu.Unlock()
			return sessions.Report{MissingIndexIDs: []string{"a", "b"}, BackupPath: "/x/session_index.jsonl.bak", PlannedEntries: 4}, nil
		}
		o.Sources.ScanRecentSessions = func(_ context.Context, limit int) (sessions.Summary, error) {
			return sessions.Summary{Items: []sessions.Item{{ID: "s1", Title: "Fix bug", UpdatedAt: t0}}}, nil
		}
	})
	showPanel(s)
	c := refreshAll(t, s).Codex
	if c.SessionReport == nil || len(c.SessionReport.MissingIndexIDs) != 2 || len(c.SessionLines) == 0 {
		t.Fatalf("report: %+v", c.SessionReport)
	}
	if len(c.RecentSessions) != 1 || c.RecentSessions[0].Title != "Fix bug" || len(c.RecentSessionLines) == 0 {
		t.Errorf("recent sessions: %+v", c.RecentSessions)
	}
	wait(t, s.RepairSessions())
	settle(t, s)
	c = s.Snapshot().Codex
	if !strings.Contains(strings.Join(lineValues(c.SessionLines), "|")+c.SessionText, "session_index.jsonl.bak") {
		t.Errorf("backup name missing from the repair result: %q %+v", c.SessionText, c.SessionLines)
	}
}

func TestSessionFailuresAreShownNotFatal(t *testing.T) {
	f := newFake(t)
	s, _ := newService(t, f, func(o *Options) {
		o.Sources.DryRunSessions = func(context.Context) (sessions.Report, error) {
			return sessions.Report{}, errors.New("index unreadable")
		}
	})
	showPanel(s)
	c := refreshAll(t, s).Codex
	if c.Quota == nil {
		t.Error("session failure broke the quota")
	}
	if !strings.Contains(c.SessionText+strings.Join(lineValues(c.SessionLines), "|"), "index unreadable") {
		t.Errorf("session error not shown: %q %+v", c.SessionText, c.SessionLines)
	}
}

func lineValues(ls []DetailLine) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Value)
	}
	return out
}

// ---- cost queries --------------------------------------------------------------------

func TestQueryCostSharesOneScanAndCachesDisplayableResults(t *testing.T) {
	f := newFake(t)
	g := newGate()
	f.dailyFn = func(context.Context, codexapi.Auth) (usage.Summary, error) {
		g.wait()
		return usage.Summary{Source: usage.SourceOnlineAnalytics, Confidence: usage.ConfidencePriced, Totals: usage.Totals{TotalTokens: 9}}, nil
	}
	f.scanFn = func(context.Context, []usage.Query) (map[string]usage.Summary, error) {
		return map[string]usage.Summary{}, nil
	}
	s, _ := newService(t, f, func(o *Options) { o.Preferences.ShowsQuotaEstimateSummary = false })
	wait(t, s.RefreshQuota())
	settle(t, s)
	g.off.Store(true)
	f.mu.Lock()
	f.calls = map[string]int{}
	f.mu.Unlock()
	g.off.Store(false)

	r := usage.Today(t0, time.UTC)
	var wg sync.WaitGroup
	results := make([]usage.Summary, 3)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sum, err := s.QueryCost(context.Background(), r)
			if err != nil {
				t.Error(err)
			}
			results[i] = sum
		}()
	}
	g.waitStarted(t)
	time.Sleep(20 * time.Millisecond)
	g.open()
	wg.Wait()
	for _, sum := range results {
		if sum.Totals.TotalTokens != 9 {
			t.Errorf("result %+v", sum.Totals)
		}
	}
	if n := f.count("daily"); n != 1 {
		t.Errorf("online analytics fetched %d times for 3 callers", n)
	}
	if _, err := s.QueryCost(context.Background(), r); err != nil || f.count("daily") != 1 {
		t.Errorf("cached query refetched: %v %d", err, f.count("daily"))
	}
}

func TestQueryCostWithoutLoginFails(t *testing.T) {
	f := newFake(t)
	f.setAuth(f.auth, &AuthLoadError{Kind: AuthFileInvalid})
	s, _ := newService(t, f, nil)
	if _, err := s.QueryCost(context.Background(), usage.Today(t0, time.UTC)); err == nil {
		t.Error("expected an error without a credential")
	}
}
