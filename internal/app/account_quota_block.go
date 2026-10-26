package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

type accountQuotaOptions struct{ current, showUpdated bool }

func claudeQuotaBlock(c *ui.Context, l l10n.Localizer, q *runway.ClaudeAccountQuota, view accountQuotaOptions) {
	ui.Column(c).Gap(8).Children(func() {
		if q == nil {
			dimText(c, l.Text(l10n.KeyClaudeNoQuotaData))
			zero := 0
			accountResetCredits(c, l, &zero, time.Time{}, time.Time{})
			return
		}
		if len(q.Meters) == 0 {
			dimText(c, l.Text(l10n.KeyClaudeNoQuotaData))
		}
		for _, m := range q.Meters {
			accountMeter(c, l, projectedAccountMeter(m, view.current))
		}
		if q.ExtraUsage != "" {
			dimText(c, q.ExtraUsage).MaxLines(2)
		}
		if view.showUpdated {
			accountQuotaUpdated(c, l, q.UpdatedAt)
		}
		count := accountResetCount(q.ResetGrants, c.Now())
		accountResetCredits(c, l, &count, q.ResetCreditsUpdatedAt, accountResetExpiry(q.ResetGrants, c.Now()))
	})
}

func grokQuotaBlock(c *ui.Context, l l10n.Localizer, q *runway.GrokAccountQuota, view accountQuotaOptions) {
	ui.Column(c).Gap(8).Children(func() {
		if q == nil || len(q.Meters) == 0 {
			dimText(c, l.Text(l10n.KeyGrokNoQuotaData))
		}
		if q == nil {
			accountResetCredits(c, l, nil, time.Time{}, time.Time{})
			return
		}
		for i, m := range q.Meters {
			if i > 0 {
				grokMeterRow(c, l, m, false, c.Now(), resetSwitch{})
				continue
			}
			spec := projectedAccountMeter(m, view.current)
			spec.trailing = q.IncludedUSD
			accountMeter(c, l, spec)
		}
		for _, detail := range q.Details {
			dimText(c, detail.Title+": "+detail.Value).MaxLines(2)
		}
		if view.showUpdated {
			accountQuotaUpdated(c, l, q.UpdatedAt)
		}
		var count *int
		if q.ResetCreditsKnown {
			n := accountResetCount(q.ResetGrants, c.Now())
			count = &n
		}
		accountResetCredits(c, l, count, q.ResetCreditsUpdatedAt, accountResetExpiry(q.ResetGrants, c.Now()))
	})
}

func projectedAccountMeter(m runway.Meter, current bool) meterSpec {
	return meterSpec{title: m.Title, remaining: m.RemainingPercent, used: m.UsedPercent, resetsAt: m.ResetsAt, hasReset: m.HasReset(), current: current}
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

func accountResetCredits(c *ui.Context, l l10n.Localizer, count *int, at, expires time.Time) {
	value := "—"
	if count != nil {
		value = fmt.Sprint(*count)
	}
	text := l.Text(l10n.KeyAvailableResets) + ": " + value
	hint := accountUpdatedSummary(l, text, at)
	if count == nil {
		hint += " · " + l.Text(l10n.KeyNotLoaded)
	}
	dimText(c, text).MaxLines(2).Tooltip(hint)
	if count != nil && *count > 0 {
		expiry := l.Text(l10n.KeyResetExpiryUnavailable)
		hint := expiry
		if !expires.IsZero() {
			expiry = accountResetExpiryText(l, expires)
			hint = expires.In(time.Local).Format("2006-01-02 15:04:05 MST")
		}
		dimText(c, expiry).MaxLines(2).Tooltip(hint)
	}
	c.After(time.Second)
}

func accountResetExpiryText(l l10n.Localizer, expires time.Time) string {
	return l.Text(l10n.KeyNextExpiry) + ": " + expires.In(time.Local).Format("2006-01-02 15:04")
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
		return runway.DurationLocalized(float64(minutes)*60, l.Language(), false)
	}
	return l.Text(l10n.KeyQuota)
}
