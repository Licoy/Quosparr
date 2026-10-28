package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
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
	// quota carries the reset format and the credits switch of the
	// preferences; the zero value keeps the defaults.
	quota accountQuotaOptions
}

// quotaOptions lays a settings row's quota out wide; its update time stays
// in the header.
func (v accountIdentityView) quotaOptions() accountQuotaOptions {
	o := v.quota
	o.current, o.wide = v.current, true
	return o
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
				grokPlanBadgeInfo(c, l, grokPlanInfo{plan: v.grokQuota.Plan, billingSystem: v.grokQuota.PlanBillingSystem, compact: true})
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
	view := v.quotaOptions()
	switch {
	case acc.IsRelay():
		apiBalanceBlock(c, l, acc.APIQuota, view)
	case v.provider == store.ProviderClaude:
		claudeQuotaBlock(c, l, v.claudeQuota, view)
	case v.provider == store.ProviderGrok:
		grokQuotaBlock(c, l, v.grokQuota, view)
	default:
		codexQuotaBlock(c, l, acc, view)
	}
}

// accountRowFade is how a settings account row lights up under the pointer.
var accountRowFade = ui.Animation{Duration: 180 * time.Millisecond, Ease: ui.EaseInOut}

// accountSettingsRow is one account; its wash reaches past the content into
// the box's padding and fades in and out under the pointer.
func accountSettingsRow(c *ui.Context, l l10n.Localizer, v accountIdentityView, controls func()) {
	row := ui.Column(c.Key("account-row-"+string(v.provider)+"-"+v.account.ID)).Gap(4).Padding(10, 8).Margin(0, -8).Radius(8)
	wash := ui.Transparent
	if row.Hovered() {
		wash = pal(c.Theme()).Hover
	}
	row.Background(row.AnimateColor("hover", wash, accountRowFade)).Children(func() {
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
