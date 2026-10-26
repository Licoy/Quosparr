package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestAccountResetExpiryAdvancesToNextAvailableCard(t *testing.T) {
	l := l10n.New(l10n.English)
	for _, provider := range store.Providers() {
		for _, settings := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/settings=%t", provider, settings), func(t *testing.T) {
				var acc codex.ManagedAccount
				var cq runway.ClaudeAccountQuota
				var gq runway.GrokAccountQuota
				tt := ui.NewTester(func(c *ui.Context) {
					if acc.ID == "" {
						at := c.Now()
						a, b := at.Add(time.Minute), at.Add(2*time.Minute)
						acc, cq, gq = quotaAccountFixture(t, l, at)
						acc.ResetCredits.AvailableExpiries = []time.Time{a, b, at.Add(-time.Minute)}
						grants := []runway.AccountResetGrant{{Count: 1, EndsAt: a}, {Count: 1, EndsAt: b}, {Count: 0, EndsAt: at.Add(10 * time.Second)}, {Count: 1, StartsAt: at.Add(time.Hour), EndsAt: at.Add(20 * time.Second)}, {Count: 1, EndsAt: at.Add(-time.Minute)}}
						cq.ResetGrants, gq.ResetGrants = grants, grants
					}
					if settings {
						accountSettingsStatus(c, l, accountIdentityView{account: acc, provider: provider, claudeQuota: &cq, grokQuota: &gq})
						return
					}
					view := accountQuotaOptions{showUpdated: true}
					switch provider {
					case store.ProviderCodex:
						codexQuotaBlock(c, l, acc, view)
					case store.ProviderClaude:
						claudeQuotaBlock(c, l, &cq, view)
					case store.ProviderGrok:
						grokQuotaBlock(c, l, &gq, view)
					}
				}, 320, 900)
				first, second := tt.Now().Add(time.Minute), tt.Now().Add(2*time.Minute)
				if !hasExact(tt, accountResetExpiryText(l, first)) {
					t.Fatalf("wrong first card expiry: %q", tt.Texts())
				}
				tt.Advance(time.Minute)
				if !hasExact(tt, accountResetExpiryText(l, second)) || hasExact(tt, accountResetExpiryText(l, first)) {
					t.Fatalf("did not advance card expiry: %q", tt.Texts())
				}
				if provider != store.ProviderCodex && !hasResetStat(tt, l, "1") {
					t.Fatalf("expired card still available: %q", tt.Texts())
				}
				tt.Advance(time.Minute)
				if provider == store.ProviderCodex {
					if !tt.HasText(l.Text(l10n.KeyResetExpiryUnavailable)) {
						t.Fatalf("cached count without remaining expiry must be unknown: %q", tt.Texts())
					}
				} else if tt.HasText(l.Text(l10n.KeyNextExpiry)) || tt.HasText(l.Text(l10n.KeyResetExpiryUnavailable)) {
					t.Fatalf("zero cards have expiry: %q", tt.Texts())
				}
			})
		}
	}
}

func TestAccountResetUnknownExpiryNeverShowsInventedDate(t *testing.T) {
	l := l10n.New(l10n.English)
	count := 2
	tt := ui.NewTester(func(c *ui.Context) {
		accountStats(c, accountQuotaOptions{}, []accountStat{accountResetStat(l, &count, time.Time{}, time.Time{}, "")})
	}, 320, 150)
	if !tt.HasText(l.Text(l10n.KeyResetExpiryUnavailable)) {
		t.Fatal(tt.Texts())
	}
	for _, text := range tt.Texts() {
		if strings.Contains(text, "0001") {
			t.Fatalf("invented expiry: %q", text)
		}
	}
	count = 0
	tt.Frame()
	if tt.HasText(l.Text(l10n.KeyResetExpiryUnavailable)) || tt.HasText(l.Text(l10n.KeyNextExpiry)) {
		t.Fatal("zero cards showed expiry")
	}
}
