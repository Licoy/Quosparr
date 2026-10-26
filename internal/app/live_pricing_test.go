package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

const livePricingJSON = `{"openai":{"name":"OpenAI","models":{"online":{"name":"Online","cost":{"input":2,"output":10,"cache_read":0}}}},"other":{"models":{"reference":{"cost":{"input":0,"output":0}}}}}`

func TestPricingDueDefaultsFailureRestoreAndDisabled(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	p := store.DefaultModelPricing()
	s := pricing.Status{}
	if next, due := pricingDue(p, s, now); !due || next != now {
		t.Fatal("missing not immediately due")
	}
	s.LastSuccess = now.Add(-30 * time.Minute)
	if next, due := pricingDue(p, s, now); due || next != now.Add(30*time.Minute) {
		t.Fatal("fresh persisted cache refetched")
	}
	s.LastAttempt = now.Add(-time.Minute)
	if next, due := pricingDue(p, s, now); due || next != s.LastAttempt.Add(time.Hour) {
		t.Fatal("failure retry not delayed")
	}
	p.SyncIntervalSeconds = 900
	s.LastAttempt = now.Add(-20 * time.Minute)
	if _, due := pricingDue(p, s, now); !due {
		t.Fatal("interval change ignored")
	}
	p.AutoSync = false
	if next, due := pricingDue(p, s, now); due || !next.IsZero() {
		t.Fatal("disabled due")
	}
}
func TestPricingWorkerCancelsParentAndShowsRunning(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	prefs := store.DefaultPreferences()
	p := newLivePricing(t.TempDir(), prefs, func(ctx context.Context, _ string) (pricing.Response, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return pricing.Response{}, ctx.Err()
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	p.start(ctx)
	<-started
	if !p.snapshot(prefs).Syncing {
		t.Fatal("background sync not observable")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("parent cancellation did not cancel active request")
	}
	p.close()
}
func TestPricingManualDisabledAndChangeNotifications(t *testing.T) {
	prefs := store.DefaultPreferences()
	prefs.ModelPricing.AutoSync = false
	body := livePricingJSON
	status := 200
	var calls atomic.Int64
	p := newLivePricing(t.TempDir(), prefs, func(context.Context, string) (pricing.Response, error) {
		calls.Add(1)
		return pricing.Response{StatusCode: status, Body: []byte(body), ETag: "etag"}, nil
	}, nil)
	defer p.close()
	changed := []string{}
	p.changed = func(id string) { changed = append(changed, id) }
	if err := p.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != "openai" {
		t.Fatal(changed)
	}
	status = 304
	body = ""
	if err := p.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 {
		t.Fatal("304 repriced")
	}
	status = 200
	body = livePricingJSON
	if err := p.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 {
		t.Fatal("same content repriced")
	}
	a, b := p.snapshot(prefs), p.snapshot(prefs)
	if len(a.Rows) == 0 || &a.Rows[0] != &b.Rows[0] || a.DataVersion != b.DataVersion {
		t.Fatal("snapshot cloned immutable directory")
	}
	if calls.Load() != 3 || !a.NextSync.IsZero() {
		t.Fatal("disabled manual sync failed")
	}
}
func TestPricingCloseCancelsManualWorkAndPreventsNew(t *testing.T) {
	prefs := store.DefaultPreferences()
	prefs.ModelPricing.AutoSync = false
	started := make(chan struct{})
	p := newLivePricing(t.TempDir(), prefs, func(ctx context.Context, _ string) (pricing.Response, error) {
		close(started)
		<-ctx.Done()
		return pricing.Response{}, ctx.Err()
	}, nil)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = p.refresh(context.Background()) }()
	<-started
	p.close()
	wg.Wait()
	if !errors.Is(p.refresh(context.Background()), context.Canceled) {
		t.Fatal("new work after shutdown")
	}
}
func TestPricingCatalogMergedAndLocalized(t *testing.T) {
	catalog, err := pricing.Parse([]byte(livePricingJSON))
	if err != nil {
		t.Fatal(err)
	}
	rows := pricingRows(catalog, l10n.ForPreference("zh-Hans", nil))
	found := map[string]PricingRow{}
	for _, r := range rows {
		found[r.ProviderID+"/"+r.ID] = r
	}
	online := found["openai/online"]
	if !online.Available || online.Builtin || online.Source != "models.dev" || online.SourceURL == "" || online.Input == nil {
		t.Fatalf("remote row %+v", online)
	}
	if _, ok := found["other/reference"]; ok {
		t.Fatal("non-native provider shown")
	}
	builtin := found["openai/gpt-5"]
	if !builtin.Builtin || !builtin.Available || builtin.SourceURL == "" {
		t.Fatalf("builtin row %+v", builtin)
	}
}

// Set QUOSPARR_TEST_PUBLIC_PRICES to an already downloaded public response for a live-schema smoke check.
func TestPublicPricingCatalogSmoke(t *testing.T) {
	path := os.Getenv("QUOSPARR_TEST_PUBLIC_PRICES")
	if path == "" {
		t.Skip("no downloaded public catalog supplied")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	catalog, err := pricing.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	parseDuration := time.Since(start)
	start = time.Now()
	rows := pricingRows(catalog, l10n.ForPreference("zh-Hans", nil))
	t.Logf("public response %d bytes, %d models, %d merged rows, parse=%s, projection=%s", len(data), len(catalog.Models()), len(rows), parseDuration, time.Since(start))
	expected := map[string]bool{}
	for _, group := range [][]pricing.Model{usage.BuiltinPricingModels(), claude.BuiltinPricingModels(), grok.BuiltinPricingModels()} {
		for _, model := range group {
			expected[model.ProviderID+"/"+model.ID] = true
		}
	}
	for _, model := range catalog.Models() {
		switch model.ProviderID {
		case "openai", "anthropic", "xai":
			expected[model.ProviderID+"/"+model.ID] = true
		}
	}
	if len(rows) != len(expected) {
		t.Fatalf("native rows=%d expected=%d", len(rows), len(expected))
	}
	for _, row := range rows {
		key := row.ProviderID + "/" + row.ID
		if !expected[key] {
			t.Fatalf("unexpected provider/model %s", key)
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		t.Fatalf("native rows missing: %v", expected)
	}
}

func TestOpenCostRangeFollowsPriceChangesForEveryProvider(t *testing.T) {
	for _, provider := range []store.Provider{store.ProviderCodex, store.ProviderGrok, store.ProviderClaude} {
		t.Run(string(provider), func(t *testing.T) {
			queue := detailMainQueue(t)
			var version atomic.Value
			version.Store("v1")
			started := make(chan string, 2)
			release := make(chan struct{})
			acts := PanelActions{PricingVersion: func(store.Provider) string { return version.Load().(string) }, QueryCost: func(_ context.Context, _ store.Provider, q CostQuery) (usage.Summary, error) {
				v := version.Load().(string)
				started <- v
				if v == "v1" {
					<-release
				}
				return usage.Summary{PricingVersion: v, Totals: usage.Totals{TotalTokens: 100}}, nil
			}}
			a, tt := newPanel(t, func(p *store.Preferences) {
				p.SelectedProvider = provider
				p.APICostSummaryRange = store.CostRangeToday
			}, panelFixture(), acts)
			a.panelState.detail = store.ModuleAPICost
			tt.Frame()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("initial query missing")
			}
			custom := usage.Today(time.Now(), time.Local)
			a.panelState.cost.query = &CostQuery{Custom: &custom}
			version.Store("v2")
			tt.Frame()
			select {
			case v := <-started:
				if v != "v2" {
					t.Fatal(v)
				}
			case <-time.After(time.Second):
				t.Fatal("price change did not requery open custom range")
			}
			close(release)
			finishDetail(t, queue)
			finishDetail(t, queue)
			if a.panelState.cost.summary == nil || a.panelState.cost.summary.PricingVersion != "v2" {
				t.Fatalf("old price overwrote UI %+v", a.panelState.cost.summary)
			}
		})
	}
}

func TestPricingCatalogAliasRowsKeepExactRatesAndBuiltinRules(t *testing.T) {
	models := []pricing.Model{
		{ProviderID: "openai", ID: "gpt-6-sol-2026-09-26", Rate: pricing.Rate{Input: pricingTestDecimal("99"), Output: pricingTestDecimal("199")}},
		{ProviderID: "anthropic", ID: "us.anthropic.claude-opus-5-5-20261001-v1:0", Rate: pricing.Rate{Input: pricingTestDecimal("98"), Output: pricingTestDecimal("198")}},
		{ProviderID: "xai", ID: "grok-4.7-fast-latest", Rate: pricing.Rate{Input: pricingTestDecimal("97"), Output: pricingTestDecimal("197")}},
	}
	snapshot, err := pricing.NewSnapshot(models)
	if err != nil {
		t.Fatal(err)
	}
	l := l10n.ForPreference("en", nil)
	rows := pricingRows(snapshot, l)
	byKey := map[string]PricingRow{}
	for _, row := range rows {
		byKey[row.ProviderID+"/"+row.ID] = row
	}
	for _, m := range models {
		row := byKey[m.ProviderID+"/"+m.ID]
		if row.Builtin || row.Source != "models.dev" || row.Input.String() != m.Rate.Input.String() {
			t.Fatalf("exact remote rates overwritten %+v", row)
		}
		if !strings.Contains(row.Details, builtinVersion(m.ProviderID)) {
			t.Fatalf("builtin rule version missing %+v", row)
		}
		rules := builtinDetailsFor(m.ProviderID, m.ID)
		if len(rules) < 2 {
			t.Fatalf("alias special rules missing %s", m.ID)
		}
		switch m.ProviderID {
		case "openai":
			if !strings.Contains(row.Details, l.Pricing("ruleLegacyLong")) {
				t.Fatal("Codex dated special rate hidden")
			}
		case "anthropic":
			if !strings.Contains(row.Details, "5m") || !strings.Contains(row.Details, l.PricingRule("ruleGeoUS", "1.1")) {
				t.Fatal("Claude alias TTL/region rules hidden")
			}
		case "xai":
			if !strings.Contains(row.Details, "≥ 200000") {
				t.Fatal("Grok latest context rule hidden")
			}
		}
	}
}
func pricingTestDecimal(s string) *decimal.Decimal { d := decimal.MustParse(s); return &d }
