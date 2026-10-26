package app

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

type settingsPricing struct {
	provider                                       store.Provider
	syncSettings                                   bool
	customOpen                                     bool
	taskAttempt, taskSuccess                       time.Time
	cacheSearch                                    string
	cached                                         []PricingRow
	cacheSource                                    *PricingRow
	cacheKey                                       string
	cacheLength                                    int
	counts                                         [len(pricingFilters)]int
	providerCounts                                 map[string]int
	search, filter, sort, open, custom, validation string
	page                                           int
	task                                           Task
	generation                                     int
	cancel                                         context.CancelFunc
}

func (u *settingsUI) pricingState() *settingsPricing {
	if u.pricing == nil {
		u.pricing = &settingsPricing{provider: store.ProviderCodex}
	}
	return u.pricing
}
func (u *settingsUI) pricingActions() PricingActions {
	if u.opts.Pricing != nil {
		return u.opts.Pricing()
	}
	return PricingActions{}
}
func pricingValue(v *decimal.Decimal) string {
	if v == nil {
		return "—"
	}
	return "$" + v.String()
}
func pricingLess(a, b *decimal.Decimal) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.Cmp(*b) < 0
}

// pricingFilters are the state filters in their order, "" showing all.
var pricingFilters = [...]string{"", "available", "reference", "incomplete", "builtin"}

func pricingMatches(r PricingRow, filter string) bool {
	switch filter {
	case "available":
		return r.Available
	case "reference":
		return !r.Available
	case "incomplete":
		return r.Input == nil || r.Output == nil
	case "builtin":
		return r.Builtin
	}
	return true
}

// filtered also counts the rows of each state filter under the search.
func (s *settingsPricing) filtered(rows []PricingRow) []PricingRow {
	out := make([]PricingRow, 0, len(rows))
	search := strings.ToLower(strings.TrimSpace(s.search))
	s.counts = [len(pricingFilters)]int{}
	s.providerCounts = map[string]int{}
	for _, r := range rows {
		s.providerCounts[r.ProviderID]++
		if r.ProviderID != pricingProviderID(s.provider) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(r.ID+" "+r.Name), search) {
			continue
		}
		for i, f := range pricingFilters {
			if pricingMatches(r, f) {
				s.counts[i]++
			}
		}
		if pricingMatches(r, s.filter) {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch s.sort {
		case "input":
			if pricingValue(a.Input) != pricingValue(b.Input) {
				return pricingLess(a.Input, b.Input)
			}
		case "output":
			if pricingValue(a.Output) != pricingValue(b.Output) {
				return pricingLess(a.Output, b.Output)
			}
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ProviderID+"/"+a.ID < b.ProviderID+"/"+b.ID
	})
	return out
}
func (s *settingsPricing) pageRows(rows []PricingRow) []PricingRow {
	s.page = max(0, min(s.page, max(0, (len(rows)-1)/100)))
	start := min(len(rows), s.page*100)
	return rows[start:min(start+100, len(rows))]
}
func (u *settingsUI) refreshPricing(a PricingActions) {
	s := u.pricingState()
	if a.Refresh == nil || s.task.Running {
		return
	}
	s.generation++
	gen := s.generation
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.task = Task{Running: true}
	go func() {
		err := a.Refresh(ctx)
		resultSnapshot := PricingSnapshot{}
		if err != nil && a.Snapshot != nil {
			resultSnapshot = a.Snapshot()
		}
		u.post(func() {
			if gen != s.generation || ctx.Err() != nil {
				return
			}
			s.task = Task{Err: err}
			if err != nil {
				s.taskAttempt, s.taskSuccess = resultSnapshot.LastAttempt, resultSnapshot.LastSuccess
			}
			s.cancel = nil
			cancel()
		})
	}()

}
func (u *settingsUI) cancelPricingTasks() {
	if u.pricing == nil {
		return
	}
	u.pricing.generation++
	if u.pricing.cancel != nil {
		u.pricing.cancel()
		u.pricing.cancel = nil
	}
}

// cachedList rebuilds the sort/filter result only when immutable catalog rows
// or controls change, so drawing a frame never sorts the full catalog again.
func (s *settingsPricing) cachedList(snap PricingSnapshot) []PricingRow {
	if s.cacheSearch != s.search {
		s.page = 0
		s.cacheSearch = s.search
	}
	var first *PricingRow
	if len(snap.Rows) > 0 {
		first = &snap.Rows[0]
	}
	key := string(s.provider) + "\x00" + snap.DataVersion + "\x00" + s.search + "\x00" + s.filter + "\x00" + s.sort
	if s.cacheSource != first || s.cacheLength != len(snap.Rows) || s.cacheKey != key {
		s.cacheSource, s.cacheLength, s.cacheKey = first, len(snap.Rows), key
		s.cached = s.filtered(snap.Rows)
	}
	return s.cached
}

// syncError retains an action-only failure until a later snapshot result
// supersedes it; identities come from the manager rather than wall-clock time.
func (s *settingsPricing) syncError(snap PricingSnapshot) string {
	if s.task.Err != nil && (!s.taskAttempt.Equal(snap.LastAttempt) || !s.taskSuccess.Equal(snap.LastSuccess)) {
		s.task.Err = nil
	}
	if snap.Error != "" {
		return snap.Error
	}
	if s.task.Err != nil {
		return s.task.Err.Error()
	}
	return ""
}

func pricingProviderID(provider store.Provider) string {
	switch provider {
	case store.ProviderClaude:
		return "anthropic"
	case store.ProviderGrok:
		return "xai"
	}
	return "openai"
}
func (s *settingsPricing) switchProvider(provider store.Provider) {
	s.provider = provider
	s.search, s.filter, s.sort, s.open = "", "", "", ""
	s.page = 0
}

func (s *settingsPricing) detailRow(snapshot PricingSnapshot) (PricingRow, bool) {
	if s.open == "" {
		return PricingRow{}, false
	}
	for _, row := range snapshot.Rows {
		if row.ProviderID == pricingProviderID(s.provider) && row.ProviderID+"/"+row.ID == s.open {
			return row, true
		}
	}
	return PricingRow{}, false
}
