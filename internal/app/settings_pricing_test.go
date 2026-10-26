package app

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func TestPricingFiltersPagingAndMissing(t *testing.T) {
	zero, paid := decimal.Zero, decimal.MustParse("1.25")
	rows := []PricingRow{{ProviderID: "openai", ID: "zero", Name: "Free", Input: &zero, Output: &zero, Available: true}, {ProviderID: "openai", ID: "missing", Name: "Missing", Builtin: true}, {ProviderID: "openai", ID: "paid", Name: "Paid", Input: &paid, Output: &paid}}
	if pricingValue(nil) != "—" || pricingValue(&zero) != "$0" {
		t.Fatal("missing and zero conflated")
	}
	s := settingsPricing{filter: "incomplete"}
	if got := s.filtered(rows); len(got) != 1 || got[0].ID != "missing" {
		t.Fatalf("incomplete %v", got)
	}
	s = settingsPricing{search: "ZEro"}
	if len(s.filtered(rows)) != 1 {
		t.Fatal("search id")
	}
	s = settingsPricing{sort: "input"}
	if got := s.filtered(rows); got[0].ID != "zero" || got[2].ID != "missing" {
		t.Fatalf("sort %v", got)
	}
	large := make([]PricingRow, 9001)
	for i := range large {
		large[i] = PricingRow{ProviderID: "openai", ID: fmt.Sprint(i), Name: fmt.Sprintf("Model %05d", i)}
	}
	s = settingsPricing{page: 90}
	if len(s.pageRows(s.filtered(large))) != 1 {
		t.Fatal("last page")
	}
	s.page = 999
	if len(s.pageRows(large)) != 1 || s.page != 90 {
		t.Fatal("page clamping")
	}
}
func TestPricingPageAndCustomInterval(t *testing.T) {
	e := newEnv(t, nil)
	u := newSettingsUI(e.s, SettingsOptions{})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	tt.Frame()
	if !hasExact(tt, u.L().Pricing("empty")) || hasExact(tt, u.L().Pricing("auto")) {
		t.Fatalf("empty page %q", tt.Texts())
	}
	if err := tt.Click(u.L().Pricing("syncSettings")); err != nil {
		t.Fatal(err)
	}
	if !hasExact(tt, u.L().Pricing("auto")) {
		t.Fatal("auto missing in modal")
	}
	for _, raw := range []string{"14", "10081", "oops", "15.5"} {
		if u.savePricingInterval(raw) || u.pricing.validation == "" {
			t.Fatal("invalid frequency accepted")
		}
	}
	for _, raw := range []string{"15", "10080"} {
		if !u.savePricingInterval(raw) {
			t.Fatal("valid frequency rejected")
		}
	}
	e.settle()
	if e.s.Prefs().ModelPricing.SyncIntervalSeconds != store.MaxModelPricingInterval {
		t.Fatal("frequency not applied")
	}
}

func TestPricingCustomIntervalSaveFailureRollsBack(t *testing.T) {
	e := newEnv(t, nil)
	u := newSettingsUI(e.s, SettingsOptions{})
	e.mem.fail.Store(true)
	if !u.savePricingInterval("30") {
		t.Fatal("update rejected before save")
	}
	e.settle()
	if e.s.Prefs().ModelPricing.SyncIntervalSeconds != 3600 || e.s.SaveError() == nil {
		t.Fatal("save did not roll back")
	}
}
func TestPricingOfflineBuiltinAndSyncError(t *testing.T) {
	e := newEnv(t, nil)
	zero := decimal.Zero
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions {
		return PricingActions{Snapshot: func() PricingSnapshot {
			return PricingSnapshot{Error: "offline", Rows: []PricingRow{{ProviderID: "openai", ProviderName: "OpenAI", ID: "free", Name: "Free Model", Input: &zero, Output: &zero, Builtin: true, Source: "builtin"}}}
		}}
	}})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	tt.Frame()
	if !hasExact(tt, "Free Model") || !hasExact(tt, u.L().Pricing("error")+": offline") {
		t.Fatalf("offline builtin not visible %q", tt.Texts())
	}
}

func TestPricingRefreshIsAsyncAndDropsCanceledCompletion(t *testing.T) {
	e := newEnv(t, nil)
	queue := detailMainQueue(t)
	u := newSettingsUI(e.s, SettingsOptions{})
	release := make(chan struct{})
	var calls atomic.Int32
	a := PricingActions{Refresh: func(ctx context.Context) error { calls.Add(1); <-release; return errors.New("late error") }}
	u.refreshPricing(a)
	u.refreshPricing(a)
	if !u.pricing.task.Running {
		t.Fatal("not running")
	}
	u.cancelPricingTasks()
	close(release)
	finishDetail(t, queue)
	if calls.Load() != 1 || u.pricing.task.Err != nil {
		t.Fatalf("duplicate or stale completion: %d %v", calls.Load(), u.pricing.task.Err)
	}
}
func TestPricingCatalogCacheTracksDataVersion(t *testing.T) {
	rows := []PricingRow{{ProviderID: "openai", ID: "one", Name: "Alpha"}, {ProviderID: "openai", ID: "two", Name: "Beta"}}
	s := settingsPricing{}
	got := s.cachedList(PricingSnapshot{Rows: rows, DataVersion: "one"})
	again := s.cachedList(PricingSnapshot{Rows: rows, DataVersion: "one"})
	if &got[0] != &again[0] {
		t.Fatal("unmodified frame rebuilt catalog")
	}
	s.search = "beta"
	if len(s.cachedList(PricingSnapshot{Rows: rows, DataVersion: "one"})) != 1 {
		t.Fatal("search cache not invalidated")
	}
	s.search = ""
	changed := []PricingRow{{ProviderID: "openai", ID: "one", Name: "Zulu"}, {ProviderID: "openai", ID: "two", Name: "Beta"}}
	if s.cachedList(PricingSnapshot{Rows: changed, DataVersion: "two"})[0].ID != "two" {
		t.Fatal("renamed model not sorted")
	}
}

func TestPricingSourceLinksAreHTTPOnly(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "file:///tmp/prices", "https://user:password@example.com/prices", "https:///prices"} {
		if pricingSafeSourceURL(raw) != "" {
			t.Fatalf("accepted unsafe URL %q", raw)
		}
	}
	if pricingSafeSourceURL("https://models.dev/") != "https://models.dev/" {
		t.Fatal("public source unavailable")
	}
}
func TestPricingBuiltinWaitingIsVisible(t *testing.T) {
	e := newEnv(t, nil)
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions {
		return PricingActions{Snapshot: func() PricingSnapshot {
			return PricingSnapshot{Rows: []PricingRow{{ProviderID: "openai", Name: "Builtin", Builtin: true}}}
		}}
	}})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	tt.Frame()
	if !hasExact(tt, u.L().Pricing("usingBuiltin")) {
		t.Fatalf("waiting state missing: %q", tt.Texts())
	}
}

func TestPricingManualFailureSupersededBySnapshotResult(t *testing.T) {
	for _, nextError := range []string{"", "new automatic error"} {
		t.Run(nextError, func(t *testing.T) {
			e := newEnv(t, nil)
			queue := detailMainQueue(t)
			snap := PricingSnapshot{}
			release := make(chan struct{})
			actions := PricingActions{Snapshot: func() PricingSnapshot { return snap }, Refresh: func(context.Context) error {
				<-release
				return errors.New("manual action error")
			}}
			u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions { return actions }})
			u.tab = tabModelPricing
			tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
			snap.LastAttempt = tt.Now()
			snap.LastSuccess = tt.Now()
			tt.Frame()
			// Another UI callback may run before the refresh completion.
			queue <- func() { close(release) }
			u.refreshPricing(actions)
			for u.pricing.task.Running {
				finishDetail(t, queue)
			}
			tt.Frame()
			old := u.L().Pricing("error") + ": manual action error"
			if !hasExact(tt, old) {
				t.Fatalf("action-only failure hidden: %q", tt.Texts())
			}
			tt.Frame()
			if !hasExact(tt, old) {
				t.Fatal("unchanged snapshot cleared action-only failure")
			}
			tt.Advance(time.Minute)
			snap.LastAttempt = tt.Now()
			snap.Error = nextError
			if nextError == "" {
				snap.LastSuccess = tt.Now()
			}
			tt.Frame()
			if hasExact(tt, old) || u.pricing.task.Err != nil {
				t.Fatal("old manual error survived a newer result")
			}
			if nextError != "" && !hasExact(tt, u.L().Pricing("error")+": "+nextError) {
				t.Fatal("new snapshot error hidden")
			}
		})
	}
}
func TestPricingSearchReturnsToFirstPage(t *testing.T) {
	e := newEnv(t, nil)
	rows := make([]PricingRow, 9000)
	for i := range rows {
		name := fmt.Sprintf("Other %04d", i)
		if i < 1000 {
			name = fmt.Sprintf("Match %04d", i)
		}
		rows[i] = PricingRow{ProviderID: "openai", ID: fmt.Sprint(i), Name: name}
	}
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions {
		return PricingActions{Snapshot: func() PricingSnapshot { return PricingSnapshot{Rows: rows, DataVersion: "large"} }}
	}})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	tt.Frame()
	u.pricing.page = 5
	tt.Frame()
	if err := tt.Click(u.L().Pricing("search")); err != nil {
		t.Fatal(err)
	}
	tt.Type("Match")
	tt.Frame()
	if u.pricing.page != 0 || len(u.pricing.cached) != 1000 || !hasExact(tt, "Match 0000") {
		t.Fatalf("search kept page %d, matches %d", u.pricing.page, len(u.pricing.cached))
	}
}
func TestPricingIntervalMenuClearsOldValidation(t *testing.T) {
	e := newEnv(t, nil)
	u := newSettingsUI(e.s, SettingsOptions{})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	tt.Frame()
	s := u.pricingState()
	s.syncSettings = true
	tt.Frame()
	for _, choice := range []string{pricingIntervalLabel(u, 1800), u.L().Pricing("custom")} {
		s.customOpen = true
		s.validation = u.L().Pricing("invalid")
		tt.Frame()
		if err := tt.Click(u.L().Pricing("interval")); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(choice); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if s.validation != "" || hasExact(tt, u.L().Pricing("invalid")) {
			t.Fatalf("validation retained after %q", choice)
		}
	}
	e.settle()
}

func TestPricingDelayedManualFailureDoesNotHideLaterSuccess(t *testing.T) {
	e := newEnv(t, nil)
	queue := detailMainQueue(t)
	snap := PricingSnapshot{}
	actions := PricingActions{Snapshot: func() PricingSnapshot { return snap }, Refresh: func(context.Context) error { return errors.New("old failure") }}
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions { return actions }})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	snap.LastAttempt = tt.Now()
	tt.Frame()
	u.refreshPricing(actions)
	// Receiving the post proves the worker has captured the failed attempt.
	complete := <-queue
	tt.Advance(time.Minute)
	snap.LastAttempt = tt.Now()
	snap.LastSuccess = tt.Now()
	tt.Frame()
	complete()
	tt.Frame()
	if u.pricing.task.Err != nil || hasExact(tt, u.L().Pricing("error")+": old failure") {
		t.Fatal("delayed manual error overrode later automatic success")
	}
}
func TestPricingStateCountsFollowSearch(t *testing.T) {
	zero := decimal.Zero
	rows := []PricingRow{{ProviderID: "openai", ID: "a", Name: "Alpha", Input: &zero, Output: &zero, Available: true}, {ProviderID: "openai", ID: "b", Name: "Beta", Builtin: true}, {ProviderID: "anthropic", ID: "c", Name: "Alpha C"}}
	s := settingsPricing{provider: store.ProviderCodex, filter: "builtin"}
	if got := s.filtered(rows); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("builtin %v", got)
	}
	if s.counts != [len(pricingFilters)]int{2, 1, 1, 1, 1} || s.providerCounts["openai"] != 2 || s.providerCounts["anthropic"] != 1 {
		t.Fatalf("counts %v %v", s.counts, s.providerCounts)
	}
	s.search = "alpha"
	s.filtered(rows)
	if s.counts != [len(pricingFilters)]int{1, 1, 0, 0, 0} || s.providerCounts["openai"] != 2 {
		t.Fatalf("search counts %v %v", s.counts, s.providerCounts)
	}
}
func TestPricingStateChipFiltersList(t *testing.T) {
	e := newEnv(t, nil)
	paid := decimal.MustParse("1.25")
	rows := []PricingRow{{ProviderID: "openai", ID: "paid", Name: "Paid Model", Input: &paid, Output: &paid, Available: true}, {ProviderID: "openai", ID: "missing", Name: "Missing Model"}}
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions {
		return PricingActions{Snapshot: func() PricingSnapshot { return PricingSnapshot{Rows: rows} }}
	}})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsMinWidth, settingsMinHeight)
	tt.Frame()
	if !hasExact(tt, "Paid Model") || !hasExact(tt, "Missing Model") || !hasExact(tt, u.L().Pricing("incomplete")) {
		t.Fatalf("list %q", tt.Texts())
	}
	if err := tt.Click(u.L().Pricing("available")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if u.pricing.filter != "available" || !hasExact(tt, "Paid Model") || hasExact(tt, "Missing Model") {
		t.Fatalf("filter %q: %q", u.pricing.filter, tt.Texts())
	}
}
