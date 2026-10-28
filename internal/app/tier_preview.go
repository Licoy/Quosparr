package app

import (
	"errors"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
)

// TierPreviewEnv opens the dev-only window that shows every subscription
// badge in the light and the dark look.
const TierPreviewEnv = "QUOSPARR_DEV_TIER_BADGES"

// TierPreviewAllowed limits the preview to development packages and runs
// with a scratch preferences domain.
func TierPreviewAllowed(dev bool) bool { return dev || developmentPackage }

var (
	previewCodexTiers = []codex.Tier{
		codex.TierFree, codex.TierPlus, codex.TierPro100, codex.TierPro200, codex.TierPro500,
		codex.TierBusiness, codex.TierBusinessPremium, codex.TierTeam, codex.TierEnterprise,
		codex.TierEdu, codex.TierAPI, codex.TierUnknown,
	}
	previewGrokTiers = []grok.TierKind{
		grok.TierFree, grok.TierSuperGrokLite, grok.TierSuperGrok, grok.TierSuperGrokPlus,
		grok.TierSuperGrokHeavy, grok.TierXBasic, grok.TierXPremium, grok.TierXPremiumPlus,
		grok.TierXSubscriptionPlus, grok.TierXSubscriptionPremium, grok.TierXSubscriptionSuper, grok.TierXSubscriptionUltra,
		grok.TierAPIKey, grok.TierUnknown,
	}
	previewClaudeTiers = []claude.TierKind{
		claude.TierPro, claude.TierMax, claude.TierMax5x, claude.TierMax20x,
		claude.TierTeam, claude.TierEnterprise,
	}
)

// OpenTierPreview shows the badge preview window, or focuses it.
func (a *App) OpenTierPreview() {
	if a.tierWin != nil {
		a.tierWin.Show()
		a.tierWin.Focus()
		return
	}
	a.tierWin = mygo.NewWindow(mygo.WindowOptions{
		Title:  "Quosparr Tier Badges",
		Width:  760,
		Height: 520,
		Content: ui.View(func(c *ui.Context) {
			tierPreview(c, a.L())
		}),
	})
	a.tierWin.OnClosed(func() { a.tierWin = nil })
}

// CaptureTierPreview returns a PNG of the preview window, for dev runs.
func (a *App) CaptureTierPreview() ([]byte, error) {
	var w *mygo.Window
	mygo.RunOnMain(func() { w = a.tierWin })
	if w == nil {
		return nil, errors.New("tier preview window is not open")
	}
	return w.CapturePage()
}

func tierPreview(c *ui.Context, l l10n.Localizer) {
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		for _, dark := range []bool{false, true} {
			t := *c.Theme()
			t.Dark = dark
			tierPreviewPane(c.Key(pick(!dark, "light", "dark")), l, &t)
		}
	})
}

// tierPreviewPane draws every badge with the looks of theme t on a fixed
// light or dark ground, whatever the system appearance is.
func tierPreviewPane(c *ui.Context, l l10n.Localizer, t *ui.Theme) {
	light := !t.Dark
	ground := pick(light, ui.RGBA(246, 246, 248, 1), ui.RGBA(30, 30, 34, 1))
	ink := pick(light, ui.RGBA(0, 0, 0, 0.85), ui.RGBA(255, 255, 255, 0.88))
	ui.Scroll(c).Grow(1).Basis(0).Background(ground).Padding(20).Gap(16).Children(func() {
		ui.Text(c, pick(light, "Light", "Dark")).FontSize(15).FontWeight(600).TextColor(ink)
		tierPreviewGroup(c, l.Text(l10n.KeyProviderCodex), ink, func() {
			for _, tier := range previewCodexTiers {
				tierBadge(c, codexTierLook(t, tier), l.Text(accountTierKey(tier)))
			}
		})
		tierPreviewGroup(c, l.Text(l10n.KeyProviderGrok), ink, func() {
			for _, tier := range previewGrokTiers {
				tierBadge(c, grokTierLook(t, tier), tier.DisplayName())
			}
		})
		tierPreviewGroup(c, l.Text(l10n.KeyProviderClaude), ink, func() {
			for _, tier := range previewClaudeTiers {
				tierBadge(c, claudeTierLook(t, tier), tier.DisplayName())
			}
			tierBadge(c, plainLook(t), l.Text(l10n.KeyProviderClaude))
		})
	})
}

func tierPreviewGroup(c *ui.Context, title string, ink ui.Color, badges func()) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(ink.Alpha(0.6))
		ui.Row(c).Wrap().Gap(6).AlignItems(ui.Center).Children(badges)
	})
}
