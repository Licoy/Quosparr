package gemini

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// SessionUsageTotals is the token and cost sum of local Antigravity model
// calls. InputTokens is the whole prompt: uncached input, cache reads and
// cache writes. OutputTokens includes ReasoningTokens. EstimatedUSD is the
// API-equivalent cost of the priced calls (nil when none was priced).
type SessionUsageTotals struct {
	TotalTokens      int64
	InputTokens      int64
	OutputTokens     int64
	CachedReadTokens int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	ModelCalls       int64
	EstimatedUSD     *decimal.Decimal
	UnpricedTokens   int64
	unpricedReasons  []string
}

// HasData reports whether any token or cost was counted.
func (t SessionUsageTotals) HasData() bool {
	return t.TotalTokens > 0 || t.ModelCalls > 0 || (t.EstimatedUSD != nil && t.EstimatedUSD.Sign() > 0)
}

// APITotals is the usage.Totals view: cache reads are the cached input, and
// a conversation counts once as a thread when any call was counted.
func (t SessionUsageTotals) APITotals() usage.Totals {
	threads := int64(0)
	if t.ModelCalls > 0 {
		threads = 1
	}
	cached := max(0, t.CachedReadTokens)
	return usage.Totals{
		TotalTokens:         t.TotalTokens,
		UncachedInputTokens: max(0, t.InputTokens-cached),
		CachedInputTokens:   cached,
		OutputTokens:        t.OutputTokens,
		Turns:               t.ModelCalls,
		Threads:             threads,
	}
}

// Merge adds o to t.
func (t *SessionUsageTotals) Merge(o SessionUsageTotals) {
	t.TotalTokens += o.TotalTokens
	t.InputTokens += o.InputTokens
	t.OutputTokens += o.OutputTokens
	t.CachedReadTokens += o.CachedReadTokens
	t.CacheWriteTokens += o.CacheWriteTokens
	t.ReasoningTokens += o.ReasoningTokens
	t.ModelCalls += o.ModelCalls
	t.UnpricedTokens += o.UnpricedTokens
	t.unpricedReasons = append(t.unpricedReasons, o.unpricedReasons...)
	switch {
	case t.EstimatedUSD != nil && o.EstimatedUSD != nil:
		sum := t.EstimatedUSD.Add(*o.EstimatedUSD)
		t.EstimatedUSD = &sum
	case o.EstimatedUSD != nil:
		v := *o.EstimatedUSD
		t.EstimatedUSD = &v
	}
}

// SessionModelRow is the usage of one model.
type SessionModelRow struct {
	Model  string
	Totals SessionUsageTotals
}

// SessionItem is one recent conversation. Totals count only the part of the
// conversation inside the cost window.
type SessionItem struct {
	ID           string
	Title        string
	ProjectName  string
	CWD          *string
	Model        *string
	Client       string // SessionClientApp or SessionClientCLI
	UpdatedAt    time.Time
	MessageCount int64 // conversation steps
	Totals       SessionUsageTotals
}

// SessionUsageSummary is the local Antigravity usage in the cost window.
type SessionUsageSummary struct {
	Totals       SessionUsageTotals
	SessionCount int
	// Unreadable counts conversations that could not be read: damaged
	// databases and the older one-file .pb conversations.
	Unreadable  int
	ModelRows   []SessionModelRow
	RecentItems []SessionItem
	// DailyTokens maps a local calendar day (yyyy-MM-dd) to its total tokens,
	// for every day found, window or not.
	DailyTokens  map[string]int64
	CostSummary  usage.Summary
	CalculatedAt time.Time
	SourceLabel  string
}

// SessionScanner reads the conversations of the Antigravity desktop App
// (<GeminiHome>/antigravity) and CLI (<GeminiHome>/antigravity-cli): each
// conversations/<id>.db and the conversation_summaries.db beside it. The
// databases are read from private copies, never opened in place.
type SessionScanner struct {
	// GeminiHome is the Antigravity data directory (normally ~/.gemini).
	GeminiHome string
	// Location decides the calendar days; nil means time.Local.
	Location *time.Location
	// Cache reuses the parse of unchanged databases.
	Cache *SessionCache
	// PricingSnapshot is captured once for each complete scan.
	PricingSnapshot func() pricing.Snapshot
	// ClientRunning reports whether the client owning a data root (one of
	// SessionRoots) is running. DeleteManaged refuses to touch a root unless
	// it reports false; nil refuses every deletion.
	ClientRunning func(root string) bool
	// BackupDir receives a copy of each summaries database DeleteManaged
	// edits.
	BackupDir string
}

// SessionScanOptions selects what Scan returns.
type SessionScanOptions struct {
	// RecentLimit is the number of recent conversations listed (0 for none).
	RecentLimit int
	// SessionLimit caps how many of the newest conversations are read; it is
	// raised to RecentLimit when smaller.
	SessionLimit int
	// CostWindow is the half-open range [Start, End) of days to total. The
	// zero value means January 1st of Now's year up to Now.
	CostWindow usage.Window
	Now        time.Time
}

// DefaultSessionScanOptions mirrors the app defaults: five recent
// conversations, up to 300 read, year to date.
func DefaultSessionScanOptions(now time.Time) SessionScanOptions {
	return SessionScanOptions{RecentLimit: 5, SessionLimit: 300, Now: now}
}

func (s SessionScanner) loc() *time.Location {
	if s.Location != nil {
		return s.Location
	}
	return time.Local
}

func (s SessionScanner) pricingSnapshot() pricing.Snapshot {
	if s.PricingSnapshot != nil {
		return s.PricingSnapshot()
	}
	return pricing.Snapshot{}
}

// ScanCost totals only the API-equivalent cost of a window (up to 400
// conversations).
func (s SessionScanner) ScanCost(ctx context.Context, window usage.Window, now time.Time) (usage.Summary, error) {
	r, err := s.Scan(ctx, SessionScanOptions{SessionLimit: 400, CostWindow: window, Now: now})
	return r.CostSummary, err
}

// sessionParsed is one conversation priced and split by local day.
type sessionParsed struct {
	byDay      map[string]SessionUsageTotals
	byDayModel map[string]map[string]SessionUsageTotals
}

// Scan reads the newest conversations and totals them. Missing roots are an
// empty result and a damaged database is counted as unreadable; only a
// cancelled context or an unreadable conversations folder is an error.
func (s SessionScanner) Scan(ctx context.Context, opts SessionScanOptions) (SessionUsageSummary, error) {
	snapshot := s.pricingSnapshot()
	loc := s.loc()
	recentLimit := max(0, opts.RecentLimit)
	limit := max(recentLimit, opts.SessionLimit)
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	window := opts.CostWindow
	if window.Start.IsZero() && window.End.IsZero() {
		window = usage.Window{Start: time.Date(now.In(loc).Year(), 1, 1, 0, 0, 0, 0, loc), End: now}
	}
	entries, unreadable, err := s.sessionList(ctx)
	if err != nil {
		return SessionUsageSummary{}, err
	}
	listed := map[string]bool{}
	for _, entry := range entries {
		listed[entry.path] = true
	}
	s.Cache.retain(listed)
	entries = entries[:min(len(entries), limit)]

	var agg SessionUsageTotals
	byModel, byProject, byDay, byClient := map[string]SessionUsageTotals{}, map[string]SessionUsageTotals{}, map[string]SessionUsageTotals{}, map[string]SessionUsageTotals{}
	dailyTokens := map[string]int64{}
	items := []SessionItem{}
	count := 0
	for _, entry := range entries {
		conv, err := s.sessionConversationOf(ctx, entry)
		if err != nil {
			if ctx.Err() != nil {
				return SessionUsageSummary{}, ctx.Err()
			}
			unreadable++
			continue
		}
		count++
		parsed := sessionPrice(conv, entry.updatedAt(), loc, snapshot)
		for day, t := range parsed.byDay {
			dailyTokens[day] += t.TotalTokens
		}
		matched := sessionMergeWindow(parsed, window, loc, byModel, byDay)
		if matched.HasData() {
			agg.Merge(matched)
			sessionAdd(byProject, entry.projectName(), matched)
			sessionAdd(byClient, entry.root.client, matched)
		}
		if len(items) < recentLimit {
			items = append(items, sessionItemOf(entry, conv, matched))
		}
	}
	return SessionUsageSummary{
		Totals:       agg,
		SessionCount: count,
		Unreadable:   unreadable,
		ModelRows:    sessionSortedModels(byModel),
		RecentItems:  items,
		DailyTokens:  dailyTokens,
		CostSummary:  sessionCostSummary(agg, byModel, byProject, byDay, byClient, window, now, sessionPricingVersion(snapshot)),
		CalculatedAt: now,
		SourceLabel:  "local-sessions",
	}, nil
}

func sessionItemOf(entry sessionEntry, conv sessionConversation, totals SessionUsageTotals) SessionItem {
	item := SessionItem{
		ID: entry.id, Title: entry.title(), ProjectName: entry.projectName(), CWD: entry.cwd(),
		Client: entry.root.client, UpdatedAt: entry.updatedAt(), MessageCount: entry.summary.steps, Totals: totals,
	}
	if item.MessageCount == 0 {
		item.MessageCount = conv.steps
	}
	if conv.model != "" {
		model := conv.model
		item.Model = &model
	}
	return item
}

// sessionPrice prices every call of a conversation; a call without its own
// time belongs to the day the conversation was last updated.
func sessionPrice(conv sessionConversation, fallback time.Time, loc *time.Location, snapshot pricing.Snapshot) sessionParsed {
	parsed := sessionParsed{byDay: map[string]SessionUsageTotals{}, byDayModel: map[string]map[string]SessionUsageTotals{}}
	for _, rec := range conv.records {
		at := rec.at
		if at.IsZero() {
			at = fallback
		}
		day := at.In(loc).Format(time.DateOnly)
		model := rec.model
		if model == "" {
			model = "unknown"
			rec.model = model
		}
		t := sessionTotalsOf(rec, snapshot)
		sessionAdd(parsed.byDay, day, t)
		models := parsed.byDayModel[day]
		if models == nil {
			models = map[string]SessionUsageTotals{}
			parsed.byDayModel[day] = models
		}
		sessionAdd(models, model, t)
	}
	return parsed
}

func sessionTotalsOf(rec sessionRecord, snapshot pricing.Snapshot) SessionUsageTotals {
	u := rec.usage
	t := SessionUsageTotals{
		TotalTokens: u.total(), InputTokens: u.input + u.cacheRead + u.cacheWrite, OutputTokens: u.output,
		CachedReadTokens: u.cacheRead, CacheWriteTokens: u.cacheWrite, ReasoningTokens: u.thinking, ModelCalls: 1,
	}
	if cost, ok, reason := sessionCost(snapshot, rec); ok {
		t.EstimatedUSD = &cost
	} else if t.TotalTokens > 0 {
		t.UnpricedTokens, t.unpricedReasons = t.TotalTokens, []string{reason}
	}
	return t
}

func sessionAdd(m map[string]SessionUsageTotals, key string, t SessionUsageTotals) {
	cur := m[key]
	cur.Merge(t)
	m[key] = cur
}

func sessionInWindow(day string, w usage.Window, loc *time.Location) bool {
	t, err := time.ParseInLocation(time.DateOnly, day, loc)
	return err == nil && !t.Before(w.Start) && t.Before(w.End)
}

// sessionMergeWindow adds the days of a conversation inside the window to
// the day and model rows and returns their sum.
func sessionMergeWindow(parsed sessionParsed, w usage.Window, loc *time.Location, byModel, byDay map[string]SessionUsageTotals) SessionUsageTotals {
	var matched SessionUsageTotals
	for day, t := range parsed.byDay {
		if !sessionInWindow(day, w, loc) {
			continue
		}
		matched.Merge(t)
		sessionAdd(byDay, day, t)
		for model, mt := range parsed.byDayModel[day] {
			sessionAdd(byModel, model, mt)
		}
	}
	return matched
}

func sessionWindowTotals(parsed sessionParsed) SessionUsageTotals {
	var all SessionUsageTotals
	for _, t := range parsed.byDay {
		all.Merge(t)
	}
	return all
}

func sessionSortedModels(byModel map[string]SessionUsageTotals) []SessionModelRow {
	rows := make([]SessionModelRow, 0, len(byModel))
	for m, t := range byModel {
		rows = append(rows, SessionModelRow{m, t})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Totals.TotalTokens != rows[j].Totals.TotalTokens {
			return rows[i].Totals.TotalTokens > rows[j].Totals.TotalTokens
		}
		return sessionNameLess(rows[i].Model, rows[j].Model)
	})
	return rows
}

func sessionNameLess(a, b string) bool {
	if la, lb := strings.ToLower(a), strings.ToLower(b); la != lb {
		return la < lb
	}
	return a < b
}

func sessionRows(m map[string]SessionUsageTotals) []usage.BreakdownRow {
	rows := make([]usage.BreakdownRow, 0, len(m))
	for name, t := range m {
		rows = append(rows, usage.BreakdownRow{Name: name, Totals: t.APITotals(), EstimatedUSD: t.EstimatedUSD, UnpricedTokens: t.UnpricedTokens})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Totals.TotalTokens != rows[j].Totals.TotalTokens {
			return rows[i].Totals.TotalTokens > rows[j].Totals.TotalTokens
		}
		return sessionNameLess(rows[i].Name, rows[j].Name)
	})
	return rows
}

func sessionCostSummary(agg SessionUsageTotals, byModel, byProject, byDay, byClient map[string]SessionUsageTotals, w usage.Window, now time.Time, version string) usage.Summary {
	days := make([]string, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Strings(days)
	daily := make([]usage.DailyRow, 0, len(days))
	for _, d := range days {
		t := byDay[d]
		daily = append(daily, usage.DailyRow{Date: d, Totals: t.APITotals(), EstimatedUSD: t.EstimatedUSD, UnpricedTokens: t.UnpricedTokens})
	}
	models := []usage.BreakdownRow{}
	for _, r := range sessionSortedModels(byModel) {
		models = append(models, usage.BreakdownRow{Name: r.Model, Totals: r.Totals.APITotals(), EstimatedUSD: r.Totals.EstimatedUSD, UnpricedTokens: r.Totals.UnpricedTokens})
	}
	reasons := map[string]bool{}
	for _, reason := range agg.unpricedReasons {
		reasons[reason] = true
	}
	warnings := make([]string, 0, len(reasons))
	for reason := range reasons {
		warnings = append(warnings, reason)
	}
	sort.Strings(warnings)
	source, confidence := usage.SourceUnavailable, usage.ConfidencePriced
	if agg.TotalTokens > 0 {
		source = usage.SourceLocalSessions
	}
	switch {
	case agg.TotalTokens == 0:
		confidence = usage.ConfidenceUnavailable
	case agg.EstimatedUSD == nil:
		confidence = usage.ConfidenceTokensOnly
	case agg.UnpricedTokens > 0:
		confidence = usage.ConfidencePartiallyPriced
	}
	return usage.Summary{
		Source:         source,
		Confidence:     confidence,
		Window:         w,
		EstimatedUSD:   agg.EstimatedUSD,
		UnpricedTokens: agg.UnpricedTokens,
		Totals:         agg.APITotals(),
		DailyRows:      daily,
		ModelRows:      models,
		ProjectRows:    sessionRows(byProject),
		ClientRows:     sessionRows(byClient),
		Warnings:       warnings,
		PricingVersion: version,
		CalculatedAt:   now,
	}
}
