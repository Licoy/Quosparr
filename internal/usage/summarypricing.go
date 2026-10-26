package usage

import "github.com/licoy/quosparr/internal/usage/decimal"

type pricedTotal struct {
	amount   decimal.Decimal
	priced   bool
	unpriced int64
}

func (p pricedTotal) usd() *decimal.Decimal {
	if !p.priced {
		return nil
	}
	d := p.amount
	return &d
}
func (p pricedTotal) add(cost decimal.Decimal, ok bool, t Totals) pricedTotal {
	if !t.HasTokenParts() {
		return p
	}
	if ok {
		p.priced = true
		p.amount = p.amount.Add(cost)
	} else {
		p.unpriced += t.TotalTokens
	}
	return p
}

type pricedGroups struct {
	total                  pricedTotal
	models, projects, days map[string]pricedTotal
	warnings               map[string]bool
}

func priceGroups(g eventGroups, book PriceBook) pricedGroups {
	p := pricedGroups{models: map[string]pricedTotal{}, projects: map[string]pricedTotal{}, days: map[string]pricedTotal{}, warnings: map[string]bool{}}
	for _, e := range g.events {
		c, ok, source := book.CostWithSource(e.model, e.totals)
		if e.totals.HasTokenParts() {
			if !ok {
				p.warnings[source+":"+e.model] = true
			} else if book.snapshot != nil {
				p.warnings["price-source:"+source+":"+e.model] = true
			}
		}
		p.total = p.total.add(c, ok, e.totals)
		p.models[e.model] = p.models[e.model].add(c, ok, e.totals)
		p.projects[e.project] = p.projects[e.project].add(c, ok, e.totals)
		p.days[e.day] = p.days[e.day].add(c, ok, e.totals)
	}
	return p
}
