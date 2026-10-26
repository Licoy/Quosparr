package runway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
)

func TestCurrentResetCreditsAlsoUpdatesAccountCache(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	s, _ := newService(t, f, af.apply(f))
	f.creditsFn = func(context.Context, codexapi.Auth) (codexapi.ResetCreditsSnapshot, error) {
		return codexapi.ResetCreditsSnapshot{AvailableCount: 0, UpdatedAt: f.clk.Now()}, nil
	}
	wait(t, s.RefreshResetCredits())
	a := af.account(t, s, af.a.ID)
	if a.ResetCredits == nil || a.ResetCredits.AvailableCount == nil || *a.ResetCredits.AvailableCount != 0 {
		t.Fatalf("current zero not cached: %+v", a.ResetCredits)
	}
	if af.account(t, s, af.b.ID).ResetCredits != nil {
		t.Fatal("current credits crossed account identity")
	}
	f.creditsFn = func(context.Context, codexapi.Auth) (codexapi.ResetCreditsSnapshot, error) {
		return codexapi.ResetCreditsSnapshot{}, errors.New("synthetic provider failure")
	}
	wait(t, s.RefreshResetCredits())
	a = af.account(t, s, af.a.ID)
	if a.ResetCredits.AvailableCount == nil || *a.ResetCredits.AvailableCount != 0 || a.ResetCredits.LastError != "reset_credits_unavailable" {
		t.Fatalf("failed refresh lost known zero: %+v", a.ResetCredits)
	}
}

func TestPolledResetCreditsFailurePreservesQuotaAndUnknown(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	s, _ := newService(t, f, af.apply(f))
	q := f.quota("plus", 23)
	s.applyAccountResults([]codexapi.QuotaRefreshResult{{AccountID: af.b.ID, Generation: af.gens.Begin(af.b.ID), Quota: &q, ResetCreditsErr: errors.New("sensitive transport detail")}}, s.Epoch())
	b := af.account(t, s, af.b.ID)
	if b.CachedQuota == nil || b.CachedQuota.PrimaryUsedPercent != 23 || b.ResetCredits == nil || b.ResetCredits.AvailableCount != nil || b.ResetCredits.LastError != "reset_credits_unavailable" {
		t.Fatalf("failure not isolated: %+v", b)
	}
	credits := codexapi.ResetCreditsSnapshot{AvailableCount: 5, UpdatedAt: f.clk.Now()}
	s.applyAccountResults([]codexapi.QuotaRefreshResult{{AccountID: af.b.ID, Generation: af.gens.Begin(af.b.ID), Quota: &q, ResetCredits: &credits}}, s.Epoch())
	b = af.account(t, s, af.b.ID)
	if *b.ResetCredits.AvailableCount != 5 || b.ResetCredits.LastError != "" {
		t.Fatalf("success did not clear error: %+v", b.ResetCredits)
	}
	if af.account(t, s, af.a.ID).ResetCredits != nil {
		t.Fatal("non-current credits altered current account")
	}
}

func TestCurrentResetCreditCacheFailureIsReported(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	s, _ := newService(t, f, func(o *Options) {
		af.apply(f)(o)
		o.Sources.SaveAccount = func(codex.ManagedAccount) error { return errors.New("synthetic cache write failure") }
	})
	auth, gen, err := s.loadValidAuth(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.fetchCreditsStep(context.Background(), auth, gen); err == nil || err.Error() != "synthetic cache write failure" {
		t.Fatalf("cache failure swallowed: %v", err)
	}
	if s.Snapshot().Codex.ResetCredits == nil {
		t.Fatal("cache failure discarded the fetched credits")
	}
}

func TestLateResetCreditsCannotUpdateAnyAccountCache(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	g := newGate()
	f.creditsFn = func(context.Context, codexapi.Auth) (codexapi.ResetCreditsSnapshot, error) {
		g.wait()
		return codexapi.ResetCreditsSnapshot{AvailableCount: 9, UpdatedAt: f.clk.Now()}, nil
	}
	s, _ := newService(t, f, af.apply(f))
	done := s.RefreshResetCredits()
	g.waitStarted(t)
	next := testAuth(t, "acct-b", "b@example.com", "refresh-b")
	s.mu.Lock()
	s.clearAccountScopedLocked(&next)
	s.mu.Unlock()
	g.open()
	wait(t, done)
	for _, id := range []string{af.a.ID, af.b.ID} {
		if af.account(t, s, id).ResetCredits != nil {
			t.Fatal("stale credit response entered account cache")
		}
	}
}

func TestFullRefreshSkipsCurrentAccountDuplicateCreditRequest(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	skipped := make(chan string, 2)
	s, _ := newService(t, f, func(o *Options) {
		af.apply(f)(o)
		o.Sources.RefreshAccounts = func(_ context.Context, skipID string) []codexapi.QuotaRefreshResult { skipped <- skipID; return nil }
	})
	refreshAll(t, s)
	if skip := <-skipped; skip != af.a.ID {
		t.Fatalf("full refresh skip = %q, want %q", skip, af.a.ID)
	}
	a := af.account(t, s, af.a.ID)
	if a.ResetCredits == nil || a.ResetCredits.AvailableCount == nil {
		t.Fatal("full refresh did not cache current credits")
	}
	wait(t, s.RefreshAllAccountQuotas())
	if skip := <-skipped; skip != "" {
		t.Fatalf("manual account refresh skipped %q", skip)
	}
}

func TestAccountCreditExpiryFiltersAndUpdateOrdering(t *testing.T) {
	now := t0
	soon, later := now.Add(time.Hour), now.Add(2*time.Hour)
	credits := codexapi.ResetCreditsSnapshot{AvailableCount: 2, UpdatedAt: now, Credits: []codexapi.ResetCredit{
		{ID: "private-later", Status: "available", ExpiresAt: later},
		{Status: "used", ExpiresAt: now.Add(time.Minute)},
		{Status: "expired", ExpiresAt: soon},
		{Status: "available", ExpiresAt: now.Add(-time.Minute)},
		{Status: "available"},
		{Status: "available", ExpiresAt: now},
		{Status: "available", ExpiresAt: soon},
	}}
	got := cacheAccountResetCredits(codex.ManagedAccount{}, credits, now)
	if len(got.ResetCredits.AvailableExpiries) != 2 || !got.ResetCredits.AvailableExpiries[0].Equal(soon) || !got.ResetCredits.AvailableExpiries[1].Equal(later) {
		t.Fatalf("unavailable or invalid expiry cached: %+v", got.ResetCredits)
	}
	stale := credits
	stale.UpdatedAt = now.Add(-time.Minute)
	stale.Credits = nil
	if next := cacheAccountResetCredits(got, stale, now); len(next.ResetCredits.AvailableExpiries) != 2 {
		t.Fatal("older snapshot erased expiry details")
	}
	noDetails := credits
	noDetails.UpdatedAt = now.Add(time.Minute)
	noDetails.Credits = nil
	if next := cacheAccountResetCredits(got, noDetails, now); len(next.ResetCredits.AvailableExpiries) != 0 {
		t.Fatal("count-only response reused unsupported expiry")
	}
	zero := credits
	zero.AvailableCount = 0
	if next := cacheAccountResetCredits(got, zero, now); len(next.ResetCredits.AvailableExpiries) != 0 {
		t.Fatal("zero count retained expiry")
	}
	// Cards already expired by the time a response is applied are not cached.
	if next := cacheAccountResetCredits(codex.ManagedAccount{}, credits, later); len(next.ResetCredits.AvailableExpiries) != 0 {
		t.Fatal("expired in-flight response retained expiry")
	}
}

func TestCurrentAndPolledCreditExpiryCaches(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	s, _ := newService(t, f, af.apply(f))
	now := f.clk.Now()
	expiry := now.Add(time.Hour)
	credits := codexapi.ResetCreditsSnapshot{AvailableCount: 1, UpdatedAt: now, Credits: []codexapi.ResetCredit{{Status: "available", ExpiresAt: expiry}}}
	f.creditsFn = func(context.Context, codexapi.Auth) (codexapi.ResetCreditsSnapshot, error) { return credits, nil }
	wait(t, s.RefreshResetCredits())
	if got := af.account(t, s, af.a.ID).ResetCredits.NextAvailableExpiry(now); !got.Equal(expiry) {
		t.Fatalf("current expiry: %v", got)
	}
	q := f.quota("plus", 23)
	s.applyAccountResults([]codexapi.QuotaRefreshResult{{AccountID: af.b.ID, Generation: af.gens.Begin(af.b.ID), Quota: &q, ResetCredits: &credits}}, s.Epoch())
	if got := af.account(t, s, af.b.ID).ResetCredits.NextAvailableExpiry(now); !got.Equal(expiry) {
		t.Fatalf("polled expiry: %v", got)
	}
}
