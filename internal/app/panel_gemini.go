package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

const geminiInstallGuideURL = "https://antigravity.google/download"

// panelGeminiQuota shows the Gemini 5-hour window first, then the other
// windows; the info button lists the models, credits and resets.
func panelGeminiQuota(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState, rs resetSwitch) {
	g := snap.Gemini
	refreshing := sectionRefreshing(snap, runway.SectionQuota)
	spec := sectionSpec{title: l.Text(l10n.KeyProviderGemini), refreshing: refreshing, refresh: sectionRefresh(acts, runway.SectionQuota)}
	if len(g.Details) > 0 {
		spec.info = &sectionInfo{id: "gemini-quota", help: l.Text(l10n.KeyGeminiQuotaDetails), body: geminiDetailsBody(c, l, st, g.Details)}
	}
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, spec)
		if len(g.Meters) == 0 || g.Availability == runway.GeminiValidationRequired || g.Availability == runway.GeminiSubscriptionRequired {
			if refreshing && g.Availability == runway.GeminiLoading {
				sectionEmpty(c, l, true)
			} else {
				geminiEmptyState(c, l, g)
			}
			if len(g.Meters) == 0 {
				return
			}
		}
		ui.Column(c).Gap(12).Children(func() {
			now := time.Now()
			for i, m := range g.Meters {
				grokMeterRow(c, l, m, i == 0, now, rs)
			}
		})
	})
}

// geminiEmptyState explains why there is no reading and offers the page
// that fixes it.
func geminiEmptyState(c *ui.Context, l l10n.Localizer, g runway.GeminiState) {
	t := c.Theme()
	title, detail, icon, tint := geminiEmptyCopy(l, g)
	cardColumn(c, surfaceSunkenKind, radiusCard).Gap(7).Padding(12).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).Size(13, 13).TextColor(tint(t))
			ui.Text(c, title).FontSize(fsCallout).FontWeight(600).TextColor(tint(t))
		})
		if detail != "" {
			dimText(c, detail)
		}
		switch {
		case g.Availability == runway.GeminiNotInstalled:
			if linkButton(c, l.Text(l10n.KeyGeminiOpenInstallGuide)).Clicked() {
				c.OpenURL(geminiInstallGuideURL)
			}
		case g.ValidationURL != "" && (g.Availability == runway.GeminiValidationRequired || g.Availability == runway.GeminiSubscriptionRequired):
			if linkButton(c, l.Text(l10n.KeyGeminiOpenValidation)).Clicked() {
				c.OpenURL(g.ValidationURL)
			}
		}
	})
}

func geminiEmptyCopy(l l10n.Localizer, g runway.GeminiState) (title, detail string, icon *ui.SVG, tint func(*ui.Theme) ui.Color) {
	secondary := func(t *ui.Theme) ui.Color { return inkSecondary(t) }
	warn := func(t *ui.Theme) ui.Color { return sys(t, sysOrange) }
	switch g.Availability {
	case runway.GeminiLoading:
		return l.Text(l10n.KeyGeminiRefreshing), "", icRefresh, secondary
	case runway.GeminiReady:
		return l.Text(l10n.KeyGeminiNoQuotaData), "", icUsers, secondary
	case runway.GeminiNotInstalled:
		return l.Text(l10n.KeyGeminiNotInstalled), l.Text(l10n.KeyGeminiInstallHint), icTerminal, secondary
	case runway.GeminiNotLoggedIn:
		return l.Text(l10n.KeyGeminiNotLoggedIn), "", icUsers, secondary
	case runway.GeminiReauthRequired:
		return l.Text(l10n.KeyGeminiReauthRequired), "", icKey, warn
	case runway.GeminiValidationRequired:
		return l.Text(l10n.KeyGeminiValidationRequired), l.Text(l10n.KeyGeminiValidationHint), icKey, warn
	case runway.GeminiSubscriptionRequired:
		return l.Text(l10n.KeyGeminiSubscriptionRequired), l.Text(l10n.KeyGeminiValidationHint), icKey, warn
	case runway.GeminiRateLimited:
		return l.Text(l10n.KeyGeminiRateLimited), "", icAlert, warn
	case runway.GeminiQuotaParseFailed:
		return l.Text(l10n.KeyGeminiQuotaParseFailed), "", icAlert, warn
	}
	return l.Text(l10n.KeyGeminiRefreshFailed), "", icAlert, warn
}

// geminiDetailsBody is the quota details popover: the plan, every window
// and model with its reset, and the credits.
func geminiDetailsBody(c *ui.Context, l l10n.Localizer, st *panelState, lines []runway.DetailLine) func() {
	t := c.Theme()
	return func() {
		ui.Text(c, l.Text(l10n.KeyGeminiQuotaDetails)).FontSize(fsTitle3).FontWeight(600)
		cardColumn(c, surfaceSunkenKind, radiusCard).Children(func() {
			for i, line := range lines {
				tableRowRule(c, i == 0)
				ui.Row(c).Gap(10).Padding(7, 10).Children(func() {
					ui.Text(c, line.Title).FontSize(fsCallout).TextColor(inkSecondary(t)).Grow(1).Basis(0)
					ui.Text(c, line.Value).FontSize(fsCallout).FontFeatures("tnum").TextAlign(ui.End)
				})
			}
		})
		ui.Row(c).Children(func() {
			ui.Spacer(c).Grow(1)
			if PillButton(c, nil, l.Text(l10n.KeyOk), false).Clicked() {
				st.info = ""
			}
		})
	}
}

func geminiIdentity(l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot) string {
	name := snap.Gemini.Accounts.Display.DisplayName
	if name == "" {
		return l.Text(l10n.KeyGeminiNotLoggedIn)
	}
	if prefs.AccountPrivacyModeEnabled {
		return codex.MaskAccountText(name)
	}
	return name
}

func geminiIdentityRow(c *ui.Context, a *App, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot) {
	name := geminiIdentity(l, prefs, snap)
	row := identityPill(c, l.Text(l10n.KeyAccounts))
	row.Children(func() {
		geminiPlanBadge(c, l, snap.Gemini.TierKind, snap.Gemini.Plan)
		ui.Text(c, name).FontSize(fsCallout).FontWeight(600).SingleLine().Ellipsis("…").MaxWidth(220)
		identityChevron(c)
	})
	if row.Clicked() {
		a.panelState.detail = detailAccounts
	}
}

// geminiPlanBadge is the tier plate, labelled with the plan name the
// service gave when there is one.
func geminiPlanBadge(c *ui.Context, l l10n.Localizer, kind, plan string) {
	geminiPlanBadgePadded(c, l, kind, plan, 3, 7)
}

func geminiPlanBadgePadded(c *ui.Context, l l10n.Localizer, kind, plan string, vertical, horizontal float32) {
	label := plan
	if label == "" {
		label = geminiTierLabel(l, kind)
	}
	tierBadgePadded(c, geminiTierLook(c.Theme(), gemini.TierKind(kind)), label, vertical, horizontal)
}
