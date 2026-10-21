package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// providerView reuses Codex display widgets with the same typed Grok and
// Claude data.
func providerView(l l10n.Localizer, snap runway.Snapshot, displayRange store.CostRange) runway.CodexView {
	if snap.Provider == store.ProviderCursor {
		return runway.CodexView{Meters: snap.Cursor.Meters}
	}
	if snap.Provider == store.ProviderClaude {
		return claudeAsCodexView(l, snap.Claude, displayRange)
	}
	if snap.Provider != store.ProviderGrok {
		return snap.Codex
	}
	g := snap.Grok
	v := runway.CodexView{Meters: g.Meters, Heatmap: g.Heatmap, DisplayedCost: g.Cost, DisplayedRange: displayRange, Cost: g.Cost, ResetCredits: g.ResetCredits, ResetCreditSummary: g.ResetCreditSummary, RecentSessions: g.RecentSessions, Speed: g.Speed, SpeedError: g.SpeedError}
	if g.Cost != nil {
		d := runway.DisplayCost(l, *g.Cost, displayRange, "", time.Now())
		v.CostText, v.CostSubtitle, v.CostLines = d.Text, d.Subtitle, d.Lines
	}
	if g.ResetCredits != nil {
		v.CreditDetails = runway.CreditDetails(l, *g.ResetCredits)
	}
	return v
}

func claudeAsCodexView(l l10n.Localizer, g runway.ClaudeState, displayRange store.CostRange) runway.CodexView {
	v := runway.CodexView{Meters: g.Meters, Heatmap: g.Heatmap, DisplayedCost: g.Cost, DisplayedRange: displayRange, Cost: g.Cost, ResetCredits: g.ResetCredits, ResetCreditSummary: g.ResetCreditSummary, RecentSessions: g.RecentSessions, Speed: withoutTTFT(g.Speed), SpeedError: g.SpeedError}
	if g.Cost != nil {
		d := runway.DisplayCost(l, *g.Cost, displayRange, "", time.Now())
		v.CostText, v.CostSubtitle, v.CostLines = d.Text, d.Subtitle, d.Lines
		if claudePartialCost(*g.Cost) {
			v.CostLines = append(v.CostLines, runway.DetailLine{Value: l.Text(l10n.KeyClaudePartialUnpriced)})
		}
	}
	if g.ResetCredits != nil {
		v.CreditDetails = runway.CreditDetails(l, *g.ResetCredits)
	} else {
		// The module stays on the panel for every Claude account; without
		// reset grants it counts zero.
		empty := codexapi.ResetCreditsSnapshot{Credits: []codexapi.ResetCredit{}, UpdatedAt: g.UpdatedAt}
		sum := runway.SummarizeCredits(empty)
		v.ResetCredits, v.ResetCreditSummary = &empty, &sum
	}
	return v
}

func claudePartialCost(sum usage.Summary) bool {
	if sum.EstimatedUSD != nil && sum.Confidence != usage.ConfidenceTokensOnly && len(sum.Warnings) == 0 {
		return false
	}
	for _, w := range sum.Warnings {
		if strings.HasPrefix(w, "unknown-model:") || strings.HasPrefix(w, "fast-unpriced:") {
			return true
		}
	}
	return sum.Confidence == usage.ConfidenceTokensOnly && sum.Totals.TotalTokens > 0
}

// withoutTTFT clears time-to-first-token figures and marks the report so
// the views hide those columns: Claude logs have no first-token time.
func withoutTTFT(r *speed.Report) *speed.Report {
	if r == nil {
		return nil
	}
	out := *r
	out.NoTTFT = true
	out.Groups = append([]speed.Stat(nil), r.Groups...)
	for i := range out.Groups {
		out.Groups[i].Calls = 0
		out.Groups[i].TTFTP50 = 0
		out.Groups[i].TTFTP90 = 0
	}
	out.Sessions = append([]speed.SessionStat(nil), r.Sessions...)
	for i := range out.Sessions {
		out.Sessions[i].Groups = append([]speed.Stat(nil), r.Sessions[i].Groups...)
		for j := range out.Sessions[i].Groups {
			out.Sessions[i].Groups[j].Calls = 0
			out.Sessions[i].Groups[j].TTFTP50 = 0
			out.Sessions[i].Groups[j].TTFTP90 = 0
		}
	}
	return &out
}

// panelDetail draws the scrolling body of the reset credits, quota estimate
// and speed analysis pages. The accounts and API cost pages lay themselves out.
func panelDetail(c *ui.Context, l l10n.Localizer, m store.Module, v runway.CodexView) {
	switch m {
	case store.ModuleResetCredits:
		creditsDetail(c, l, v)
	case store.ModuleQuotaEstimate:
		estimateDetail(c, l, v)
	case store.ModuleSpeedAnalysis:
		speedDetail(c, l, v)
	}
}
