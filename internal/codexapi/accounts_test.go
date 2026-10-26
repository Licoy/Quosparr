package codexapi

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestGenerations(t *testing.T) {
	var g Generations
	a1 := g.Begin("a")
	b1 := g.Begin("b")
	if !g.Valid("a", a1) || !g.Valid("b", b1) {
		t.Fatal("fresh generations must be valid")
	}
	a2 := g.Begin("a")
	if g.Valid("a", a1) || !g.Valid("a", a2) || !g.Valid("b", b1) {
		t.Fatal("a newer refresh supersedes only its own account")
	}
	g.Invalidate("a")
	if g.Valid("a", a2) || !g.Valid("b", b1) {
		t.Fatal("invalidate drops one account")
	}
	g.InvalidateAll()
	if g.Valid("b", b1) {
		t.Fatal("invalidate all drops everything")
	}
	if b2 := g.Begin("b"); !g.Valid("b", b2) || g.Valid("b", b1) {
		t.Fatal("generation after invalidate all")
	}
	if g.Valid("never", 0) {
		t.Fatal("zero generation is never valid")
	}
}

func refresherFor(srv *recorder, targets []AccountTarget, cred func(context.Context, string) (Auth, error)) AccountQuotaRefresher {
	return AccountQuotaRefresher{
		Client:      newQuotaClient(srv),
		Accounts:    func(context.Context) ([]AccountTarget, error) { return targets, nil },
		Credential:  cred,
		Generations: &Generations{},
	}
}

func tokenCred(_ context.Context, id string) (Auth, error) {
	return Auth{AccessToken: "tok-" + id, AccountID: id}, nil
}

func TestRefreshAllSuccessKeepsOrderAndIdentity(t *testing.T) {
	srv := newRecorder(t, func(r recorded) (int, string) {
		if r.Path == "/accounts" {
			return 200, `{"items":[{"id":"team","name":"Acme"}]}`
		}
		return 200, usageFixture
	})
	targets := []AccountTarget{
		{ID: "plus", Tier: TierPlus},
		{ID: "key", APIKey: true},
		{ID: "team", Tier: TierTeam},
		{ID: "named", Tier: TierTeam, HasWorkspaceName: true},
	}
	r := refresherFor(srv, targets, tokenCred)
	results := r.RefreshAll(context.Background(), "")
	if len(results) != 4 {
		t.Fatalf("%d results", len(results))
	}
	for i, res := range results {
		if res.AccountID != targets[i].ID || res.Generation == 0 || !r.Generations.Valid(res.AccountID, res.Generation) {
			t.Errorf("%d: %+v", i, res)
		}
	}
	if results[0].Quota == nil || results[0].WorkspaceName != "" || results[0].Err != nil {
		t.Errorf("plus %+v", results[0])
	}
	if !results[1].APIKey || results[1].Quota != nil || results[1].Err != nil {
		t.Errorf("api key %+v", results[1])
	}
	if results[2].WorkspaceName != "Acme" || results[2].Quota == nil || results[2].Auth.AccessToken != "tok-team" {
		t.Errorf("team %+v", results[2])
	}
	for _, req := range srv.all() {
		if req.Header.Get("Authorization") == "Bearer tok-key" {
			t.Error("API key account must not hit the quota endpoint")
		}
		if req.Path == "/accounts" && req.Header.Get("ChatGPT-Account-Id") == "named" {
			t.Error("known workspace name must not be refetched")
		}
	}
}

func TestWorkspaceNameFailureKeepsQuota(t *testing.T) {
	srv := newRecorder(t, func(r recorded) (int, string) {
		if r.Path == "/accounts" {
			return 403, `{}`
		}
		return 200, usageFixture
	})
	res := refresherFor(srv, []AccountTarget{{ID: "team", Tier: TierTeam}}, tokenCred).Refresh(context.Background(), "team")
	if res.Quota == nil || res.Err != nil || res.Warning != WorkspaceMetadataUnavailable || res.ErrorMessage() != WorkspaceMetadataUnavailable {
		t.Fatalf("%+v", res)
	}
}

func TestUnauthorizedQuotaNeedsReauth(t *testing.T) {
	srv := newRecorder(t, func(recorded) (int, string) { return 401, `{}` })
	res := refresherFor(srv, []AccountTarget{{ID: "a", Tier: TierPlus}}, tokenCred).Refresh(context.Background(), "a")
	if res.Quota != nil || !res.NeedsReauth || !errors.Is(res.Err, ErrAuthenticationRequired) || res.ErrorMessage() == "" {
		t.Fatalf("%+v", res)
	}
}

func TestCredentialFailureAndUnknownAccount(t *testing.T) {
	srv := newRecorder(t, func(recorded) (int, string) { return 200, usageFixture })
	cred := func(context.Context, string) (Auth, error) { return Auth{}, ErrAuthenticationRequired }
	r := refresherFor(srv, []AccountTarget{{ID: "a"}}, cred)
	if res := r.Refresh(context.Background(), "a"); !res.NeedsReauth || len(srv.all()) != 0 {
		t.Fatalf("%+v", res)
	}
	if res := r.Refresh(context.Background(), "missing"); !errors.Is(res.Err, ErrAccountNotFound) {
		t.Fatalf("%+v", res)
	}
}

func TestStaleResultIsDetectable(t *testing.T) {
	srv := newRecorder(t, func(recorded) (int, string) { return 200, usageFixture })
	r := refresherFor(srv, []AccountTarget{{ID: "a"}}, tokenCred)
	first := r.Refresh(context.Background(), "a")
	r.Generations.Invalidate("a")
	if r.Generations.Valid(first.AccountID, first.Generation) {
		t.Fatal("result of an invalidated account must be dropped")
	}
	second := r.Refresh(context.Background(), "a")
	if !r.Generations.Valid("a", second.Generation) || second.Generation <= first.Generation {
		t.Fatalf("%d %d", first.Generation, second.Generation)
	}
}

func TestRefreshAllBoundsConcurrency(t *testing.T) {
	var running, peak atomic.Int32
	srv := newRecorder(t, func(recorded) (int, string) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		running.Add(-1)
		return 200, usageFixture
	})
	var targets []AccountTarget
	for _, id := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
		targets = append(targets, AccountTarget{ID: id})
	}
	r := refresherFor(srv, targets, tokenCred)
	r.MaxConcurrent = 2
	for _, res := range r.RefreshAll(context.Background(), "") {
		if res.Err != nil || res.Quota == nil {
			t.Fatalf("%+v", res)
		}
	}
	if p := peak.Load(); p > 2 || p < 1 {
		t.Fatalf("peak concurrency %d", p)
	}
}

func TestRefreshAllCancelled(t *testing.T) {
	srv := newRecorder(t, func(recorded) (int, string) { return 200, usageFixture })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := refresherFor(srv, []AccountTarget{{ID: "a"}, {ID: "b"}}, tokenCred)
	r.MaxConcurrent = 1
	for _, res := range r.RefreshAll(ctx, "") {
		if res.Quota != nil && res.Err == nil && len(srv.all()) == 0 {
			t.Fatalf("%+v", res)
		}
	}
}

func TestRefreshAllAccountListError(t *testing.T) {
	r := AccountQuotaRefresher{Accounts: func(context.Context) ([]AccountTarget, error) { return nil, errors.New("boom") }}
	if got := r.RefreshAll(context.Background(), ""); got != nil {
		t.Fatalf("%+v", got)
	}
}

func TestAccountResetCreditsIdentityAndFailureKeepsQuota(t *testing.T) {
	srv := newRecorder(t, func(r recorded) (int, string) {
		if r.Path == "/wham/rate-limit-reset-credits" {
			switch r.Header.Get("ChatGPT-Account-Id") {
			case "a":
				return 200, `{"available_count":0,"credits":[]}`
			case "b":
				return 503, `{}`
			default:
				t.Error("unexpected credit identity")
				return 500, `{}`
			}
		}
		return 200, usageFixture
	})
	r := refresherFor(srv, []AccountTarget{{ID: "a"}, {ID: "b"}, {ID: "key", APIKey: true}}, tokenCred)
	results := r.RefreshAll(context.Background(), "")
	if c := results[0].ResetCredits; c == nil || c.AvailableCount != 0 || results[0].ResetCreditsErr != nil {
		t.Fatalf("zero credits: %+v", results[0])
	}
	if results[1].Quota == nil || results[1].Err != nil || results[1].ResetCredits != nil || results[1].ResetCreditsErr == nil {
		t.Fatalf("quota discarded on credit failure: %+v", results[1])
	}
	if results[2].ResetCredits != nil || results[2].ResetCreditsErr != nil {
		t.Fatalf("key: %+v", results[2])
	}
	for _, req := range srv.all() {
		id := req.Header.Get("ChatGPT-Account-Id")
		if id == "key" || req.Header.Get("Authorization") != "Bearer tok-"+id {
			t.Errorf("credential crossed accounts: %s", id)
		}
	}
}

func TestRefreshAllSkipsOnlyExplicitCurrentCredits(t *testing.T) {
	srv := newRecorder(t, func(r recorded) (int, string) {
		if r.Path == "/wham/rate-limit-reset-credits" {
			return 200, `{"available_count":3,"credits":[]}`
		}
		return 200, usageFixture
	})
	r := refresherFor(srv, []AccountTarget{{ID: "a"}, {ID: "b"}}, tokenCred)
	got := r.RefreshAll(context.Background(), "a")
	if got[0].Quota == nil || got[0].ResetCredits != nil || got[1].ResetCredits == nil {
		t.Fatalf("skip targeted wrong account: %+v", got)
	}
	if single := r.Refresh(context.Background(), "a"); single.ResetCredits == nil {
		t.Fatal("single refresh must fetch current credits")
	}
	if all := r.RefreshAll(context.Background(), ""); all[0].ResetCredits == nil {
		t.Fatal("manual all refresh must fetch current credits")
	}
}

func TestRejectNegativeResetCreditResponse(t *testing.T) {
	if _, err := DecodeResetCredits([]byte(`{"available_count":-1,"credits":[]}`), time.Now()); err == nil {
		t.Fatal("negative reset credits accepted")
	}
}
