package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
)

const (
	// speedCardRows is how many model rows the main card shows.
	speedCardRows = 3
	// speedColTTFT and speedColRate are the figure columns of a model row.
	speedColTTFT float32 = 72
	speedColRate float32 = 68
)

// panelSpeed is the speed analysis summary: the most used models with their
// time to first token and output speed, where the numbers come from and a
// row to the details page.
func panelSpeed(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState) {
	v := snap.Codex
	busy := sectionRefreshing(snap, runway.SectionSpeed)
	title := tx(l, xSpeedTitle)
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, sectionSpec{
			title: title, refreshing: busy, refresh: sectionRefresh(acts, runway.SectionSpeed),
			info: &sectionInfo{id: "speed-info", help: title, body: popoverBody(c, l, st, title, speedInfo(l, v.Speed)...)},
		})
		switch {
		case v.Speed != nil:
			speedSummary(c, l, st, *v.Speed, v.SpeedError)
		case v.SpeedError != "":
			ui.Text(c, v.SpeedError).FontSize(fsCallout).TextColor(sys(c.Theme(), sysRed))
		default:
			sectionEmpty(c, l, busy)
		}
	})
}

func speedSummary(c *ui.Context, l l10n.Localizer, st *panelState, r speed.Report, errText string) {
	if r.Empty() {
		calloutDim(c, tx(l, xSpeedEmpty))
		speedError(c, errText)
		return
	}
	ui.Column(c).Gap(6).Children(func() {
		cardColumn(c, surfaceSunkenKind, radiusCard).Children(func() {
			for i, g := range r.Groups[:min(speedCardRows, len(r.Groups))] {
				speedGroupRow(c, l, g, i == 0, !r.NoTTFT)
			}
		})
		speedAttribution(c, l, r)
		speedError(c, errText)
	})
	if disclosureRow(c, nil, l.Text(l10n.KeyShowDetails), true).Clicked() {
		st.detail = store.ModuleSpeedAnalysis
	}
}

// speedError is the user-facing error of the last scan, kept under stale data.
func speedError(c *ui.Context, errText string) {
	if errText != "" {
		ui.Text(c, errText).FontSize(fsCaption).TextColor(sys(c.Theme(), sysRed))
	}
}

// speedInfo is the explanation of the figures of r, nil before the first
// scan; without TTFT it says why the column is missing instead of defining
// it, and a tiered report adds how the tiers are told apart.
func speedInfo(l l10n.Localizer, r *speed.Report) []string {
	first := tx(l, xSpeedInfoTTFT)
	if r != nil && r.NoTTFT {
		first = tx(l, xSpeedInfoNoTTFT)
	}
	info := []string{first, tx(l, xSpeedInfoOutput), tx(l, xSpeedInfoSource)}
	if r != nil && r.Tiered {
		info = append(info, tx(l, xSpeedInfoTier))
	}
	return info
}

// speedGroupRow is one model at one effort: its name and effort, then the
// time to first token when ttft and the output speed in fixed columns.
func speedGroupRow(c *ui.Context, l l10n.Localizer, s speed.Stat, first, ttft bool) {
	ui.Column(c).Children(func() {
		tableRowRule(c, first)
		ui.Row(c).Gap(8).Padding(7, 10).AlignItems(ui.Center).Children(func() {
			speedModelCell(c, l, s)
			if ttft {
				speedFigure(c, tx(l, xSpeedTTFT), speedTTFT(s.TTFTP50, s.Calls), speedColTTFT)
			}
			speedFigure(c, "", speedRate(s, true), speedColRate)
		})
	})
}

// speedModelCell shows the model, reasoning effort and tier on one line.
func speedModelCell(c *ui.Context, l l10n.Localizer, s speed.Stat) {
	t := c.Theme()
	name := speedModelName(l, s.Model)
	spans := []ui.Span{{Text: name, Size: fsCallout, Weight: 500}}
	if s.Effort != "" {
		spans = append(spans, ui.Span{Text: " " + s.Effort, Size: fsCallout, Color: inkSecondary(t)})
		name += " " + s.Effort
	}
	if tier := speedTierLabel(l, s.Tier); tier != "" {
		stops := []ui.Color{inkSecondary(t)}
		switch s.Tier {
		case speed.TierFast:
			stops[0] = sys(t, sysPurple)
		case speed.TierUltrafast:
			stops = pick(!t.Dark,
				[]ui.Color{ui.Hex("#B21C36"), ui.Hex("#946200")},
				[]ui.Color{ui.Hex("#FF6466"), ui.Hex("#FFD074")},
			)
		}
		tierSpans := gradientSpans(tier, stops, fsCallout, 500)
		tierSpans[0].Text = " · " + tierSpans[0].Text
		spans = append(spans, tierSpans...)
		name += " · " + tier
	}
	ui.RichText(c, spans...).SingleLine().Ellipsis("…").Grow(1).Basis(0).Tooltip(name)
}

// speedTierLabel names a known tier; an unknown tier has no label.
func speedTierLabel(l l10n.Localizer, tier speed.Tier) string {
	switch tier {
	case speed.TierFast:
		return tx(l, xSpeedTierFast)
	case speed.TierUltrafast:
		return tx(l, xSpeedTierUltrafast)
	case speed.TierStandard:
		return tx(l, xSpeedTierStandard)
	}
	return ""
}

// speedFigure is an optional dim label before a right-aligned figure.
func speedFigure(c *ui.Context, label, value string, width float32) {
	t := c.Theme()
	ui.Row(c).Width(width).Shrink(0).Gap(3).Justify(ui.End).Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine()
		}
		ui.Text(c, value).FontSize(fsCaption).FontWeight(500).FontFeatures("tnum").SingleLine()
	})
}

// speedAttribution is the grey line under the rows: how many sessions the
// numbers come from, the newest one by title and how long ago it ran. Only
// the title shrinks when the line is too long.
func speedAttribution(c *ui.Context, l l10n.Localizer, r speed.Report) {
	t := c.Theme()
	grey := func(s string) ui.Element {
		return ui.Text(c, s).FontSize(fsCaption).TextColor(inkTertiary(t)).SingleLine()
	}
	basis := tx(l, xSpeedBasisOne)
	if n := len(r.Sessions); n > 1 {
		basis = tx(l, xSpeedBasis, n)
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		grey(basis).Shrink(0)
		if len(r.Sessions) == 0 {
			return
		}
		latest := r.Sessions[0]
		if title := speedOneLine(latest.Title); title != "" {
			grey("·").Shrink(0)
			text := tx(l, xSpeedLatest, title)
			grey(text).Ellipsis("…").Tooltip(text)
		}
		if !latest.UpdatedAt.IsZero() {
			grey("·").Shrink(0)
			grey(runway.RelativePast(latest.UpdatedAt, time.Now(), l.Language(), false)).Shrink(0)
		}
	})
}

// speedOneLine folds a title's line breaks and runs of blanks into spaces.
func speedOneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func speedModelName(l l10n.Localizer, model string) string {
	if name := speedOneLine(model); name != "" {
		return name
	}
	return l.Text(l10n.KeyUnknown)
}

// speedTTFT is a time to first token in seconds; "—" without samples.
func speedTTFT(d time.Duration, calls int) string {
	if calls == 0 || d <= 0 {
		return "—"
	}
	if s := d.Seconds(); s < 100 {
		return fmt.Sprintf("%.1fs", s)
	}
	return fmt.Sprintf("%.0fs", d.Seconds())
}

// speedRate is the output speed with its unit when withUnit; "—" without
// output samples.
func speedRate(s speed.Stat, withUnit bool) string {
	if !s.HasTPS() {
		return "—"
	}
	v, text := s.TPS(), ""
	if v < 10 {
		text = fmt.Sprintf("%.1f", v)
	} else {
		text = fmt.Sprintf("%.0f", v)
	}
	if withUnit {
		return text + " tok/s"
	}
	return text
}
