package app

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func quotaAccountFixture(t *testing.T, l l10n.Localizer, now time.Time) (codex.ManagedAccount, runway.ClaudeAccountQuota, runway.GrokAccountQuota) {
	t.Helper()
	five, week, custom, weeklyUsed, cards, balance := 300, 10080, 120, 61, 2, 12.5
	short, long, other := now.Add(2*time.Hour), now.Add(4*24*time.Hour), now.Add(90*time.Minute)
	acc := codex.ManagedAccount{ID: "one", DisplayName: "Personal", Email: "personal@example.com", CachedQuota: &codex.CachedAccountQuota{
		Plan: "plus", PrimaryUsedPercent: 49, PrimaryWindowMinutes: &five, PrimaryResetsAt: &short,
		SecondaryUsedPercent: &weeklyUsed, SecondaryWindowMinutes: &week, SecondaryResetsAt: &long, UpdatedAt: now,
		AdditionalMeters: []codex.CachedQuotaMeterRow{{ID: "model", Name: "GPT-5.4", WindowMinutes: &custom, UsedPercent: 88, ResetsAt: &other}},
		CreditsBalance:   &balance,
	}, ResetCredits: &codex.CachedAccountResetCredits{AvailableCount: &cards, UpdatedAt: &now, AvailableExpiries: []time.Time{short, long}}}
	raw := json.RawMessage(fmt.Sprintf(`{"five_hour":{"utilization":49,"resets_at":%q},"seven_day":{"utilization":61,"resets_at":%q},"seven_day_sonnet":{"utilization":88,"resets_at":%q},"extra_usage":{"is_enabled":true,"used_credits":1000,"monthly_limit":123400,"currency":"USD"},"cedar_ember":{"eligible":true,"grants":[{"id":"test","resets_total":3,"resets_left":2,"ends_at":%q}]}}`, short.Format(time.RFC3339), long.Format(time.RFC3339), other.Format(time.RFC3339), short.Format(time.RFC3339)))
	cq := claudeAccountQuotas(l, []claude.ManagedAccount{{ID: "one", SubscriptionType: "pro", CachedQuota: raw, LastQuotaAt: &now}})["one"]
	used, limit, prepaid, demand, demandLimit, enabled := int64(7350), int64(15000), int64(2500), int64(100), int64(3000), true
	billing := grok.BillingSnapshot{Source: grok.BillingSourceCurrent, IncludedUsagePercent: 49, UpdatedAt: now, Period: &grok.BillingPeriod{Kind: grok.BillingWeekly, ResetsAt: &long}, IncludedUsedCents: &used, IncludedLimitCents: &limit, PrepaidBalanceCents: &prepaid, OnDemandEnabled: &enabled, OnDemandUsedCents: &demand, OnDemandLimitCents: &demandLimit, ProductUsage: []grok.BillingProductUsage{{Product: "GrokBuild", UsagePercent: 20}}, ResetCredits: &grok.ResetCreditSnapshot{UpdatedAt: now, Tokens: []grok.ResetCreditToken{{TokenID: "one", ValidityEnd: &short}, {TokenID: "two", ValidityEnd: &long}}}}
	data, err := json.Marshal(billing)
	if err != nil {
		t.Fatal(err)
	}
	gq, ok := grokAccountQuota(l, grok.ManagedAccount{CachedQuota: data})
	if !ok {
		t.Fatal("missing grok quota")
	}
	return acc, cq, gq
}

func TestSettingsAndPanelAccountsShowCompleteQuota(t *testing.T) {
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageFrench, store.LanguageRussian, store.LanguageJapanese, store.LanguageKorean, store.LanguageTraditionalChinese} {
		for _, provider := range store.Providers() {
			for _, panel := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/panel=%t", language, provider, panel), func(t *testing.T) {
					l := l10n.ForPreference(string(language), nil)
					var acc codex.ManagedAccount
					var cq runway.ClaudeAccountQuota
					var gq runway.GrokAccountQuota
					a, _ := newPanel(t, nil, runway.Snapshot{}, PanelActions{})
					tt := ui.NewTester(func(c *ui.Context) {
						if acc.ID == "" {
							acc, cq, gq = quotaAccountFixture(t, l, c.Now())
						}
						ui.Column(c).Fill().Padding(12).Children(func() {
							if !panel {
								accountSettingsIdentity(c, l, accountIdentityView{account: acc, provider: provider, claudeQuota: &cq, grokQuota: &gq})
								return
							}
							snap := runway.Snapshot{Accounts: runway.AccountsView{ActiveID: "one"}, Claude: runway.ClaudeState{AccountQuotas: map[string]runway.ClaudeAccountQuota{"one": cq}}, Grok: runway.GrokState{AccountQuotas: map[string]runway.GrokAccountQuota{"one": gq}}}
							switch provider {
							case store.ProviderCodex:
								a.codexAccountCard(c, l, store.DefaultPreferences(), snap.Accounts, acc, AccountActions{})
							case store.ProviderClaude:
								a.claudeAccountCard(c, l, store.DefaultPreferences(), snap, acc, AccountActions{})
							case store.ProviderGrok:
								a.grokAccountCard(c, l, store.DefaultPreferences(), snap, acc, grokCapabilities{}, AccountActions{})
							}
						})
					}, 320, 1100)
					if !hasResetStat(tt, l, "2") || !tt.HasText("51% "+l.Text(l10n.KeyLeft)) {
						t.Fatalf("missing resets or remaining share in %q", tt.Texts())
					}
					if provider == store.ProviderCodex && !hasStat(tt, l.Text(l10n.KeyCreditsBalance), "12.50") {
						t.Fatalf("missing credits balance in %q", tt.Texts())
					}
					resets := []time.Time{tt.Now().Add(2 * time.Hour), tt.Now().Add(4 * 24 * time.Hour), tt.Now().Add(90 * time.Minute)}
					if provider == store.ProviderGrok {
						resets = resets[1:2]
					}
					for _, reset := range resets {
						want := accountResetText(l, reset, tt.Now(), store.ResetTimeRelative)
						if !hasExact(tt, want) {
							t.Fatalf("missing per-window reset %q: %q", want, tt.Texts())
						}
					}
					title := l.Text(l10n.KeyFiveHourUsage)
					if provider == store.ProviderGrok {
						title = gq.Meters[0].Title
					}
					name, _ := tt.Find("Personal")
					row, ok := tt.Find(title)
					if !ok || row.Y <= name.Y+name.H {
						t.Fatalf("quota was not below identity: %v %v", name, row)
					}
					if !panel && row.X != name.X {
						t.Fatalf("quota was not aligned with identity: %v %v", name, row)
					}
					for _, text := range []string{title, "51% " + l.Text(l10n.KeyLeft), l.Text(l10n.KeyResetCredits)} {
						bounds, ok := tt.Find(text)
						if !ok || bounds.X < 0 || bounds.X+bounds.W > 320.5 {
							t.Fatalf("narrow quota text %q exceeds window: %v", text, bounds)
						}
					}
					if provider != store.ProviderGrok {
						for _, want := range []string{"39% " + l.Text(l10n.KeyLeft), "12% " + l.Text(l10n.KeyLeft), l.Text(l10n.KeyWeeklyUsage)} {
							if !tt.HasText(want) {
								t.Fatalf("lost period %q: %q", want, tt.Texts())
							}
						}
						if provider == store.ProviderClaude && !tt.HasText("Sonnet · "+l.Text(l10n.KeyWeeklyUsage)) {
							t.Fatalf("lost model period: %q", tt.Texts())
						}
					} else {
						if !tt.HasText("20% "+l.Text(l10n.KeyUsed)) || tt.HasText("80% "+l.Text(l10n.KeyLeft)) {
							t.Fatalf("product share became a quota: %q", tt.Texts())
						}
						if !tt.HasText("$73.50 / $150.00") || !tt.HasText("$25.00") {
							t.Fatalf("billing details lost: %q", tt.Texts())
						}
					}
					if panel && provider == store.ProviderClaude {
						for _, text := range []string{tx(l, xDesktopBind), tx(l, xDesktopOpen), l.Text(l10n.KeyAccountsDelete)} {
							bounds, ok := tt.Find(text)
							if !ok || bounds.X < 24 || bounds.X+bounds.W > 296.5 {
								t.Fatalf("Claude management action %q exceeds card: %v", text, bounds)
							}
						}
					}
					// Settings hide stale readings on errors; panel cards keep their warnings and cached readings.
					acc.RequiresReauth = true
					tt.Frame()
					reauth := l10n.KeyAccountsNeedsReauth
					if provider == store.ProviderGrok {
						reauth = l10n.KeyGrokReauthenticationRequired
					}
					if provider == store.ProviderClaude {
						reauth = l10n.KeyClaudeReauthRequired
					}
					if !tt.HasText(l.Text(reauth)) {
						t.Fatalf("missing reauth: %q", tt.Texts())
					}
					if !panel && tt.HasText("51% "+l.Text(l10n.KeyLeft)) {
						t.Fatal("stale quota is still shown instead of reauthentication warning")
					}
					acc.RequiresReauth = false
					acc.LastError = "Synthetic refresh error"
					tt.Frame()
					if !tt.HasText(acc.LastError) {
						t.Fatal("refresh error hidden")
					}
					if !panel && tt.HasText("51% "+l.Text(l10n.KeyLeft)) {
						t.Fatal("stale quota is still shown instead of refresh error")
					}
				})
			}
		}
	}
}

func TestClaudeSettingsQuotaWithoutWindowDoesNotShowFullQuota(t *testing.T) {
	l := l10n.New(l10n.English)
	q := runway.ClaudeAccountQuota{ExtraUsage: "10 / 1234 USD"}
	tt := ui.NewTester(func(c *ui.Context) {
		accountSettingsStatus(c, l, accountIdentityView{provider: store.ProviderClaude, claudeQuota: &q})
	}, 320, 300)
	if tt.HasText("100%") || !tt.HasText(l.Text(l10n.KeyClaudeNoQuotaData)) || !tt.HasText(q.ExtraUsage) {
		t.Fatalf("unknown quota mishandled: %q", tt.Texts())
	}
	q.Meters = []runway.Meter{{Title: l.Text(l10n.KeyFiveHourUsage), RemainingPercent: 100}}
	tt.Frame()
	if !tt.HasText("100%") || tt.HasText(l.Text(l10n.KeyClaudeNoQuotaData)) {
		t.Fatalf("valid zero usage missing: %q", tt.Texts())
	}
	// Null and absent percentages must never create a full quota meter.
	raw := json.RawMessage(`{"five_hour":null,"seven_day":{},"seven_day_sonnet":{"utilization":null}}`)
	projected := claudeAccountQuotas(l, []claude.ManagedAccount{{ID: "null", CachedQuota: raw}})
	if len(projected["null"].Meters) != 0 {
		t.Fatalf("unknown window became quota: %+v", projected)
	}
}

func TestAccountResetAvailabilityUsesFrameClock(t *testing.T) {
	l := l10n.New(l10n.English)
	var q runway.ClaudeAccountQuota
	tt := ui.NewTester(func(c *ui.Context) { claudeQuotaBlock(c, l, &q, accountQuotaOptions{}) }, 320, 500)
	at := tt.Now()
	q.ResetGrants = []runway.AccountResetGrant{{Count: 2, EndsAt: at.Add(time.Minute)}, {Count: 1, StartsAt: at.Add(2 * time.Minute), EndsAt: at.Add(3 * time.Minute)}}
	tt.Frame()
	if !hasResetStat(tt, l, "2") {
		t.Fatal(tt.Texts())
	}
	tt.Advance(time.Minute)
	if !hasResetStat(tt, l, "0") {
		t.Fatal(tt.Texts())
	}
	tt.Advance(time.Minute)
	if !hasResetStat(tt, l, "1") {
		t.Fatal(tt.Texts())
	}
	tt.Advance(time.Minute)
	if !hasResetStat(tt, l, "0") {
		t.Fatal(tt.Texts())
	}
}

func TestAccountResetUnknownAndKnownZero(t *testing.T) {
	l := l10n.New(l10n.English)
	acc := codex.ManagedAccount{}
	tt := ui.NewTester(func(c *ui.Context) { codexQuotaBlock(c, l, acc, accountQuotaOptions{}) }, 320, 300)
	if !hasResetStat(tt, l, "—") {
		t.Fatal(tt.Texts())
	}
	zero := 0
	acc.ResetCredits = &codex.CachedAccountResetCredits{AvailableCount: &zero, LastError: "Synthetic reset failure"}
	tt.Frame()
	if !hasResetStat(tt, l, "0") || !tt.HasText("Synthetic reset failure") {
		t.Fatal(tt.Texts())
	}
	acc.ResetCredits.LastError = "reset_credits_unavailable"
	tt.Frame()
	if !hasResetStat(tt, l, "0") || !hasExact(tt, l.Text(l10n.KeyFailed)) {
		t.Fatal(tt.Texts())
	}
	gq := runway.GrokAccountQuota{}
	gt := ui.NewTester(func(c *ui.Context) { grokQuotaBlock(c, l, &gq, accountQuotaOptions{}) }, 320, 300)
	if !hasResetStat(gt, l, "—") || gt.HasText("100%") {
		t.Fatal(gt.Texts())
	}
	gq.ResetCreditsKnown = true
	gt.Frame()
	if !hasResetStat(gt, l, "0") {
		t.Fatal(gt.Texts())
	}
}

func TestGrokSettingsQuotaKeepsCachedTimestamp(t *testing.T) {
	at := time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
	data, err := json.Marshal(grok.BillingSnapshot{UpdatedAt: at, IncludedUsagePercent: 49, Source: grok.BillingSourceCurrent})
	if err != nil {
		t.Fatal(err)
	}
	q, ok := grokAccountQuota(l10n.New(l10n.English), grok.ManagedAccount{CachedQuota: data})
	if !ok || !q.UpdatedAt.Equal(at) {
		t.Fatalf("cached update time was lost: %+v", q)
	}
	if got := accountUpdatedSummary(l10n.New(l10n.English), "51% left", q.UpdatedAt); got != "51% left · Updated "+at.In(time.Local).Format("15:04") {
		t.Fatalf("wrong summary: %s", got)
	}
}

func TestAccountWindowTitlesKeepNameAndActualPeriod(t *testing.T) {
	l := l10n.New(l10n.English)
	custom := 120
	row := codex.CachedQuotaMeterRow{Name: "GPT-5.4", WindowMinutes: &custom}
	want := "GPT-5.4 · " + runway.ShortDuration(120*60, l.Language())
	if got := cachedRowTitle(l, row); got != want {
		t.Fatalf("custom model period lost: %q", got)
	}
	if got := meterTitle(l, runway.Meter{Title: "GPT-5.4", WindowMinutes: custom, Source: runway.SourceModelSpecific}, true); got != want {
		t.Fatalf("main panel period lost: %q", got)
	}
}

func TestClaudeAccountResetProjectionMatchesExistingGrantSemantics(t *testing.T) {
	l := l10n.New(l10n.English)
	raw := json.RawMessage(`{"cedar_ember":{"eligible":true,"grants":[{"id":"large","resets_total":1000,"resets_left":800},{"id":"paused","resets_total":2,"resets_left":2,"paused":true}]}}`)
	snap, err := claude.ParseUsageSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	q := claudeAccountQuotas(l, []claude.ManagedAccount{{ID: "one", CachedQuota: raw}})["one"]
	want := snap.ResetGrants.CodexSnapshot(now, now).AvailableCount
	if got := accountResetCount(q.ResetGrants, now); got != want {
		t.Fatalf("account grants=%d panel grants=%d", got, want)
	}
}

// hasStat reports a figure tile showing value under label.
func hasStat(tt *ui.Tester, label, value string) bool {
	texts := tt.Texts()
	for i, text := range texts {
		if text == label && i+1 < len(texts) && texts[i+1] == value {
			return true
		}
	}
	return false
}

// hasResetStat reports the reset credits tile showing value.
func hasResetStat(tt *ui.Tester, l l10n.Localizer, value string) bool {
	return hasStat(tt, l.Text(l10n.KeyResetCredits), value)
}

func TestAccountQuotaFollowsResetFormatAndCreditsSwitch(t *testing.T) {
	l := l10n.New(l10n.English)
	var acc codex.ManagedAccount
	prefs := store.DefaultPreferences()
	tt := ui.NewTester(func(c *ui.Context) {
		if acc.ID == "" {
			acc, _, _ = quotaAccountFixture(t, l, c.Now())
		}
		codexQuotaBlock(c, l, acc, panelQuotaOptions(prefs, false))
	}, 320, 500)
	short := tt.Now().Add(2 * time.Hour)
	if !hasExact(tt, "Resets in 2h") || !hasStat(tt, l.Text(l10n.KeyCreditsBalance), "12.50") {
		t.Fatalf("relative reset or balance missing: %q", tt.Texts())
	}
	prefs.QuotaResetTimeFormat = store.ResetTimeExact
	prefs.ShowsCreditsBalance = false
	tt.Frame()
	if !hasExact(tt, "Resets "+runway.ResetExactTime(short, tt.Now(), l.Language(), nil)) {
		t.Fatalf("exact reset missing: %q", tt.Texts())
	}
	if tt.HasText(l.Text(l10n.KeyCreditsBalance)) {
		t.Fatalf("hidden balance shown: %q", tt.Texts())
	}
	zero := 0.0
	prefs.ShowsCreditsBalance = true
	acc.CachedQuota.CreditsBalance = &zero
	tt.Frame()
	if tt.HasText(l.Text(l10n.KeyCreditsBalance)) {
		t.Fatalf("zero balance shown: %q", tt.Texts())
	}
}
