package usage

import (
	"sort"
	"time"
)

// buildSummary aggregates index events into a Summary for window. Models the
// price book does not know are listed in warnings and left out of the cost, so
// a partly priced window never claims an exact total.
func buildSummary(events []indexedEvent, window Window, calculatedAt time.Time, book PriceBook, warnings []string) (Summary, error) {
	g, err := groupEvents(events)
	if err != nil {
		return Summary{}, err
	}
	return summaryFromGroups(g, window, calculatedAt, book, nil, warnings)
}

// summaryFromGroups turns grouped totals into a Summary. unknown lists the
// models to report as "unknown-model:<name>" warnings, ahead of warnings.
func summaryFromGroups(g eventGroups, window Window, calculatedAt time.Time, book PriceBook, unknown, warnings []string) (Summary, error) {
	dayTotals := make([]Totals, 0, len(g.byDay))
	for _, t := range g.byDay {
		dayTotals = append(dayTotals, t)
	}
	totals, err := sumTotals(dayTotals)
	if err != nil {
		return Summary{}, err
	}
	models := sortedKeys(g.byModel)
	priced := priceGroups(g, book)
	estimated := priced.total.usd()
	confidence := ConfidencePriced
	switch {
	case totals.TotalTokens == 0:
		confidence = ConfidenceUnavailable
	case estimated == nil:
		confidence = ConfidenceTokensOnly
	case priced.total.unpriced > 0:
		confidence = ConfidencePartiallyPriced
	}
	source := SourceUnavailable
	if totals.TotalTokens > 0 {
		source = SourceLocalSessions
	}
	projectRows, err := projectRows(g.byProjectModel, priced.projects)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{
		Source:         source,
		Confidence:     confidence,
		Window:         window,
		EstimatedUSD:   estimated,
		UnpricedTokens: priced.total.unpriced,
		Totals:         totals,
		DailyRows:      dailyRows(g, priced.days),
		ModelRows:      modelRows(g.byModel, models, priced.models),
		ProjectRows:    projectRows,
		ClientRows:     []BreakdownRow{},
		Warnings:       []string{},
		PricingVersion: book.Version(),
		CalculatedAt:   calculatedAt,
	}
	for _, warning := range sortedKeys(priced.warnings) {
		out.Warnings = append(out.Warnings, warning)
	}
	out.Warnings = append(out.Warnings, warnings...)
	return out, nil
}

func newEventGroups() eventGroups {
	return eventGroups{
		byModel:        map[string]Totals{},
		byProjectModel: map[string]map[string]Totals{},
		byDay:          map[string]Totals{},
	}
}

// add retains per-event usage and accumulates model, project and day totals.
func (g *eventGroups) add(model, project, day string, t Totals) error {
	g.events = append(g.events, groupedUsage{model, project, day, t})
	if err := addInto(g.byModel, model, t); err != nil {
		return err
	}
	if err := addNested(g.byProjectModel, project, model, t); err != nil {
		return err
	}
	if err := addInto(g.byDay, day, t); err != nil {
		return err
	}
	return nil
}

type groupedUsage struct {
	model, project, day string
	totals              Totals
}
type eventGroups struct {
	events         []groupedUsage
	byModel        map[string]Totals
	byProjectModel map[string]map[string]Totals
	byDay          map[string]Totals
}

func addInto(m map[string]Totals, key string, t Totals) error {
	sum, err := m[key].Add(t)
	if err != nil {
		return err
	}
	m[key] = sum
	return nil
}

func addNested(m map[string]map[string]Totals, outer, inner string, t Totals) error {
	if m[outer] == nil {
		m[outer] = map[string]Totals{}
	}
	return addInto(m[outer], inner, t)
}

func groupEvents(events []indexedEvent) (eventGroups, error) {
	g := newEventGroups()
	for _, e := range events {
		input, err := checkedAdd(e.Uncached, e.Cached, "input tokens")
		if err != nil {
			return g, err
		}
		total, err := checkedAdd(input, e.Output, "total tokens")
		if err != nil {
			return g, err
		}
		t := Totals{TotalTokens: total, UncachedInputTokens: e.Uncached, CachedInputTokens: e.Cached, OutputTokens: e.Output, Turns: e.Turns}
		if err := g.add(e.Model, e.Project, e.DayKey, t); err != nil {
			return g, err
		}
	}
	return g, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func modelRows(byModel map[string]Totals, models []string, prices map[string]pricedTotal) []BreakdownRow {
	rows := make([]BreakdownRow, 0, len(models))
	for _, m := range models {
		t := byModel[m]
		c := prices[m]
		row := BreakdownRow{Name: m, Totals: t, EstimatedUSD: c.usd(), UnpricedTokens: c.unpriced}
		rows = append(rows, row)
	}
	return rows
}

func projectRows(byProject map[string]map[string]Totals, prices map[string]pricedTotal) ([]BreakdownRow, error) {
	rows := make([]BreakdownRow, 0, len(byProject))
	for _, project := range sortedKeys(byProject) {
		byModel := byProject[project]
		all := make([]Totals, 0, len(byModel))
		for _, m := range sortedKeys(byModel) {
			all = append(all, byModel[m])
		}
		t, err := sumTotals(all)
		if err != nil {
			return nil, err
		}
		c := prices[project]
		rows = append(rows, BreakdownRow{Name: project, Totals: t, EstimatedUSD: c.usd(), UnpricedTokens: c.unpriced})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Totals.TotalTokens != rows[j].Totals.TotalTokens {
			return rows[i].Totals.TotalTokens > rows[j].Totals.TotalTokens
		}
		return rows[i].Name < rows[j].Name
	})
	return rows, nil
}

func dailyRows(g eventGroups, prices map[string]pricedTotal) []DailyRow {
	days := sortedKeys(g.byDay)
	rows := make([]DailyRow, 0, len(days))
	for _, day := range days {
		c := prices[day]
		rows = append(rows, DailyRow{Date: day, Totals: g.byDay[day], EstimatedUSD: c.usd(), UnpricedTokens: c.unpriced})
	}
	return rows
}
