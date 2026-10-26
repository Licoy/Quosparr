package app

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
)

// Number formats of QuotaEstimateFormatting.
func estCredits(v float64) string      { return fmt.Sprintf("%.1f", v) }
func estTableCredits(v float64) string { return fmt.Sprintf("%.3f", v) }
func estUSD(v float64) string          { return fmt.Sprintf("$%.2f", v) }
func estShowUSD(v float64) bool        { return !math.IsInf(v, 0) && !math.IsNaN(v) && math.Abs(v) >= 0.005 }
func estApproxUSD(v float64) string    { return "(≈" + estUSD(v) + ")" }
func estSignedPercent(v float64) string {
	return fmt.Sprintf("%+.0f%%", math.Round(v))
}

func estPercent(v float64) string {
	if v == math.Round(v) {
		return fmt.Sprintf("%.0f%%", v)
	}
	return fmt.Sprintf("%.1f%%", v)
}

// panelEstimate is the quota estimate summary: the weekly allowance, what
// this week used, the data age and a row to the details page.
func panelEstimate(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState) {
	v := snap.Codex
	busy := sectionRefreshing(snap, runway.SectionQuotaEstimate)
	title := l.Text(l10n.KeyQuotaEstimate)
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, sectionSpec{
			title: title, refreshing: busy, refresh: sectionRefresh(acts, runway.SectionQuotaEstimate),
			info: &sectionInfo{id: "quota-estimate-info", help: title, body: popoverBody(c, l, st, title, l.Text(l10n.KeyQuotaEstimateInfo))},
		})
		switch {
		case v.Estimate != nil:
			estimateSummary(c, l, st, *v.Estimate, v.EstimateError)
		case v.EstimateError != "":
			ui.Text(c, v.EstimateError).FontSize(fsCallout).TextColor(sys(c.Theme(), sysRed))
		default:
			sectionEmpty(c, l, busy)
		}
	})
}

func estimateSummary(c *ui.Context, l l10n.Localizer, st *panelState, s estimate.Snapshot, errText string) {
	t := c.Theme()
	ui.RichText(c, estimateHeroSpans(t, l, s)...)
	ui.RichText(c, estimateSubtitleSpans(t, l, s)...)
	ui.Column(c).Gap(6).Children(func() {
		dimText(c, runway.EstimateDataTimeText(l, s))
		if msg := runway.EstimateRefreshErrorText(l, errText); msg != "" {
			ui.Text(c, msg).FontSize(fsCaption).TextColor(sys(t, sysRed))
		}
	})
	if disclosureRow(c, nil, l.Text(l10n.KeyQuotaEstimateThisWeek), true).Clicked() {
		st.detail = store.ModuleQuotaEstimate
	}
}

func estimateHeroSpans(t *ui.Theme, l l10n.Localizer, s estimate.Snapshot) []ui.Span {
	hero := ui.Span{Size: fsTitle3, Weight: 600, Features: "tnum"}
	if s.EstimatedCredits == nil {
		hero.Text = "—"
		return []ui.Span{hero}
	}
	hero.Text = estCredits(*s.EstimatedCredits) + " " + l.Text(l10n.KeyQuotaEstimateCredits)
	spans := []ui.Span{hero}
	if s.EstimatedUSD != nil && estShowUSD(*s.EstimatedUSD) {
		usd := hero
		usd.Text, usd.Color = " "+estApproxUSD(*s.EstimatedUSD), sys(t, sysBlue)
		spans = append(spans, usd)
	}
	return spans
}

func estimateSubtitleSpans(t *ui.Theme, l l10n.Localizer, s estimate.Snapshot) []ui.Span {
	secondary := inkSecondary(t)
	span := func(text string, color ui.Color) ui.Span {
		return ui.Span{Text: text, Size: fsCaption, Color: color}
	}
	if s.UnavailableReason != nil {
		return []ui.Span{span(runway.EstimateUnavailableText(l, *s.UnavailableReason), secondary)}
	}
	unit := l.Text(l10n.KeyQuotaEstimateCredits)
	spans := []ui.Span{span(l.Text(l10n.KeyQuotaEstimateThisWeekUsed)+" "+estCredits(s.UsedCredits)+" "+unit, secondary)}
	if estShowUSD(s.UsedUSD) {
		spans = append(spans, span(" "+estApproxUSD(s.UsedUSD), sys(t, sysBlue)))
	}
	if s.CanExtrapolate {
		spans = append(spans, span(" · "+estPercent(s.UsedPercent), secondary))
	}
	if text, color, ok := estimateChange(t, l, s); ok {
		spans = append(spans, span(" · "+text, color))
	}
	return spans
}

// estimateChange is the "vs last cycle" note; similar cycles show none.
func estimateChange(t *ui.Theme, l l10n.Localizer, s estimate.Snapshot) (string, ui.Color, bool) {
	if s.ChangeKind == nil || s.ChangePercent == nil {
		return "", ui.Color{}, false
	}
	delta := estSignedPercent(*s.ChangePercent)
	switch *s.ChangeKind {
	case estimate.ChangeDecreased:
		return l.Text(l10n.KeyQuotaEstimateVsLast) + " " + delta + " · " + l.Text(l10n.KeyQuotaEstimateCut), sys(t, sysOrange), true
	case estimate.ChangeIncreased:
		return l.Text(l10n.KeyQuotaEstimateVsLast) + " " + delta + " · " + l.Text(l10n.KeyQuotaEstimateIncreased), sys(t, sysGreen), true
	}
	return "", ui.Color{}, false
}

// panelCredits is the reset credits summary.
func panelCredits(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState) {
	sum := snap.Codex.ResetCreditSummary
	busy := sectionRefreshing(snap, runway.SectionResetCredits)
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, sectionSpec{
			title: l.Text(l10n.KeyResetCredits), refreshing: busy, refresh: sectionRefresh(acts, runway.SectionResetCredits),
		})
		if sum == nil || busy && snap.Provider == store.ProviderClaude && snap.Claude.UpdatedAt.IsZero() && snap.Claude.ResetCredits == nil {
			sectionEmpty(c, l, busy)
			return
		}
		creditsHero(c, l, *sum)
		creditsExpiryRow(c, l, *sum)
		if disclosureRow(c, nil, strconv.Itoa(sum.AvailableCount)+" "+l.Text(l10n.KeyAvailableResets), true).Clicked() {
			st.detail = store.ModuleResetCredits
		}
	})
}

func creditsHero(c *ui.Context, l l10n.Localizer, sum runway.ResetCreditSummary) {
	ui.Row(c).Gap(6).AlignItems(ui.End).Children(func() {
		ui.Text(c, strconv.Itoa(sum.AvailableCount)).FontSize(fsTitle3).FontWeight(600).FontFeatures("tnum")
		calloutDim(c, l.Text(l10n.KeyAvailable)+" / "+strconv.Itoa(sum.TotalCount)+" "+l.Text(l10n.KeyTotal))
	})
}

// creditsExpiryRow is the next / latest expiry pair, each with the exact
// expiry time as its tooltip.
func creditsExpiryRow(c *ui.Context, l l10n.Localizer, sum runway.ResetCreditSummary) {
	t := c.Theme()
	lang := l.Language()
	remaining := func(at time.Time, secs float64) *float64 {
		if at.IsZero() {
			return nil
		}
		return &secs
	}
	next := l.Text(l10n.KeyNextExpiry) + ": " + runway.ExpiryDuration(remaining(sum.NextExpiry, sum.NextExpiryRemaining), lang)
	latest := l.Text(l10n.KeyLatestExpiry) + ": " + runway.ExpiryDuration(remaining(sum.LatestExpiry, sum.LatestExpiryRemaining), lang)
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, next).FontSize(fsCaption).FontFeatures("tnum").TextColor(inkSecondary(t)).Grow(1).Basis(0).
			Tooltip(runway.ExpiryHelp(l, sum.NextExpiry, sum.UpdatedAt, nil))
		ui.Text(c, latest).FontSize(fsCaption).FontFeatures("tnum").TextColor(inkSecondary(t)).Grow(1).Basis(0).TextAlign(ui.End).
			Tooltip(runway.ExpiryHelp(l, sum.LatestExpiry, sum.UpdatedAt, nil))
	})
}

// panelCost is the API cost summary.
func panelCost(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState) {
	v := snap.Codex
	busy := sectionRefreshing(snap, runway.SectionAPICost)
	t := c.Theme()
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, sectionSpec{
			title: l.Text(l10n.KeyApiCost), refreshing: busy, refresh: sectionRefresh(acts, runway.SectionAPICost),
		})
		ui.Column(c).Gap(3).Children(func() {
			if busy && v.DisplayedCost == nil && len(v.CostLines) == 0 || v.CostText == "" {
				sectionEmpty(c, l, busy)
			} else {
				ui.Text(c, v.CostText).FontSize(fsCallout).TextColor(inkSecondary(t))
			}
			if v.CostSubtitle != "" {
				ui.Text(c, v.CostSubtitle).FontSize(fsCaption).TextColor(inkTertiary(t))
			}
		})
		if disclosureRow(c, nil, l.Text(l10n.KeyShowDetails), true).Clicked() {
			st.detail = store.ModuleAPICost
		}
	})
}

// panelRepair is the session repair summary with its confirm popover.
func panelRepair(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState) {
	v := snap.Codex
	busy := sectionRefreshing(snap, runway.SectionSessionRepair)
	t := c.Theme()
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, sectionSpec{
			title: l.Text(l10n.KeySessionRepair), refreshing: busy, refresh: sectionRefresh(acts, runway.SectionSessionRepair),
		})
		if busy && v.SessionReport == nil && len(v.SessionLines) == 0 {
			sectionEmpty(c, l, true)
		} else {
			ui.Text(c, v.SessionText).FontSize(fsCaption).TextColor(inkSecondary(t))
		}
		repairRow(c, l, acts, st)
	})
}

// repairRow is the "Repair index" row; it asks before it rewrites anything.
func repairRow(c *ui.Context, l l10n.Localizer, acts PanelActions, st *panelState) {
	row := disclosureRow(c, icRepair, l.Text(l10n.KeyRepairIndex), false)
	row.Disabled(acts.RepairSessions == nil)
	open := st.info == "repair-confirm"
	if row.Clicked() {
		st.info, open = "repair-confirm", true
	}
	panelPopover(c, row, &open, func() {
		ui.Column(c).Gap(14).Width(280).Padding(4).Children(func() {
			ui.Text(c, l.Text(l10n.KeyRepairConfirmTitle)).FontSize(fsTitle3).FontWeight(600)
			calloutDim(c, l.Text(l10n.KeyRepairConfirmMessage))
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c).Grow(1)
				if PillButton(c, nil, l.Text(l10n.KeyCancel), false).Clicked() {
					st.info = ""
				}
				if PillButton(c, nil, l.Text(l10n.KeyRepair), acts.RepairSessions == nil).Clicked() {
					st.info = ""
					acts.RepairSessions()
				}
			})
		})
	})
	if !open && st.info == "repair-confirm" {
		st.info = ""
	}
}

// panelRecent is the recent sessions card: at most five rows with a state dot.
func panelRecent(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, grok bool, acts PanelActions, first *bool, st *panelState) {
	busy := sectionRefreshing(snap, runway.SectionRecentSessions)
	items := snap.Codex.RecentSessions
	refresh := sectionRefresh(acts, runway.SectionRecentSessions)
	if grok && snap.Provider == store.ProviderClaude {
		items = snap.Claude.RecentSessions
		refresh = sectionRefresh(acts, runway.SectionAPICost)
	} else if grok {
		items = snap.Grok.RecentSessions
		refresh = sectionRefresh(acts, runway.SectionAPICost)
	}
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, sectionSpec{title: l.Text(l10n.KeyRecentSessions), refreshing: busy, refresh: refresh})
		if len(items) == 0 {
			sectionEmpty(c, l, busy)
			return
		}
		cardColumn(c, surfaceSunkenKind, radiusCard).Children(func() {
			for i, it := range items[:min(5, len(items))] {
				recentRow(c, l, it, grok, i == 0)
			}
		})
	})
}

func recentRow(c *ui.Context, l l10n.Localizer, it sessions.Item, grok, first bool) {
	t := c.Theme()
	ui.Column(c).Children(func() {
		tableRowRule(c, first)
		ui.Row(c).Gap(8).Padding(7, 10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(sessionDot(t, it.State, grok))
			ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
				title := it.Title
				if title == "" {
					title = l.Text(l10n.KeyUnknown)
				}
				ui.Text(c, title).FontSize(fsCallout).FontWeight(500).SingleLine().Ellipsis("…")
				ui.Text(c, recentSubtitle(l, it, grok)).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine().Ellipsis("…")
			})
			ui.Text(c, recentCostLocalized(l, it)).FontSize(fsCaption).FontFeatures("tnum").TextColor(inkSecondary(t)).SingleLine()
		})
	})
}

func sessionDot(t *ui.Theme, state sessions.State, grok bool) ui.Color {
	switch {
	case grok:
		return sys(t, sysGreen)
	case state == sessions.StateNeedsAttention:
		return sys(t, sysOrange)
	case state == sessions.StateFailed:
		return sys(t, sysRed)
	}
	return sys(t, sysGreen)
}

func recentSubtitle(l l10n.Localizer, it sessions.Item, grok bool) string {
	tokens := tokenCompact(it.Totals.TotalTokens) + " " + l.Text(l10n.KeyTokens)
	if grok {
		model := l.Text(l10n.KeyUnknown)
		if it.Model != nil {
			model = *it.Model
		}
		return it.ProjectName + " · " + model + " · " + tokens
	}
	state := l.Text(l10n.KeyRecent)
	switch it.State {
	case sessions.StateNeedsAttention:
		state = l.Text(l10n.KeyNeedsAttention)
	case sessions.StateFailed:
		state = l.Text(l10n.KeyFailed)
	}
	return it.ProjectName + " · " + state + " · " + tokens
}

func recentCost(it sessions.Item) string { return recentCostLocalized(l10n.New(l10n.English), it) }
func recentCostLocalized(l l10n.Localizer, it sessions.Item) string {
	if it.EstimatedUSD == nil {
		return "—"
	}
	out := fmt.Sprintf("$%.4f", it.EstimatedUSD.Float64())
	if it.UnpricedTokens > 0 {
		out += " · " + l.Pricing("partial")
	}
	return out
}

// tokenCompact is the "1.23K" / "4.56M" style of the session rows.
func tokenCompact(v int) string { return estimate.CompactTokens(v) }
