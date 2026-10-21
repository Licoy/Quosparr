package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestSettingsQuotaUsesMainBranchSummaries(t *testing.T) {
	l := l10n.New(l10n.English)
	now := time.Now()
	reset := now.Add(6 * time.Hour)
	acc := codex.ManagedAccount{ID: "one", DisplayName: "Personal", Email: "personal@example.com", CachedQuota: &codex.CachedAccountQuota{PrimaryUsedPercent: 49, PrimaryResetsAt: &reset, UpdatedAt: now}}
	q := runway.GrokAccountQuota{Plan: "SuperGrok", UsedPercent: 49, ResetsAt: reset, IncludedUSD: "$73.50 / $150.00", UpdatedAt: now}
	cq := runway.ClaudeAccountQuota{Plan: "Max 20x", Title: l.Text(l10n.KeyFiveHourUsage), UsedPercent: 49, ResetsAt: reset, ExtraUsage: "10 / 1234 USD", UpdatedAt: now}
	for _, provider := range store.CLIProviders() {
		v := accountIdentityView{account: acc, current: true, provider: provider, grokQuota: &q, claudeQuota: &cq}
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Padding(16).Children(func() { accountSettingsIdentity(c, l, v) })
		}, 520, 220)
		want := accountUpdatedSummary(l, "51%", now)
		if provider == store.ProviderGrok {
			want = "$73.50 / $150.00 · 51% left"
		}
		if provider == store.ProviderClaude {
			want = "10 / 1234 USD · 51% left"
		}
		if !tt.HasText(want) || !tt.HasText(l.Text(l10n.KeyNextResetIn)) {
			t.Fatalf("missing %s quota summary/countdown: %q", provider, tt.Texts())
		}
		name, _ := tt.Find("Personal")
		summary, _ := tt.Find(want)
		if summary.X != name.X || summary.Y <= name.Y+name.H {
			t.Fatalf("quota was not below identity: %v %v", name, summary)
		}
		v.account.RequiresReauth = true
		tt.Frame()
		if tt.HasText(want) {
			t.Fatal("stale quota is still shown instead of reauthentication warning")
		}
	}
}

func TestClaudeSettingsQuotaWithoutWindowDoesNotShowFullQuota(t *testing.T) {
	l := l10n.New(l10n.English)
	for _, extra := range []string{"", "10 / 1234 USD"} {
		t.Run(extra, func(t *testing.T) {
			v := accountIdentityView{
				provider:    store.ProviderClaude,
				claudeQuota: &runway.ClaudeAccountQuota{Plan: "Pro", ExtraUsage: extra, UpdatedAt: time.Now()},
			}
			tt := ui.NewTester(func(c *ui.Context) { accountSettingsStatus(c, l, v) }, 520, 220)
			if tt.HasText("100%") || !tt.HasText(l.Text(l10n.KeyClaudeNoQuotaData)) {
				t.Fatalf("missing quota shown as full: %q", tt.Texts())
			}
			if extra != "" && !tt.HasText(extra) {
				t.Fatalf("extra usage was hidden: %q", tt.Texts())
			}
			v.claudeQuota.Title = l.Text(l10n.KeyFiveHourUsage)
			tt.Frame()
			if !tt.HasText("100%") || tt.HasText(l.Text(l10n.KeyClaudeNoQuotaData)) {
				t.Fatalf("valid zero usage must show full quota: %q", tt.Texts())
			}
		})
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
