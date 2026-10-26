package app

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// quotaBarHeight is RunwayProgressBar.barHeight.
const quotaBarHeight float32 = 5

// healthFill is the bar color of a meter health.
func healthFill(t *ui.Theme, h runway.Health) ui.Color {
	switch h {
	case runway.HealthYellow:
		return sys(t, sysYellow)
	case runway.HealthRed:
		return sys(t, sysRed)
	}
	return sys(t, sysGreen)
}

// healthInk is the text-safe variant of healthFill: yellow turns orange in
// light mode and every light-mode ink is deepened by 35% black.
func healthInk(t *ui.Theme, h runway.Health) ui.Color {
	base := healthFill(t, h)
	if t.Dark {
		return base
	}
	if h == runway.HealthYellow {
		base = sys(t, sysOrange)
	}
	return base.Mix(ui.RGBA(0, 0, 0, 1), 0.35)
}

// meterTitle names a standard Codex window in the user's language; the
// service's own title is kept for model-specific windows and other providers.
func meterTitle(l l10n.Localizer, m runway.Meter, codex bool) string {
	if codex && m.WindowMinutes > 0 {
		if m.Source == runway.SourceStandard {
			return accountWindowTitle(l, m.WindowMinutes)
		}
		return m.Title + " · " + accountWindowTitle(l, m.WindowMinutes)
	}
	return m.Title
}

// panelCodexQuota is the quota section: one row per meter and, with
// showCredits, a nonzero ChatGPT credits balance as the last row.
func panelCodexQuota(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState, rs resetSwitch, private, showCredits bool) {
	refreshing := sectionRefreshing(snap, runway.SectionQuota)
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, sectionSpec{title: l.Text(l10n.KeyQuota), refreshing: refreshing, refresh: sectionRefresh(acts, runway.SectionQuota)})
		switch v := snap.Codex; {
		case v.APIKeyMode && v.APIBaseURL != "":
			panelAPIBalance(c, l, v.APIBaseURL, v.APIBalance, "", refreshing, private)
		case v.APIKeyMode:
			dimText(c, l.Text(l10n.KeyAccountsAPIKeyHint)).MaxLines(2)
		default:
			meterRows(c, l, v.Meters, refreshing, true, rs)
			if balance, ok := codexCreditsBalance(snap); ok && showCredits {
				creditsBalanceRow(c, l, balance)
			}
		}
	})
}

// creditsBalanceRow is the last line of the Codex quota section: a coin, the
// label and the amount, without a bar.
func creditsBalanceRow(c *ui.Context, l l10n.Localizer, balance float64) {
	t := c.Theme()
	ui.Row(c).Gap(6).Padding(4, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icCoins).Size(13, 13).TextColor(sys(t, sysBlue))
		ui.Text(c, l.Text(l10n.KeyCreditsBalance)).FontSize(fsCallout).FontWeight(600).Grow(1).Basis(0).SingleLine()
		ui.Text(c, creditsBalanceAmount(balance)).FontSize(fsCallout).FontWeight(600).FontFeatures("tnum").SingleLine()
	})
}

func meterRows(c *ui.Context, l l10n.Localizer, meters []runway.Meter, refreshing, codex bool, rs resetSwitch) {
	if len(meters) == 0 {
		sectionEmpty(c, l, refreshing)
		return
	}
	now := time.Now()
	ui.Column(c).Gap(12).Children(func() {
		for i, m := range meters {
			quotaRow(c.Key("quota-"+strconv.Itoa(i)), l, m, now, codex, rs)
		}
	})
}

// quotaRow is QuotaMetersView.quotaRow: title and "N% left", the bar, the
// burn projection and the reset countdown.
func quotaRow(c *ui.Context, l l10n.Localizer, m runway.Meter, now time.Time, codex bool, rs resetSwitch) {
	t := c.Theme()
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, meterTitle(l, m, codex)).FontSize(fsCallout).FontWeight(600).Grow(1).Basis(0)
			ui.Text(c, l10n.Format("%d%% %s", m.RemainingPercent, l.Text(l10n.KeyLeft))).FontSize(fsCallout).FontWeight(600).FontFeatures("tnum").TextColor(healthInk(t, m.Health)).SingleLine()
		})
		quotaBar(c, m)
		ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
			if text := projectionText(l, m, now); text != "" {
				dimText(c, text).Grow(1).Basis(0)
			} else {
				ui.Spacer(c).Grow(1)
			}
			if m.HasReset() {
				resetLine(c, l, "reset", m.ResetsAt, now, rs)
			}
		})
	})
}

// projectionText is "Burn rate: exhausts in ..." or "... projected at reset N%".
func projectionText(l l10n.Localizer, m runway.Meter, now time.Time) string {
	p := m.Projection
	if p == nil {
		return ""
	}
	if !p.ExhaustsAt.IsZero() {
		return l.Text(l10n.KeyBurnRate) + ": " + l.Text(l10n.KeyExhaustsIn) + " " + quotaDuration(l, p.ExhaustsAt.Sub(now))
	}
	return l10n.Format("%s: %s %d%%", l.Text(l10n.KeyBurnRate), l.Text(l10n.KeyProjectedAtReset), p.ProjectedUsedPercentAtReset)
}

func resetCountdown(l l10n.Localizer, at, now time.Time) string {
	return l.Text(l10n.KeyNextResetIn) + quotaDuration(l, at.Sub(now))
}

// resetSwitch is the reset time format of the quota rows and the action that
// flips it; without the action the reset is plain text.
type resetSwitch struct {
	format store.ResetTimeFormat
	toggle func()
}

func (s *panelState) resetSwitch(f store.ResetTimeFormat) resetSwitch {
	rs := resetSwitch{format: f}
	if set := s.setResetTimeFormat; set != nil {
		rs.toggle = func() { set(f.Toggled()) }
	}
	return rs
}

// resetText is "Next reset: " and the countdown or the local reset time.
func resetText(l l10n.Localizer, at, now time.Time, f store.ResetTimeFormat) string {
	if f == store.ResetTimeExact {
		return l.Text(l10n.KeyNextResetIn) + runway.ResetExactTime(at, now, l.Language(), nil)
	}
	return resetCountdown(l, at, now)
}

func resetTimeFormatName(l l10n.Localizer, f store.ResetTimeFormat) string {
	if f == store.ResetTimeExact {
		return tx(l, xResetTimeExact)
	}
	return tx(l, xResetTimeRelative)
}

// resetLine is the reset of a quota row. With a toggle the whole line, a
// swap glyph at its end, switches the format and its tooltip shows the
// other one.
func resetLine(c *ui.Context, l l10n.Localizer, key string, at, now time.Time, rs resetSwitch) {
	text := resetText(l, at, now, rs.format)
	if rs.toggle == nil {
		dimText(c, text).FontFeatures("tnum").NoWrap().Shrink(0)
		return
	}
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Label(text).Tooltip(resetText(l, at, now, rs.format.Toggled())).
		Gap(4).AlignItems(ui.Center).Shrink(0).Cursor(ui.CursorPointer)
	ink := inkSecondary(t)
	if b.Hovered() {
		ink = t.Text
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(fsCaption).TextColor(ink).FontFeatures("tnum").NoWrap()
		ui.Icon(c, icSwitch).Size(10, 10).TextColor(ink)
	})
	if b.Clicked() {
		rs.toggle()
	}
}

// quotaDuration is DurationFormatter.localized, with seconds.
func quotaDuration(l l10n.Localizer, d time.Duration) string {
	return runway.DurationLocalized(d.Seconds(), l.Language(), true)
}

// quotaBar is RunwayProgressBar: a sunken capsule track, the remaining
// share in the health color and a faint tick at every threshold.
func quotaBar(c *ui.Context, m runway.Meter) {
	t := c.Theme()
	fill := healthFill(t, m.Health)
	ticks := mono(t, 0.028, 0.028)
	ui.Box(c).Height(quotaBarHeight).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, surfaceSunken(t), r.H/2)
		w := max(4, r.W*float32(m.RemainingPercent)/100)
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: min(w, r.W), H: r.H}, fill, r.H/2)
		for _, marker := range m.MarkerPercents {
			x := min(max(1, r.W*float32(marker)/100), r.W-1)
			h := max(3, r.H-2)
			p.Fill(ui.Rect{X: r.X + x, Y: r.Y + (r.H-h)/2, W: 1, H: h}, ticks, 0.5)
		}
	})
}

// usedBar is the slim bar of the secondary Grok rows: the used share.
func usedBar(c *ui.Context, m runway.Meter) {
	t := c.Theme()
	fill := healthFill(t, m.Health)
	ui.Box(c).Height(max(3, quotaBarHeight-1)).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, surfaceSunken(t), r.H/2)
		w := max(4, r.W*float32(m.UsedPercent)/100)
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: min(w, r.W), H: r.H}, fill, r.H/2)
	})
}
