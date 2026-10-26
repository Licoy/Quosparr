package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// accountQuotaOptions is how a card shows its quota. The current account
// tints its tracks, the panel adds the update time that the settings row
// keeps in its header, and the wide settings rows lay the meters out in two
// columns. The zero value shows the credits balance and relative resets.
type accountQuotaOptions struct {
	current, showUpdated, wide bool
	hideCredits                bool
	resetFormat                store.ResetTimeFormat
}

// quotaOptionsFor carries the reset format and the credits switch of the
// preferences to a card's quota.
func quotaOptionsFor(prefs store.Preferences, current bool) accountQuotaOptions {
	return accountQuotaOptions{current: current, hideCredits: !prefs.ShowsCreditsBalance, resetFormat: prefs.QuotaResetTimeFormat}
}

func (o accountQuotaOptions) meterColumns() int { return pick(o.wide, 2, 1) }
func (o accountQuotaOptions) statColumns() int  { return pick(o.wide, 4, 2) }

// accountStat is one figure under the meters: the reset credits, a balance
// or the extra usage.
type accountStat struct {
	label, value string
	note, tip    string
	warn         bool // note is a failure
}

func claudeQuotaBlock(c *ui.Context, l l10n.Localizer, q *runway.ClaudeAccountQuota, view accountQuotaOptions) {
	ui.Column(c).Gap(10).Children(func() {
		if q == nil || len(q.Meters) == 0 {
			dimText(c, l.Text(l10n.KeyClaudeNoQuotaData))
		}
		zero := 0
		if q == nil {
			accountStats(c, view, []accountStat{accountResetStat(l, &zero, time.Time{}, time.Time{}, "")})
			return
		}
		accountMeters(c, view, len(q.Meters), func(i int) {
			accountMeter(c, l, projectedAccountMeter(q.Meters[i], view.current), view)
		})
		count := accountResetCount(q.ResetGrants, c.Now())
		stats := []accountStat{accountResetStat(l, &count, q.ResetCreditsUpdatedAt, accountResetExpiry(q.ResetGrants, c.Now()), "")}
		redrawAtNext(c, grantBounds(q.ResetGrants))
		if q.ExtraUsage != "" {
			stats = append(stats, accountStat{label: l.Text(l10n.KeyClaudeExtraUsage), value: q.ExtraUsage})
		}
		accountStats(c, view, stats)
		if view.showUpdated {
			accountQuotaUpdated(c, l, q.UpdatedAt)
		}
	})
}

func grokQuotaBlock(c *ui.Context, l l10n.Localizer, q *runway.GrokAccountQuota, view accountQuotaOptions) {
	ui.Column(c).Gap(10).Children(func() {
		if q == nil || len(q.Meters) == 0 {
			dimText(c, l.Text(l10n.KeyGrokNoQuotaData))
		}
		if q == nil {
			accountStats(c, view, []accountStat{accountResetStat(l, nil, time.Time{}, time.Time{}, "")})
			return
		}
		if len(q.Meters) > 0 {
			grokAccountMeters(c, l, q.Meters, view)
		}
		var count *int
		if q.ResetCreditsKnown {
			n := accountResetCount(q.ResetGrants, c.Now())
			count = &n
		}
		stats := []accountStat{accountResetStat(l, count, q.ResetCreditsUpdatedAt, accountResetExpiry(q.ResetGrants, c.Now()), "")}
		redrawAtNext(c, grantBounds(q.ResetGrants))
		if q.IncludedUSD != "" {
			stats = append(stats, accountStat{label: l.Text(l10n.KeyGrokIncludedQuota), value: q.IncludedUSD})
		}
		for _, d := range q.Details {
			stats = append(stats, accountStat{label: d.Title, value: d.Value})
		}
		accountStats(c, view, stats)
		if view.showUpdated {
			accountQuotaUpdated(c, l, q.UpdatedAt)
		}
	})
}

// grokAccountMeters is the included quota, then the product shares; the
// wide rows put the shares beside the quota.
func grokAccountMeters(c *ui.Context, l l10n.Localizer, meters []runway.Meter, view accountQuotaOptions) {
	n := 1
	if len(meters) > 1 {
		n = 2
	}
	accountMeters(c, view, n, func(i int) {
		if i == 0 {
			accountMeter(c, l, projectedAccountMeter(meters[0], view.current), view)
			return
		}
		ui.Column(c).Gap(6).MinWidth(0).Children(func() {
			for _, m := range meters[1:] {
				accountShareRow(c, l, m, view.current)
			}
		})
	})
}

func projectedAccountMeter(m runway.Meter, current bool) meterSpec {
	return meterSpec{title: m.Title, remaining: m.RemainingPercent, used: m.UsedPercent, resetsAt: m.ResetsAt, hasReset: m.HasReset(), current: current}
}

// accountMeters is the grid of the meters of a card.
func accountMeters(c *ui.Context, view accountQuotaOptions, n int, meter func(i int)) {
	ui.Grid(c).Columns(view.meterColumns()).GapX(20).GapY(12).Children(func() {
		for i := range n {
			meter(i)
		}
	})
}

// accountStats is the grid of figures under the meters, in tiles of equal
// height.
func accountStats(c *ui.Context, view accountQuotaOptions, stats []accountStat) {
	if len(stats) == 0 {
		return
	}
	ui.Grid(c).Columns(view.statColumns()).Gap(6).Children(func() {
		for _, s := range stats {
			accountStatTile(c, s, view.current)
		}
	})
}

// accountStatTile is the label, the figure and a note under it.
func accountStatTile(c *ui.Context, s accountStat, current bool) {
	t := c.Theme()
	fill := surfaceSunken(t)
	if current {
		fill = sys(t, sysGreen).Alpha(pick[float32](t.Dark, 0.10, 0.08))
	}
	tile := ui.Column(c).Gap(2).Padding(6, 9).Radius(radiusRow).Background(fill).MinWidth(0)
	tip := s.label + ": " + s.value
	if s.note != "" {
		tip += "\n" + s.note
	}
	if s.tip != "" {
		tip = s.tip
	}
	tile.Tooltip(tip).Children(func() {
		ui.Text(c, s.label).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine().Ellipsis("…")
		ui.Text(c, s.value).FontSize(fsCallout).FontWeight(600).FontFeatures("tnum").MaxLines(2)
		if s.note != "" {
			ink := inkSecondary(t)
			if s.warn {
				ink = sys(t, sysOrange)
			}
			ui.Text(c, s.note).FontSize(fsCaption).TextColor(ink).FontFeatures("tnum").MaxLines(2)
		}
	})
}

// grantBounds are the times at which grants start or end.
func grantBounds(grants []runway.AccountResetGrant) []time.Time {
	var out []time.Time
	for _, g := range grants {
		out = append(out, g.StartsAt, g.EndsAt)
	}
	return out
}

// redrawAtNext asks for a frame at the first of times still ahead, when the
// count or the expiry of the reset credits changes.
func redrawAtNext(c *ui.Context, times []time.Time) {
	now := c.Now()
	var next time.Time
	for _, at := range times {
		if at.After(now) && (next.IsZero() || at.Before(next)) {
			next = at
		}
	}
	if !next.IsZero() {
		c.After(next.Sub(now))
	}
}

func accountResetCount(grants []runway.AccountResetGrant, now time.Time) int {
	count := 0
	for _, g := range grants {
		if !g.StartsAt.After(now) && (g.EndsAt.IsZero() || g.EndsAt.After(now)) {
			count += max(0, g.Count)
		}
	}
	return count
}

func accountResetExpiry(grants []runway.AccountResetGrant, now time.Time) time.Time {
	var next time.Time
	for _, g := range grants {
		if g.Count > 0 && !g.StartsAt.After(now) && g.EndsAt.After(now) && (next.IsZero() || g.EndsAt.Before(next)) {
			next = g.EndsAt
		}
	}
	return next
}

// accountResetStat is the reset credits tile: the available count, unknown
// as a dash, with the next expiry or the failure of the last check.
func accountResetStat(l l10n.Localizer, count *int, at, expires time.Time, failure string) accountStat {
	s := accountStat{label: l.Text(l10n.KeyResetCredits), value: "—"}
	if count != nil {
		s.value = fmt.Sprint(*count)
	}
	s.tip = accountUpdatedSummary(l, l.Text(l10n.KeyAvailableResets)+": "+s.value, at)
	switch {
	case failure != "":
		s.note, s.warn = failure, true
	case count == nil:
		s.note = l.Text(l10n.KeyNotLoaded)
	case *count > 0 && expires.IsZero():
		s.note = l.Text(l10n.KeyResetExpiryUnavailable)
	case *count > 0:
		s.note = accountResetExpiryText(l, expires)
		s.tip += "\n" + l.Text(l10n.KeyNextExpiry) + " " + expires.In(time.Local).Format("2006-01-02 15:04:05 MST")
	}
	return s
}

// accountResetExpiryText keeps the date and the time on one line.
func accountResetExpiryText(l l10n.Localizer, expires time.Time) string {
	return l.Text(l10n.KeyNextExpiry) + " " + expires.In(time.Local).Format("01-02\u00a015:04")
}

// accountResetText is the reset of a meter in the chosen format, without
// seconds: "Resets in 2h 30m" or "Resets 22:51".
func accountResetText(l l10n.Localizer, at, now time.Time, f store.ResetTimeFormat) string {
	if f == store.ResetTimeExact {
		return tx(l, xAccountResetsAt, runway.ResetExactTime(at, now, l.Language(), nil))
	}
	return tx(l, xAccountResetsIn, runway.ShortDuration(at.Sub(now).Seconds(), l.Language()))
}

func accountQuotaUpdated(c *ui.Context, l l10n.Localizer, at time.Time) {
	if !at.IsZero() {
		dimText(c, accountQuotaUpdatedText(l, at)).NoWrap().Tooltip(at.In(time.Local).Format("2006-01-02 15:04:05 MST"))
	}
}

func accountQuotaUpdatedText(l l10n.Localizer, at time.Time) string {
	return l.Text(l10n.KeyLastUpdated) + " " + at.In(time.Local).Format("01-02 15:04")
}

func accountWindowTitle(l l10n.Localizer, minutes int) string {
	if minutes == runway.FiveHourMinutes || minutes == runway.WeeklyMinutes {
		return runway.WindowTitle(l, minutes)
	}
	if minutes > 0 {
		return runway.ShortDuration(float64(minutes)*60, l.Language())
	}
	return l.Text(l10n.KeyQuota)
}
