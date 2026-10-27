package app

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestClaudeSubscriptionBadgeWithoutUsageInSettingsAndPanel(t *testing.T) {
	l := l10n.New(l10n.English)
	account := claude.ManagedAccount{ID: "browser-account", SubscriptionType: "max", RateLimitTier: "default_claude_max_20x"}
	for _, state := range []string{"no-usage", "usage-failed", "invalid-cache", "unknown-plan"} {
		account.SubscriptionType, account.RateLimitTier, account.LastError, account.CachedQuota = "max", "default_claude_max_20x", "", nil
		if state == "usage-failed" {
			account.LastError = claude.ErrorCodeUsage
		}
		if state == "invalid-cache" {
			account.CachedQuota = json.RawMessage(`{`)
		}
		if state == "unknown-plan" {
			account.SubscriptionType, account.RateLimitTier = "", ""
		}
		quotas := claudeAccountQuotas(l, []claude.ManagedAccount{account})
		for _, panel := range []bool{false, true} {
			t.Run(state+"/"+map[bool]string{false: "settings", true: "panel"}[panel], func(t *testing.T) {
				a, _ := newPanel(t, nil, runway.Snapshot{}, PanelActions{})
				tt := ui.NewTester(func(c *ui.Context) {
					q := quotas[account.ID]
					row := claudeSidebarAccount(l, account)
					if panel {
						a.claudeCardHeader(c, l, row, q, false, false, runway.AccountsView{}, AccountActions{})
					} else {
						accountSettingsIdentityInfo(c, l, accountIdentityView{account: row, provider: store.ProviderClaude, claudeQuota: &q})
					}
				}, 500, 300)
				if tt.HasText("Max 20x") != (state != "unknown-plan") {
					t.Fatal("browser-imported subscription badge is missing before usage is cached")
				}
			})
		}
	}
}
