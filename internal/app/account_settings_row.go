package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

type accountIdentityView struct {
	account          codex.ManagedAccount
	provider         store.Provider
	current, privacy bool
	grokQuota        *runway.GrokAccountQuota
	claudeQuota      *runway.ClaudeAccountQuota
}

func accountSettingsIdentity(c *ui.Context, l l10n.Localizer, v accountIdentityView) {
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
		accountSettingsIdentityInfo(c, l, v)
		accountSettingsStatus(c, l, v)
	})
}

func accountSettingsIdentityInfo(c *ui.Context, l l10n.Localizer, v accountIdentityView) {
	acc := v.account
	name := acc.ResolvedDisplayName()
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, maskAccount(v.privacy, name)).FontSize(13).FontWeight(600).SingleLine().MinWidth(0).Shrink(1).Tooltip(maskAccount(v.privacy, name))
			if v.current {
				runwayTagPadded(c, l.Text(l10n.KeyAccountsCurrent), toneGreen, 1, 5)
			}
			if v.provider == store.ProviderCodex {
				tierBadgePadded(c, codexTierLook(c.Theme(), acc.Tier()), l.Text(accountTierKey(acc.Tier())), 1, 5)
			} else if acc.IsRelay() {
				apiTierBadge(c, l)
			} else if v.grokQuota != nil && v.grokQuota.Plan != "" {
				kind := grok.TierResolve(v.grokQuota.Plan)
				tierBadgePadded(c, grokTierLook(c.Theme(), kind), kind.DisplayName(), 1, 5)
			} else if v.claudeQuota != nil && v.claudeQuota.Plan != "" {
				tierBadgePadded(c, claudeTierLook(c.Theme(), claude.TierResolve(v.claudeQuota.Plan)), v.claudeQuota.Plan, 1, 5)
			}
		})
		if acc.Email != "" && acc.Email != name {
			dimText(c, maskAccount(v.privacy, acc.Email)).SingleLine().Tooltip(maskAccount(v.privacy, acc.Email))
		}
		if v.provider == store.ProviderCodex {
			codexIdentityDetails(c, l, acc, v.privacy)
		}
		if acc.Note != "" {
			dimText(c, accountDiagnosticText(acc.Note, v.privacy)).MaxLines(2)
		}
	})
}

// Settings and panel cards share the same cached quota rendering.
func accountSettingsStatus(c *ui.Context, l l10n.Localizer, v accountIdentityView) {
	acc := v.account
	if acc.RequiresReauth {
		key := l10n.KeyAccountsNeedsReauth
		if v.provider == store.ProviderGrok {
			key = l10n.KeyGrokReauthenticationRequired
		}
		if v.provider == store.ProviderClaude {
			key = l10n.KeyClaudeReauthRequired
		}
		ui.Text(c, l.Text(key)).FontSize(11).TextColor(c.Theme().Danger)
		return
	} else if acc.LastError != "" {
		diag := privateSiteText(v.privacy, acc.LastError, acc.APIBaseURL)
		ui.Text(c, diag).FontSize(11).TextColor(sys(c.Theme(), sysOrange)).MaxLines(2).Tooltip(diag)
		return
	}
	switch {
	case acc.IsRelay():
		apiBalanceBlock(c, l, acc.APIQuota, v.current)
	case v.provider == store.ProviderClaude:
		claudeQuotaBlock(c, l, v.claudeQuota, accountQuotaOptions{current: v.current})
	case v.provider == store.ProviderGrok:
		grokQuotaBlock(c, l, v.grokQuota, accountQuotaOptions{current: v.current})
	default:
		codexQuotaBlock(c, l, acc, accountQuotaOptions{current: v.current})
	}
}

func accountSettingsRow(c *ui.Context, l l10n.Localizer, v accountIdentityView, controls func()) {
	ui.Column(c).Gap(4).Padding(10, 0).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ProviderIcon(c, v.provider, 20)
			accountSettingsIdentityInfo(c, l, v)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				accountQuotaUpdated(c, l, accountSettingsUpdatedAt(v))
				controls()
			})
		})
		ui.Column(c).Gap(8).Padding(0, 0, 0, 30).Children(func() { accountSettingsStatus(c, l, v) })
	})
}

func accountSettingsUpdatedAt(v accountIdentityView) time.Time {
	if v.provider == store.ProviderClaude && v.claudeQuota != nil {
		return v.claudeQuota.UpdatedAt
	}
	if v.provider == store.ProviderGrok && v.grokQuota != nil {
		return v.grokQuota.UpdatedAt
	}
	if v.account.CachedQuota != nil {
		return v.account.CachedQuota.UpdatedAt
	}
	if v.account.APIQuota != nil {
		return v.account.APIQuota.FetchedAt
	}
	if v.account.LastQuotaAt != nil {
		return *v.account.LastQuotaAt
	}
	return time.Time{}
}

func accountUpdatedSummary(l l10n.Localizer, text string, at time.Time) string {
	if at.IsZero() {
		return text
	}
	return text + " · " + l.Text(l10n.KeyLastUpdated) + " " + at.In(time.Local).Format("15:04")
}

func accountDiagnosticText(text string, privacy bool) string {
	if privacy {
		return codex.MaskEmailsIn(text)
	}
	return text
}
