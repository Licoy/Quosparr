package claude

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

const (
	// SessionClientName is the client row of a Claude Code cost summary.
	SessionClientName = "Claude Code"
	defaultSessionCap = 300
)

// SessionUsageTotals is the token split of local Claude Code assistant
// records. EstimatedUSD is priced in this package. It is nil when nothing
// in the total had a known price.
type SessionUsageTotals struct {
	Input          int64
	CacheWrite5m   int64
	CacheWrite1h   int64
	CacheRead      int64
	Output         int64
	Turns          int64
	EstimatedUSD   *decimal.Decimal
	UnpricedTokens int64
}

// APITotals maps the split onto usage.Totals. Uncached input is the uncached
// prompt plus both cache writes. Cached input is the cache read. Thinking
// tokens are already inside output and are not added again.
func (t SessionUsageTotals) APITotals() usage.Totals {
	uncached := t.Input + t.CacheWrite5m + t.CacheWrite1h
	threads := int64(0)
	if t.Turns > 0 {
		threads = 1
	}
	return usage.Totals{
		TotalTokens:         uncached + t.CacheRead + t.Output,
		UncachedInputTokens: uncached,
		CachedInputTokens:   t.CacheRead,
		OutputTokens:        t.Output,
		Turns:               t.Turns,
		Threads:             threads,
	}
}

func (t *SessionUsageTotals) add(o SessionUsageTotals) {
	t.Input += o.Input
	t.CacheWrite5m += o.CacheWrite5m
	t.CacheWrite1h += o.CacheWrite1h
	t.CacheRead += o.CacheRead
	t.Output += o.Output
	t.Turns += o.Turns
	t.UnpricedTokens += o.UnpricedTokens
	switch {
	case t.EstimatedUSD != nil && o.EstimatedUSD != nil:
		sum := t.EstimatedUSD.Add(*o.EstimatedUSD)
		t.EstimatedUSD = &sum
	case o.EstimatedUSD != nil:
		v := *o.EstimatedUSD
		t.EstimatedUSD = &v
	}
}

func (t SessionUsageTotals) from(parts TokenParts, cost *decimal.Decimal, turn bool) SessionUsageTotals {
	out := SessionUsageTotals{
		Input: parts.Input, CacheWrite5m: parts.CacheWrite5m, CacheWrite1h: parts.CacheWrite1h,
		CacheRead: parts.CacheRead, Output: parts.Output, EstimatedUSD: cost,
	}
	if cost == nil {
		out.UnpricedTokens = max(0, parts.Input) + max(0, parts.CacheWrite5m) + max(0, parts.CacheWrite1h) + max(0, parts.CacheRead) + max(0, parts.Output)
	}
	if turn {
		out.Turns = 1
	}
	return out
}

// SessionItem is one recent session. An empty Title is left for the caller
// to replace with the localized "unnamed" string.
type SessionItem struct {
	ID          string
	Title       string
	ProjectName string
	CWD         *string
	Model       *string
	UpdatedAt   time.Time
	Totals      SessionUsageTotals
}

// SessionsItem is the shared recent-session shape.
func (it SessionItem) SessionsItem() sessions.Item {
	item := sessions.Item{
		ID: it.ID, Title: it.Title, ProjectName: it.ProjectName,
		UpdatedAt: it.UpdatedAt, State: sessions.StateRecent,
		Totals:         toSessionTotals(it.Totals.APITotals()),
		UnpricedTokens: it.Totals.UnpricedTokens,
		Model:          it.Model, Cwd: it.CWD,
	}
	if it.Totals.EstimatedUSD != nil {
		if d, err := swiftjson.ParseDecimal(it.Totals.EstimatedUSD.String()); err == nil {
			item.EstimatedUSD = &d
		}
	}
	return item
}

func toSessionTotals(t usage.Totals) sessions.Totals {
	return sessions.Totals{
		TotalTokens: int(t.TotalTokens), UncachedInputTokens: int(t.UncachedInputTokens),
		CachedInputTokens: int(t.CachedInputTokens), OutputTokens: int(t.OutputTokens),
		Turns: int(t.Turns), Threads: int(t.Threads),
	}
}

// SessionUsageSummary is the local Claude usage inside the cost window.
type SessionUsageSummary struct {
	Totals       SessionUsageTotals
	SessionCount int
	ModelRows    []usage.BreakdownRow
	RecentItems  []SessionItem
	DailyTokens  map[string]int64
	CostSummary  usage.Summary
	BadLines     int
	CalculatedAt time.Time
}

// SessionScanner reads projects/**/*.jsonl, including subagent logs. Cache,
// when set, is shared by scans so unchanged files are not parsed again.
type SessionScanner struct {
	ConfigDir string
	Home      string
	Location  *time.Location
	Cache     *SessionCache
	// PricingSnapshot is read once per scan, never per request
	PricingSnapshot func() pricing.Snapshot
}

// SessionScanOptions selects what Scan returns. The newest SessionLimit files
// and every file changed inside CostWindow are read. The heatmap and recent
// sessions use every record read; the totals and cost summary only those in
// CostWindow. A zero CostWindow keeps every parsed record.
type SessionScanOptions struct {
	RecentLimit  int
	SessionLimit int
	CostWindow   usage.Window
	Now          time.Time
}

// DefaultSessionScanOptions is five recent sessions, 300 files, year to date
// in the local time zone.
func DefaultSessionScanOptions(now time.Time) SessionScanOptions {
	start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	return SessionScanOptions{RecentLimit: 5, SessionLimit: defaultSessionCap, CostWindow: usage.Window{Start: start, End: now}, Now: now}
}

func (s SessionScanner) loc() *time.Location {
	if s.Location != nil {
		return s.Location
	}
	return time.Local
}

// Scan reads the newest session logs. A missing projects directory is an
// empty result. Bad lines are counted and skipped. Only a cancelled context
// fails the scan.
func (s SessionScanner) Scan(ctx context.Context, opts SessionScanOptions) (SessionUsageSummary, error) {
	snapshot := s.pricingSnapshot()
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	limit := opts.SessionLimit
	if limit <= 0 {
		limit = defaultSessionCap
	}
	if opts.RecentLimit > limit {
		limit = opts.RecentLimit
	}
	sink, err := s.collect(ctx, limit, opts.CostWindow.Start)
	if err != nil {
		return SessionUsageSummary{}, err
	}
	return sink.summary(s.loc(), opts, now, snapshot), nil
}

// ScanCost is Scan limited to the cost summary.
func (s SessionScanner) ScanCost(ctx context.Context, window usage.Window, now time.Time) (usage.Summary, error) {
	res, err := s.Scan(ctx, SessionScanOptions{SessionLimit: 400, CostWindow: window, Now: now})
	return res.CostSummary, err
}

func (sink *scanSink) summary(loc *time.Location, opts SessionScanOptions, now time.Time, snapshots ...pricing.Snapshot) SessionUsageSummary {
	snapshot := pricing.Snapshot{}
	if len(snapshots) > 0 {
		snapshot = snapshots[0]
	}
	bySession := map[string][]usageRecord{}
	for _, key := range sink.order {
		rec := sink.kept[key]
		bySession[rec.sessionID] = append(bySession[rec.sessionID], rec)
	}
	var agg SessionUsageTotals
	byModel := map[string]SessionUsageTotals{}
	byProject := map[string]SessionUsageTotals{}
	byDay := map[string]SessionUsageTotals{}
	dailyTokens := map[string]int64{}
	unknown := map[string]struct{}{}
	fastMiss := map[string]struct{}{}
	var recent []SessionItem
	sessionCount := 0
	for id, recs := range bySession {
		meta := sink.meta(id)
		project := usage.ProjectDisplayName(meta.cwd)
		var all, windowed SessionUsageTotals
		var model string
		var modelAt time.Time
		for _, rec := range recs {
			pricingModel := rec.pricingModel
			if pricingModel == "" {
				pricingModel = rec.model
			}
			cost, priced, reason := PricingCostWithSnapshot(snapshot, pricingModel, rec.parts, rec.geo)
			var costPtr *decimal.Decimal
			if priced {
				costPtr = &cost
			}
			one := SessionUsageTotals{}.from(rec.parts, costPtr, !rec.toolUse)
			all.add(one)
			dailyTokens[DayKey(rec.at, loc)] += one.APITotals().TotalTokens
			if rec.model != "" && (model == "" || (rec.hasTime && !rec.at.Before(modelAt))) {
				model, modelAt = rec.model, rec.at
			}
			if !recordInWindow(rec, opts.CostWindow) {
				continue
			}
			switch {
			case priced || one.UnpricedTokens == 0:
			case rec.parts.Fast:
				fastMiss[rec.model] = struct{}{}
			case rec.model != "":
				unknown[reason] = struct{}{}
			default:
				unknown["unknown-model:unknown"] = struct{}{}
			}
			windowed.add(one)
			addTotals(byModel, rec.model, one)
			addTotals(byDay, DayKey(rec.at, loc), one)
		}
		if windowed.APITotals().TotalTokens > 0 || windowed.Turns > 0 {
			sessionCount++
			agg.add(windowed)
			addTotals(byProject, project, windowed)
		}
		item := SessionItem{ID: id, Title: meta.title, ProjectName: project, UpdatedAt: meta.updated, Totals: all}
		if meta.cwd != "" {
			cwd := meta.cwd
			item.CWD = &cwd
		}
		if model != "" {
			item.Model = &model
		}
		recent = append(recent, item)
	}
	sort.Slice(recent, func(i, j int) bool {
		if !recent[i].UpdatedAt.Equal(recent[j].UpdatedAt) {
			return recent[i].UpdatedAt.After(recent[j].UpdatedAt)
		}
		return recent[i].ID < recent[j].ID
	})
	if len(recent) > max(0, opts.RecentLimit) {
		recent = recent[:max(0, opts.RecentLimit)]
	}
	return SessionUsageSummary{
		Totals: agg, SessionCount: sessionCount, ModelRows: modelRows(byModel),
		RecentItems: recent, DailyTokens: dailyTokens, BadLines: sink.bad,
		CostSummary:  costSummary(agg, byModel, byProject, byDay, unknown, fastMiss, opts.CostWindow, now, sink.bad, snapshotPricingVersion(snapshot)),
		CalculatedAt: now,
	}
}

func addTotals(m map[string]SessionUsageTotals, key string, t SessionUsageTotals) {
	row := m[key]
	row.add(t)
	m[key] = row
}

func modelRows(byModel map[string]SessionUsageTotals) []usage.BreakdownRow {
	names := make([]string, 0, len(byModel))
	for name := range byModel {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		ti, tj := byModel[names[i]].APITotals().TotalTokens, byModel[names[j]].APITotals().TotalTokens
		if ti != tj {
			return ti > tj
		}
		return names[i] < names[j]
	})
	rows := make([]usage.BreakdownRow, 0, len(names))
	for _, name := range names {
		t := byModel[name]
		rows = append(rows, usage.BreakdownRow{Name: name, Totals: t.APITotals(), EstimatedUSD: t.EstimatedUSD, UnpricedTokens: t.UnpricedTokens})
	}
	return rows
}

func costSummary(agg SessionUsageTotals, byModel, byProject, byDay map[string]SessionUsageTotals, unknown, fastMiss map[string]struct{}, w usage.Window, now time.Time, bad int, version string) usage.Summary {
	days := mapKeys(byDay)
	sort.Strings(days)
	daily := make([]usage.DailyRow, 0, len(days))
	for _, day := range days {
		t := byDay[day]
		daily = append(daily, usage.DailyRow{Date: day, Totals: t.APITotals(), EstimatedUSD: t.EstimatedUSD, UnpricedTokens: t.UnpricedTokens})
	}
	projects := make([]usage.BreakdownRow, 0, len(byProject))
	for name, t := range byProject {
		projects = append(projects, usage.BreakdownRow{Name: name, Totals: t.APITotals(), EstimatedUSD: t.EstimatedUSD, UnpricedTokens: t.UnpricedTokens})
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Totals.TotalTokens != projects[j].Totals.TotalTokens {
			return projects[i].Totals.TotalTokens > projects[j].Totals.TotalTokens
		}
		return projects[i].Name < projects[j].Name
	})
	warnings := warningList(unknown, fastMiss, bad)
	source, confidence := usage.SourceUnavailable, usage.ConfidenceUnavailable
	if agg.APITotals().TotalTokens > 0 || agg.Turns > 0 {
		source = usage.SourceLocalSessions
		if agg.EstimatedUSD != nil {
			confidence = usage.ConfidencePriced
			if agg.UnpricedTokens > 0 {
				confidence = usage.ConfidencePartiallyPriced
			}
		} else {
			confidence = usage.ConfidenceTokensOnly
		}
	}
	return usage.Summary{
		Source: source, Confidence: confidence, Window: w, EstimatedUSD: agg.EstimatedUSD, UnpricedTokens: agg.UnpricedTokens,
		Totals: agg.APITotals(), DailyRows: daily, ModelRows: modelRows(byModel),
		ProjectRows: projects,
		ClientRows:  []usage.BreakdownRow{{Name: SessionClientName, Totals: agg.APITotals(), EstimatedUSD: agg.EstimatedUSD, UnpricedTokens: agg.UnpricedTokens}},
		Warnings:    warnings, PricingVersion: version, CalculatedAt: now,
	}
}

func warningList(unknown, fastMiss map[string]struct{}, bad int) []string {
	var warnings []string
	for _, name := range mapKeysStruct(unknown) {
		warnings = append(warnings, name)
	}
	for _, name := range mapKeysStruct(fastMiss) {
		warnings = append(warnings, "fast-unpriced:"+name)
	}
	if bad > 0 {
		warnings = append(warnings, "malformed-jsonl-lines:"+strconv.Itoa(bad))
	}
	return warnings
}

func mapKeys(m map[string]SessionUsageTotals) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func mapKeysStruct(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
