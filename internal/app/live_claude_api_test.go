package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

const testAPIKey = "sk-test-app-relay-key"

var apiTestNow = time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)

func floatPtr(v float64) *float64 { return &v }

func apiTestQuota(unit string) apiquota.Quota {
	return apiquota.Quota{
		Status: apiquota.StatusAvailable, Remaining: floatPtr(1.5), Used: floatPtr(0.5), Total: floatPtr(2),
		Unit: unit, FetchedAt: apiTestNow,
	}
}

// offlineRelay answers every request with 404 so no test reaches the network.
type offlineRelay struct{}

func (offlineRelay) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusNotFound, Status: "404 Not Found", Request: req,
		Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")),
	}, nil
}

// apiTestModule is a Claude module over a temp library with one current API
// account whose cached reading is q (none when nil). The relay is a stub, and
// the real home directory is never read.
func apiTestModule(t *testing.T, q *apiquota.Quota) (*claude.Module, claude.ManagedAccount) {
	t.Helper()
	home := t.TempDir()
	config := filepath.Join(home, ".claude")
	store := claude.NewStoreAt(filepath.Join(home, "accounts"), config, home)
	mod := claude.NewModule(store)
	mod.Now = func() time.Time { return apiTestNow }
	mod.APIClient = offlineRelay{}
	acc, err := mod.AddAPIAccount("", claude.APIProfile{BaseURL: "https://relay.example.com", Model: "m"}, testAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := mod.Switch(context.Background(), acc.ID, true); err != nil {
		t.Fatal(err)
	}
	// The switch read the stub relay; replace that reading with the wanted one.
	idx, err := store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	acc, _ = idx.Account(acc.ID)
	acc.APIQuota, acc.LastQuotaAt, acc.LastError = nil, nil, ""
	if q != nil {
		acc = acc.ApplyAPIQuota(*q, apiTestNow)
	}
	if _, err := store.PutAPI(acc, "", apiTestNow); err != nil {
		t.Fatal(err)
	}
	return mod, acc
}

func TestBuildClaudeStateForACurrentAPIAccount(t *testing.T) {
	lc := l10n.New(l10n.English)
	q := apiTestQuota(apiquota.UnitUSD)
	mod, acc := apiTestModule(t, &q)

	// A gateway env in settings.json is how the API account itself works; the
	// gateway notice must not show for it.
	st := buildClaudeState(mod, lc, true)
	if st.Availability != runway.ClaudeReady || st.Plan != "API" || st.Gateway {
		t.Fatalf("state %+v", st)
	}
	if st.Accounts.ActiveID != acc.ID || !st.Accounts.Display.Authenticated || st.Accounts.Display.DisplayName != "relay.example.com ···"+acc.API.KeyHint {
		t.Fatalf("display %+v", st.Accounts.Display)
	}
	if len(st.Meters) != 1 || !st.Meters[0].Balance || st.Meters[0].ValueText != "$1.50" || st.Meters[0].UsedPercent != 25 {
		t.Fatalf("meters %+v", st.Meters)
	}
	if st.API == nil || st.API.BaseURL != "https://relay.example.com" || st.API.Quota == nil || st.API.Quota.Status != apiquota.StatusAvailable {
		t.Fatalf("api view %+v", st.API)
	}
	if st.LastError != "" || st.Detail != nil || st.ExtraUsage != "" {
		t.Fatalf("OAuth-only fields set: %+v", st)
	}
	if !st.UpdatedAt.Equal(apiTestNow) {
		t.Fatalf("updated %v", st.UpdatedAt)
	}
	// The API key never reaches the state.
	raw, err := json.Marshal(st)
	if err == nil && strings.Contains(string(raw), testAPIKey) {
		t.Fatal("state holds the API key")
	}

	// The sidebar lists the API account and its row has the balance.
	if len(st.Accounts.Sidebar) != 1 {
		t.Fatalf("sidebar %+v", st.Accounts.Sidebar)
	}
	row, ok := st.AccountQuotas[acc.ID]
	if !ok || row.Plan != "API" || row.Balance != "$1.50 / $2.00" || row.UsedPercent != 25 || row.Title == "" {
		t.Fatalf("row %+v", row)
	}

	// Widget block: preformatted text always, dollars for USD.
	w := st.Widget
	if w == nil || w.BalanceText == nil || *w.BalanceText != "$1.50" || w.BalanceUSD == nil || w.BalanceUSD.String() != "1.5" {
		t.Fatalf("widget %+v", w)
	}
	if w.Plan == nil || *w.Plan != "API" {
		t.Fatalf("widget plan %+v", w.Plan)
	}
}

func TestBuildClaudeStateAPIAvailability(t *testing.T) {
	lc := l10n.New(l10n.English)

	// No reading yet: loading, no meter.
	mod, _ := apiTestModule(t, nil)
	st := buildClaudeState(mod, lc, false)
	if st.Availability != runway.ClaudeLoading || len(st.Meters) != 0 || st.API == nil || st.API.Quota != nil {
		t.Fatalf("loading state %+v", st)
	}

	// Only an error: failed, with the short text and no key.
	idx, err := mod.Store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	cur, _ := idx.Account(idx.CurrentAccountID)
	cur.LastError = claude.ErrorCodeAPIUnauthorized
	if _, err := mod.Store.PutAPI(cur, "", apiTestNow); err != nil {
		t.Fatal(err)
	}
	st = buildClaudeState(mod, lc, false)
	if st.Availability != runway.ClaudeFailed || st.LastError == "" || st.API.ErrorCode != claude.ErrorCodeAPIUnauthorized {
		t.Fatalf("failed state %+v", st)
	}
	if strings.Contains(st.LastError, testAPIKey) {
		t.Fatal("error text holds the key")
	}

	// A relay without a balance endpoint is ready with no meter.
	unsupported := apiquota.Quota{Status: apiquota.StatusUnsupported, FetchedAt: apiTestNow}
	mod, _ = apiTestModule(t, &unsupported)
	st = buildClaudeState(mod, lc, false)
	if st.Availability != runway.ClaudeReady || len(st.Meters) != 0 || st.Widget == nil || st.Widget.BalanceText != nil || st.Widget.BalanceUSD != nil {
		t.Fatalf("unsupported relay %+v", st)
	}
}

func TestClaudeWidgetBalanceDollarsOnlyForUSD(t *testing.T) {
	cny := apiTestQuota("CNY")
	mod, _ := apiTestModule(t, &cny)
	w := buildClaudeState(mod, l10n.New(l10n.English), false).Widget
	if w == nil || w.BalanceText == nil || *w.BalanceText != "¥1.50" {
		t.Fatalf("balance text %+v", w)
	}
	if w.BalanceUSD != nil {
		t.Fatalf("a non-dollar balance must not be sent as dollars: %v", w.BalanceUSD)
	}

	unlimited := apiquota.Quota{Status: apiquota.StatusAvailable, Unlimited: true, Unit: apiquota.UnitUSD, Remaining: floatPtr(5), FetchedAt: apiTestNow}
	mod, _ = apiTestModule(t, &unlimited)
	w = buildClaudeState(mod, l10n.New(l10n.English), false).Widget
	if w == nil || w.BalanceText == nil || *w.BalanceText != "∞" || w.BalanceUSD != nil {
		t.Fatalf("unlimited %+v", w)
	}
}

func TestRelabelClaudeAPIFollowsTheLanguage(t *testing.T) {
	q := apiTestQuota(apiquota.UnitUSD)
	mod, _ := apiTestModule(t, &q)
	st := buildClaudeState(mod, l10n.New(l10n.English), false)
	en := st.Meters[0].Title
	relabelClaudeAPI(l10n.New(l10n.SimplifiedChinese), &st)
	if len(st.Meters) != 1 || st.Meters[0].Title == "" || st.Meters[0].Title == en || st.Meters[0].ValueText != "$1.50" {
		t.Fatalf("meters after language change %+v (english %q)", st.Meters, en)
	}
	// A state without the API view just clears the localized parts.
	plain := runway.ClaudeState{Meters: []runway.Meter{{Title: "x"}}, LastError: "e"}
	relabelClaudeAPI(l10n.New(l10n.English), &plain)
	if plain.Meters != nil || plain.LastError != "" {
		t.Fatalf("%+v", plain)
	}
}

func TestClaudeAPIAccountQuotaRowWithoutAReading(t *testing.T) {
	lc := l10n.New(l10n.English)
	if _, ok := claudeAPIAccountQuota(lc, claude.ManagedAccount{Kind: claude.KindAPI}); ok {
		t.Fatal("an account that was never read has no row")
	}
	at := apiTestNow
	q, ok := claudeAPIAccountQuota(lc, claude.ManagedAccount{Kind: claude.KindAPI, LastQuotaAt: &at})
	if !ok || q.Plan != "API" || q.Balance != "" || !q.UpdatedAt.Equal(at) {
		t.Fatalf("row %+v", q)
	}
	un := apiquota.Quota{Status: apiquota.StatusUnsupported, FetchedAt: apiTestNow}
	q, ok = claudeAPIAccountQuota(lc, claude.ManagedAccount{Kind: claude.KindAPI, APIQuota: &un})
	if !ok || q.Balance != "" || q.Title != "" {
		t.Fatalf("unsupported row %+v", q)
	}
}

func TestLiveAppClaudeAPIMethodsWithoutAModule(t *testing.T) {
	rt := &liveApp{}
	ctx := context.Background()
	if _, err := rt.addClaudeAPIAccount(ctx, ClaudeAPIInput{BaseURL: "https://relay.example.com", APIKey: testAPIKey}); !errors.Is(err, errClaudeUnavailable) {
		t.Fatal(err)
	}
	if err := rt.updateClaudeAPIAccount(ctx, "x", ClaudeAPIInput{}); !errors.Is(err, errClaudeUnavailable) {
		t.Fatal(err)
	}
	if err := rt.switchClaudeAccount(ctx, "x"); !errors.Is(err, errClaudeUnavailable) {
		t.Fatal(err)
	}
	if _, err := rt.claudeAPIModels(ctx, "", "https://relay.example.com", testAPIKey); !errors.Is(err, errClaudeUnavailable) {
		t.Fatal(err)
	}
}

func TestClaudeAPIInputProfileCarriesTheMappings(t *testing.T) {
	p := ClaudeAPIInput{
		Alias: "a", BaseURL: "https://relay.example.com", APIKey: testAPIKey, Model: "m",
		OpusModel: "o", SonnetModel: "s", HaikuModel: "h", FableModel: "f", Context1M: true,
	}.profile()
	want := claude.APIProfile{BaseURL: "https://relay.example.com", Model: "m", OpusModel: "o", SonnetModel: "s", HaikuModel: "h", FableModel: "f", Context1M: true}
	if p != want {
		t.Fatalf("%+v", p)
	}
}

func TestClaudeAPIEditorRestoresContextChoice(t *testing.T) {
	mod, acc := apiTestModule(t, nil)
	p := *acc.API
	p.Context1M = true
	if _, err := mod.EditAPIAccount(acc.ID, claude.APIEdit{Profile: p, Apply: true}); err != nil {
		t.Fatal(err)
	}
	f := &accountForm{}
	f.openClaudeAPIEditor(&liveApp{claudeMod: mod}, acc.ID)
	if !f.api.context1M || !f.api.initialContext1M || f.api.model != "m" || apiNeedsApplyChoice(f) {
		t.Fatal("editor did not restore the original model and context choice")
	}
	f.api.context1M = false
	if !apiNeedsApplyChoice(f) {
		t.Fatal("disabling the current context choice must ask before applying")
	}
}
