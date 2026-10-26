package runway

import (
	"time"

	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func exportWindow(w codexapi.RateWindow) export.Window {
	return export.Window{UsedPercent: w.UsedPercent, ResetsAt: timePtr(w.ResetsAt)}
}

func statusQuota(q codexapi.QuotaSnapshot) export.StatusQuota {
	in := export.QuotaInput{
		Primary: exportWindow(q.Primary), CreditsBalance: q.CreditsBalance, UpdatedAt: q.UpdatedAt,
	}
	if q.Plan != "" {
		p := q.Plan
		in.Plan = &p
	}
	if q.Secondary != nil {
		w := exportWindow(*q.Secondary)
		in.Secondary = &w
	}
	for _, a := range q.AdditionalWindows {
		in.Additional = append(in.Additional, export.NamedWindow{Name: a.Name, Window: exportWindow(a.Window)})
	}
	return export.NewStatusQuota(in)
}

func swiftDecimal(d *decimal.Decimal) *swiftjson.Decimal {
	if d == nil {
		return nil
	}
	v, err := swiftjson.ParseDecimal(d.String())
	if err != nil {
		return nil
	}
	return &v
}

func sessionTotals(t usage.Totals) sessions.Totals {
	return sessions.Totals{
		TotalTokens: int(t.TotalTokens), UncachedInputTokens: int(t.UncachedInputTokens),
		CachedInputTokens: int(t.CachedInputTokens), OutputTokens: int(t.OutputTokens),
		Turns: int(t.Turns), Threads: int(t.Threads),
	}
}

func breakdown(rows []usage.BreakdownRow) []export.CostBreakdownRow {
	out := make([]export.CostBreakdownRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, export.CostBreakdownRow{
			Name: r.Name, Totals: sessionTotals(r.Totals),
			EstimatedUSD: swiftDecimal(r.EstimatedUSD), UnpricedTokens: r.UnpricedTokens, RawCredits: r.RawCredits,
		})
	}
	return out
}

// exportCost converts a cost summary to the status file form.
func exportCost(c usage.Summary) export.CostSummary {
	daily := make([]export.CostDailyRow, 0, len(c.DailyRows))
	for _, r := range c.DailyRows {
		daily = append(daily, export.CostDailyRow{
			Date: r.Date, Totals: sessionTotals(r.Totals), EstimatedUSD: swiftDecimal(r.EstimatedUSD), UnpricedTokens: r.UnpricedTokens,
			RawCredits: r.RawCredits, CreditsReported: r.CreditsReported, TotalsReported: r.TotalsReported,
		})
	}
	warnings := append([]string{}, c.Warnings...)
	return export.CostSummary{
		Source: string(c.Source), Confidence: string(c.Confidence),
		Window:       export.Interval{Start: c.Window.Start, Duration: c.Window.End.Sub(c.Window.Start).Seconds()},
		EstimatedUSD: swiftDecimal(c.EstimatedUSD), UnpricedTokens: c.UnpricedTokens, Totals: sessionTotals(c.Totals),
		DailyRows: daily, ModelRows: breakdown(c.ModelRows), ProjectRows: breakdown(c.ProjectRows),
		ClientRows: breakdown(c.ClientRows), RawCredits: c.RawCredits, Warnings: warnings,
		PricingVersion: c.PricingVersion, CalculatedAt: c.CalculatedAt,
	}
}

// StatusSnapshotLocked is the content of status.json for the current state.
func (s *Service) statusSnapshotLocked(now time.Time) export.StatusSnapshot {
	out := export.StatusSnapshot{GeneratedAt: now}
	if s.quota != nil {
		q := statusQuota(*s.quota)
		out.Quota = &q
	}
	if s.cost != nil {
		c := exportCost(*s.cost)
		out.Cost = &c
	}
	items := append([]sessions.Item{}, s.recent...)
	out.Sessions = &sessions.Summary{Items: items}
	return out
}

// exportStatusLocked writes status.json when the preference is on. The file
// holds the quota, the cost and the recent sessions, never credentials.
func (s *Service) exportStatusLocked() {
	if !s.prefs.ExportsStatusJSON || s.opts.StatusPath == "" {
		return
	}
	if err := (export.StatusExporter{Path: s.opts.StatusPath}).Save(s.statusSnapshotLocked(s.now())); err != nil {
		s.lastErr = err.Error()
	}
}
