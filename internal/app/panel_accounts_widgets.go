package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

// ghostButton is ToolbarGhostButton: icon and label, transparent at rest
// and a neutral pill on hover. The pill hangs 9pt into the left margin so
// the icon sits on the 16pt grid.
func ghostButton(c *ui.Context, icon *ui.SVG, title string, loading, disabled bool) ui.Element {
	t := c.Theme()
	inactive := loading || disabled
	b := ui.ButtonBase(c).Label(title).Tooltip(title).Disabled(inactive).Padding(5, 9).Gap(5).Radius(radiusRow).
		AlignItems(ui.Center).Margin(0, 0, 0, -9).Cursor(ui.CursorPointer)
	ink := inkSecondary(t)
	if b.Hovered() && !inactive {
		ink = t.Text
		b.Background(hoverNeutral(t))
	}
	return b.Children(func() {
		if loading {
			ui.Spinner(c).Size(12, 12)
		} else {
			ui.Icon(c, icon).Size(11, 11).TextColor(ink)
		}
		ui.Text(c, title).FontSize(fsCallout).FontWeight(500).TextColor(ink).SingleLine()
	})
}

// accentButton is ToolbarAccentButton: the filled call to action.
func accentButton(c *ui.Context, icon *ui.SVG, title string, disabled bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(title).Tooltip(title).Disabled(disabled).Padding(0, 11).Gap(5).Height(26).Radius(radiusRow).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	fill, ink := t.Accent, t.AccentText
	switch {
	case disabled:
		fill, ink = fill.Alpha(0.45), ink.Alpha(0.7)
	case b.Pressed():
		fill = t.AccentPressed
	case b.Hovered():
		fill = t.AccentHover
	}
	b.Background(fill)
	return b.Children(func() {
		ui.Icon(c, icon).Size(10, 10).TextColor(ink)
		ui.Text(c, title).FontSize(fsCallout).FontWeight(600).TextColor(ink).SingleLine()
	})
}

// privacyButton is AccountPrivacyModeButton: the eye of the header and the
// accounts toolbar. Privacy on shows the slashed eye in the accent color.
func privacyButton(c *ui.Context, l l10n.Localizer, on bool, w, h, radius float32) ui.Element {
	return toggleIconButton(c, pick(on, icEyeOff, icEye), l.Text(l10n.KeyAccountPrivacyMode), on, w, h, radius)
}

// toggleIconButton is an icon switch: on shows the icon in the accent color
// on a neutral wash.
func toggleIconButton(c *ui.Context, icon *ui.SVG, title string, on bool, w, h, radius float32) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(title).Tooltip(title).Size(w, h).Radius(radius).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
	hovered := b.Hovered()
	ink := inkSecondary(t)
	switch {
	case on:
		ink = brandInk(t, t.Accent)
	case hovered:
		ink = t.Text
	}
	if on || hovered {
		b.Background(hoverNeutral(t))
	}
	return b.Children(func() { ui.Icon(c, icon).Size(14, 14).TextColor(ink) })
}

// accountIconButton is AccountIconActionButton: a 28 by 24 icon control. The
// current-login tick keeps its green wash even though it is disabled.
func accountIconButton(c *ui.Context, icon *ui.SVG, title string, disabled, loading, current bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(title).Tooltip(title).Disabled(disabled || loading).Size(28, 24).Radius(radiusRow).
		AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
	ink, hovered := t.Text, b.Hovered()
	switch {
	case loading:
		ink = inkSecondary(t)
	case current:
		ink = sys(t, sysGreen)
		b.Background(sys(t, sysGreen).Alpha(pick[float32](hovered, 0.16, 0.10)))
	case disabled:
		ink = inkSecondary(t)
	case b.Pressed():
		ink = brandInk(t, t.AccentPressed)
		b.Background(t.Accent.Alpha(0.16))
	case hovered:
		ink = brandInk(t, t.AccentHover)
		b.Background(t.Accent.Alpha(0.12))
	}
	return b.Children(func() {
		if loading {
			ui.Spinner(c).Size(14, 14)
			return
		}
		ui.Icon(c, icon).Size(14, 14).TextColor(ink)
	})
}

// accountCard is AccountManagedCardChrome: the current login gets a green
// wash and stroke, the others a raised card that brightens under the pointer.
func accountCard(c *ui.Context, current bool, body func()) {
	t := c.Theme()
	col := ui.Column(c).Gap(10).Padding(11, 12).Radius(radiusCard)
	green := sys(t, sysGreen)
	switch {
	case current:
		col.Background(green.Alpha(pick[float32](!t.Dark, 0.10, 0.12)))
		col.Border(1, green.Alpha(pick[float32](!t.Dark, 0.28, 0.35)))
	case col.Hovered():
		col.Background(hoverNeutral(t))
	default:
		col.Background(surfaceRaised(t))
	}
	if !current && t.Dark {
		col.Border(1, hairlineColor(t))
	}
	col.Children(body)
}

// meterSpec is one meter of an account card.
type meterSpec struct {
	title             string
	remaining, used   int
	trailing          string // an amount shown where a reset would be
	resetsAt          time.Time
	hasReset, current bool
}

// meterValueWidth keeps the bars of a card the same length beside their
// percentages.
const meterValueWidth float32 = 58

// accountMeter is two lines: the title with the reset (or an amount), then
// the bar with what is left.
func accountMeter(c *ui.Context, l l10n.Localizer, m meterSpec, view accountQuotaOptions) {
	t := c.Theme()
	health := runway.HealthForUsedPercent(m.used)
	ui.Column(c).Gap(5).MinWidth(0).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, m.title).FontSize(11).FontWeight(600).SingleLine().Ellipsis("…").Tooltip(m.title).Grow(1).Basis(0).MinWidth(0)
			switch {
			case m.hasReset:
				now := c.Now()
				dimText(c, accountResetText(l, m.resetsAt, now, view.resetFormat)).FontFeatures("tnum").NoWrap().Shrink(0).
					Tooltip(l.Text(l10n.KeyNextResetIn) + m.resetsAt.In(time.Local).Format("2006-01-02 15:04:05 MST"))
				c.After(time.Second)
			case m.trailing != "":
				ui.Text(c, m.trailing).FontSize(fsCaption).FontWeight(600).FontFeatures("tnum").TextColor(inkSecondary(t)).SingleLine().Shrink(0)
			}
		})
		meterBarRow(c, accountBarColor(t, health), m.remaining, m.current, l10n.Format("%d%% %s", m.remaining, l.Text(l10n.KeyLeft)), healthInk(t, health))
	})
}

// accountShareRow is a product share of a Grok card on one line: the
// product, a bar of the used share and the share.
func accountShareRow(c *ui.Context, l l10n.Localizer, m runway.Meter, current bool) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, m.Title).FontSize(fsCaption).FontWeight(500).TextColor(inkSecondary(t)).SingleLine().Ellipsis("…").Tooltip(m.Title).Width(80).Shrink(0)
		meterBarRow(c, healthFill(t, m.Health), m.UsedPercent, current, l10n.Format("%d%% %s", m.UsedPercent, l.Text(l10n.KeyUsed)), healthInk(t, m.Health))
	})
}

// meterBarRow is a capsule bar filled to percent, then its value at a fixed
// width.
func meterBarRow(c *ui.Context, fill ui.Color, percent int, current bool, value string, ink ui.Color) {
	t := c.Theme()
	track := surfaceSunken(t)
	if current {
		track = sys(t, sysGreen).Alpha(0.14)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Grow(1).Basis(0).MinWidth(0).Children(func() {
		ui.Box(c).Height(6).Grow(1).Basis(0).MinWidth(0).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, track, r.H/2)
			if percent <= 0 {
				return
			}
			w := max(r.H, r.W*float32(min(percent, 100))/100)
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: min(w, r.W), H: r.H}, fill, r.H/2)
		})
		ui.Text(c, value).FontSize(fsCaption).FontWeight(600).FontFeatures("tnum").TextColor(ink).SingleLine().MinWidth(meterValueWidth).TextAlign(ui.End)
	})
}

// accountBarColor is the bar tint: yellow health reads as orange here.
func accountBarColor(t *ui.Theme, h runway.Health) ui.Color {
	switch h {
	case runway.HealthYellow:
		return sys(t, sysOrange)
	case runway.HealthRed:
		return sys(t, sysRed)
	}
	return sys(t, sysGreen)
}

// accountStatusLine is a caption-size message of a card.
func accountStatusLine(c *ui.Context, text string, ink ui.Color) {
	ui.Text(c, text).FontSize(fsCaption).TextColor(ink).MaxLines(2)
}

// accountsNotice is the operation message and the last error under the list.
func accountsNotice(c *ui.Context, messages []string, failure string) {
	t := c.Theme()
	for _, m := range messages {
		if m != "" {
			ui.Text(c, m).FontSize(fsCaption).TextColor(inkSecondary(t)).Padding(2, 0, 0, 0)
		}
	}
	if failure != "" {
		ui.Text(c, failure).FontSize(fsCaption).TextColor(sys(t, sysRed)).Padding(2, 0, 0, 0)
	}
}

// accountEmpty is the placeholder card of an empty list.
func accountEmpty(c *ui.Context, text string) {
	cardColumn(c, surfaceRaisedKind, radiusCard).Padding(12).Children(func() { calloutDim(c, text) })
}

// accountPrivate masks an identity when privacy mode is on.
func accountPrivate(on bool, v string) string {
	if !on || v == "" {
		return v
	}
	return codex.MaskAccountText(v)
}

// confirmPopover is the sheet or alert of the Swift pages as a popover
// anchored to the control that asked: a title, a message, optional extra
// controls and a Cancel / confirm pair.
func confirmPopover(c *ui.Context, l l10n.Localizer, anchor ui.Element, open *bool, title, message, confirm string, extra func(), onConfirm func()) {
	panelPopover(c, anchor, open, func() {
		ui.Column(c).Gap(14).Width(280).Padding(4).Children(func() {
			ui.Text(c, title).FontSize(fsTitle3).FontWeight(600)
			if message != "" {
				calloutDim(c, message)
			}
			if extra != nil {
				extra()
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c).Grow(1)
				if PillButton(c, nil, l.Text(l10n.KeyCancel), false).Clicked() {
					*open = false
				}
				if PillButton(c, nil, confirm, false).Clicked() {
					*open = false
					onConfirm()
				}
			})
		})
	})
}
