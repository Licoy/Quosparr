package codex

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAccountResetCreditsCacheCompatibility(t *testing.T) {
	a := ManagedAccount{ID: "test", AuthMode: AuthModeOAuth, CreatedAt: time.Now()}
	encode := func(a ManagedAccount) ManagedAccount {
		t.Helper()
		data, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		var got ManagedAccount
		if err = json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := encode(a); got.ResetCredits != nil {
		t.Fatal("legacy account must have unknown credits")
	}
	zero, now := 0, time.Now().UTC().Truncate(time.Second)
	a = a.ApplyResetCredits(&zero, &now, "")
	got := encode(a)
	if got.ResetCredits == nil || got.ResetCredits.AvailableCount == nil || *got.ResetCredits.AvailableCount != 0 || !got.ResetCredits.UpdatedAt.Equal(now) {
		t.Fatalf("zero lost: %+v", got.ResetCredits)
	}
	a = a.ApplyResetCredits(nil, nil, "reset_credits_unavailable")
	got = encode(a)
	if got.ResetCredits.AvailableCount == nil || *got.ResetCredits.AvailableCount != 0 || got.ResetCredits.LastError != "reset_credits_unavailable" {
		t.Fatalf("failed refresh: %+v", got.ResetCredits)
	}
	got = encode(ManagedAccount{ID: "test", AuthMode: AuthModeOAuth, CreatedAt: now}.ApplyResetCredits(nil, nil, "reset_credits_unavailable"))
	if got.ResetCredits.AvailableCount != nil {
		t.Fatal("failure must not invent zero")
	}
	count := 3
	got = encode(a.ApplyResetCredits(&count, &now, ""))
	if *got.ResetCredits.AvailableCount != 3 || got.ResetCredits.LastError != "" {
		t.Fatalf("success: %+v", got.ResetCredits)
	}
}

func TestOlderMetadataCannotEraseResetCredits(t *testing.T) {
	e := newEnv(t)
	a := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "credits", refresh: "refresh-credits"}), false)
	count, now := 4, time.Now().UTC()
	fresh := a.ApplyResetCredits(&count, &now, "")
	fresh.ResetCredits.AvailableExpiries = []time.Time{now.Add(time.Hour)}
	noErr(t, e.store.UpdateMetadata(fresh))
	// Simulate a quota fetch that read the row before the credit refresh.
	stale := a.ApplyQuota(QuotaSummary{Plan: "plus", UpdatedAt: now})
	noErr(t, e.store.UpdateMetadata(stale))
	idx := must(e.store.LoadIndex())
	got, _ := idx.Account(a.ID)
	if got.ResetCredits == nil || *got.ResetCredits.AvailableCount != 4 || len(got.ResetCredits.AvailableExpiries) != 1 {
		t.Fatal("old quota write erased credits")
	}
	earlier := now.Add(-time.Minute)
	oldCount := 1
	noErr(t, e.store.UpdateMetadata(a.ApplyResetCredits(&oldCount, &earlier, "")))
	idx = must(e.store.LoadIndex())
	got, _ = idx.Account(a.ID)
	if *got.ResetCredits.AvailableCount != 4 || len(got.ResetCredits.AvailableExpiries) != 1 {
		t.Fatal("older credit snapshot replaced newer result")
	}
	// A failed refresh retains the successful count and protects its error state.
	later := now.Add(time.Minute)
	noErr(t, e.store.UpdateMetadata(got.ApplyResetCredits(nil, &later, "reset_credits_unavailable")))
	noErr(t, e.store.UpdateMetadata(fresh))
	idx = must(e.store.LoadIndex())
	got, _ = idx.Account(a.ID)
	if *got.ResetCredits.AvailableCount != 4 || got.ResetCredits.LastError != "reset_credits_unavailable" {
		t.Fatalf("stale write erased failure: %+v", got.ResetCredits)
	}
}

func TestRejectNegativeCachedResetCredits(t *testing.T) {
	count := -1
	data, err := json.Marshal(ManagedAccount{ID: "bad", AuthMode: AuthModeOAuth, CreatedAt: time.Now(), ResetCredits: &CachedAccountResetCredits{AvailableCount: &count}})
	if err != nil {
		t.Fatal(err)
	}
	var got ManagedAccount
	if json.Unmarshal(data, &got) == nil {
		t.Fatal("negative cached credits must fail validation")
	}
}

func TestCachedCreditExpirySerializationAndTimeProgression(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	one, later := now.Add(time.Hour), now.Add(2*time.Hour)
	count := 2
	account := ManagedAccount{ID: "expiry", AuthMode: AuthModeOAuth, CreatedAt: now}.ApplyResetCredits(&count, &now, "")
	account.ResetCredits.AvailableExpiries = []time.Time{later, one}
	data, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ManagedAccount
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	cache := decoded.ResetCredits
	if got := cache.NextAvailableExpiry(now); !got.Equal(one) {
		t.Fatalf("nearest expiry: %v", got)
	}
	if got := cache.NextAvailableExpiry(one); !got.Equal(later) {
		t.Fatalf("expired card was not skipped: %v", got)
	}
	if got := cache.NextAvailableExpiry(later); !got.IsZero() {
		t.Fatalf("expired details must be unknown: %v", got)
	}
	failedAt := now.Add(time.Minute)
	failed := decoded.ApplyResetCredits(nil, &failedAt, "reset_credits_unavailable")
	if len(failed.ResetCredits.AvailableExpiries) != 2 {
		t.Fatal("failed read lost expiry details")
	}
	zero := 0
	cleared := failed.ApplyResetCredits(&zero, &failedAt, "")
	if len(cleared.ResetCredits.AvailableExpiries) != 0 || !cleared.ResetCredits.NextAvailableExpiry(now).IsZero() {
		t.Fatal("successful zero retained expiry")
	}
	// Legacy cached counts did not contain expiry details.
	old := CachedAccountResetCredits{AvailableCount: &count}
	if !old.NextAvailableExpiry(now).IsZero() {
		t.Fatal("missing details must not invent expiry")
	}
}
