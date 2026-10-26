package grok

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// SessionUsageTotals is the token and cost sum of local Grok CLI
// turn_completed records. EstimatedUSD is the API-equivalent cost from the
// official token prices of the priced rows (nil when none was priced).
type SessionUsageTotals struct {
	TotalTokens      int64
	InputTokens      int64
	OutputTokens     int64
	CachedReadTokens int64
	ReasoningTokens  int64
	Turns            int64
	ModelCalls       int64
	EstimatedUSD     *decimal.Decimal
	UnpricedTokens   int64
	unpricedReasons  []string
}

// HasData reports whether any token, turn or cost was counted.
func (t SessionUsageTotals) HasData() bool {
	return t.TotalTokens > 0 || t.Turns > 0 || (t.EstimatedUSD != nil && t.EstimatedUSD.Sign() > 0)
}

// APITotals is the usage.Totals view: cached input is split from the
// uncached part and a thread counts once when any turn ran.
func (t SessionUsageTotals) APITotals() usage.Totals {
	cached := max(0, t.CachedReadTokens)
	uncached := max(0, t.InputTokens-cached)
	threads := int64(0)
	if t.Turns > 0 {
		threads = 1
	}
	return usage.Totals{
		TotalTokens:         t.TotalTokens,
		UncachedInputTokens: uncached,
		CachedInputTokens:   cached,
		OutputTokens:        t.OutputTokens,
		Turns:               t.Turns,
		Threads:             threads,
	}
}

// Merge adds o to t.
func (t *SessionUsageTotals) Merge(o SessionUsageTotals) {
	t.TotalTokens += o.TotalTokens
	t.InputTokens += o.InputTokens
	t.OutputTokens += o.OutputTokens
	t.CachedReadTokens += o.CachedReadTokens
	t.ReasoningTokens += o.ReasoningTokens
	t.Turns += o.Turns
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

// SessionItem is one recent session. Totals count only the part of the
// session inside the cost window.
type SessionItem struct {
	ID           string
	Title        string
	ProjectName  string
	CWD          *string
	Model        *string
	UpdatedAt    time.Time
	MessageCount int64
	Totals       SessionUsageTotals
}

// SessionUsageSummary is the local Grok usage in the cost window.
type SessionUsageSummary struct {
	Totals       SessionUsageTotals
	SessionCount int
	ModelRows    []SessionModelRow
	RecentItems  []SessionItem
	// DailyTokens maps a local calendar day (yyyy-MM-dd) to its total tokens,
	// for every day found, window or not.
	DailyTokens  map[string]int64
	CostSummary  usage.Summary
	CalculatedAt time.Time
	SourceLabel  string
}

// SessionClientName is the client row name of the cost summary.
const SessionClientName = "Grok CLI"

// SessionScanner reads ~/.grok/sessions/**/summary.json and the turn_completed
// records of the updates.jsonl beside each summary.
type SessionScanner struct {
	// GrokHome is the Grok data directory (normally ~/.grok).
	GrokHome string
	// Location decides the calendar days; nil means time.Local.
	Location *time.Location
	// Cache reuses window-independent cache-hit samples across range changes.
	Cache *CacheScanCache
	// PricingSnapshot is captured once for each complete scan
	PricingSnapshot func() pricing.Snapshot
}

// SessionScanOptions selects what Scan returns.
type SessionScanOptions struct {
	// RecentLimit is the number of recent sessions listed (0 for none).
	RecentLimit int
	// SessionLimit caps how many of the newest summaries are read; it is
	// raised to RecentLimit when smaller.
	SessionLimit int
	// CostWindow is the half-open range [Start, End) of days to total. The
	// zero value means January 1st of Now's year up to Now.
	CostWindow usage.Window
	Now        time.Time
}

// DefaultSessionScanOptions mirrors the app defaults: five recent sessions,
// up to 300 summaries, year to date.
func DefaultSessionScanOptions(now time.Time) SessionScanOptions {
	return SessionScanOptions{RecentLimit: 5, SessionLimit: 300, Now: now}
}

// SessionCostUSDFromTicks converts the CLI's costUsdTicks (1e-9 dollar) to
// USD; zero ticks report false. It is for diagnostics: the API-equivalent
// cost always comes from the price table.
func SessionCostUSDFromTicks(ticks int64) (decimal.Decimal, bool) {
	if ticks == 0 {
		return decimal.Decimal{}, false
	}
	return decimal.FromInt64(ticks).Div(decimal.FromInt(1_000_000_000)), true
}

func (s SessionScanner) loc() *time.Location {
	if s.Location != nil {
		return s.Location
	}
	return time.Local
}

// ScanCost totals only the API-equivalent cost of a window (up to 400
// sessions).
func (s SessionScanner) ScanCost(ctx context.Context, window usage.Window, now time.Time) (usage.Summary, error) {
	r, err := s.Scan(ctx, SessionScanOptions{SessionLimit: 400, CostWindow: window, Now: now})
	return r.CostSummary, err
}

// Scan reads the newest sessions and totals them. A missing sessions
// directory is an empty result, an unreadable or malformed summary is
// skipped; only a cancelled context is an error.
func (s SessionScanner) Scan(ctx context.Context, opts SessionScanOptions) (SessionUsageSummary, error) {
	snapshot := s.pricingSnapshot()
	loc := s.loc()
	recentLimit := max(0, opts.RecentLimit)
	sessionLimit := max(recentLimit, opts.SessionLimit)
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	window := opts.CostWindow
	if window.Start.IsZero() && window.End.IsZero() {
		year := now.In(loc).Year()
		window = usage.Window{Start: sessionDayStart(year, 1, 1, loc), End: now}
	}

	summaries, err := s.listSummaries(ctx, sessionLimit)
	if err != nil {
		return SessionUsageSummary{}, err
	}
	var agg SessionUsageTotals
	byModel := map[string]SessionUsageTotals{}
	byProject := map[string]SessionUsageTotals{}
	byDay := map[string]SessionUsageTotals{}
	dailyTokens := map[string]int64{}
	items := []SessionItem{}

	for i, sum := range summaries {
		if err := ctx.Err(); err != nil {
			return SessionUsageSummary{}, err
		}
		parsed, err := parseSessionUpdates(filepath.Join(sum.directory, "updates.jsonl"), sum.updatedAt, sum.model, loc, snapshot)
		if err != nil {
			return SessionUsageSummary{}, err
		}
		for day, t := range parsed.byDay {
			dailyTokens[day] += t.TotalTokens
		}
		inWindow := mergeSessionWindow(parsed, window, loc, &agg, byModel, byProject, byDay, sum.projectName)
		if i < recentLimit {
			totals := SessionUsageTotals{}
			if inWindow {
				totals = windowedSessionTotals(parsed, window, loc)
			}
			items = append(items, SessionItem{
				ID: sum.id, Title: sum.title, ProjectName: sum.projectName, CWD: sum.cwd, Model: sum.model,
				UpdatedAt: sum.updatedAt, MessageCount: sum.messageCount, Totals: totals,
			})
		}
	}
	return SessionUsageSummary{
		Totals:       agg,
		SessionCount: len(summaries),
		ModelRows:    sortedSessionModelRows(byModel),
		RecentItems:  items,
		DailyTokens:  dailyTokens,
		CostSummary:  makeSessionCostSummary(agg, byModel, byProject, byDay, window, now, snapshotPricingVersion(snapshot)),
		CalculatedAt: now,
		SourceLabel:  "local-sessions",
	}, nil
}

// ---- summaries ----

type sessionSummaryFile struct {
	id           string
	title        string
	projectName  string
	cwd          *string
	model        *string
	effort       string // session-level reasoning_effort, "" when absent
	updatedAt    time.Time
	messageCount int64
	directory    string
}

type sessionCandidate struct {
	path  string
	mtime time.Time
}

func (s SessionScanner) listSummaries(ctx context.Context, limit int) ([]sessionSummaryFile, error) {
	if limit <= 0 {
		return nil, nil
	}
	root := filepath.Join(s.GrokHome, "sessions")
	st, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("Grok sessions root is not a directory")
	}
	var candidates []sessionCandidate
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != "summary.json" || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		candidates = append(candidates, sessionCandidate{path, info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].mtime.Equal(candidates[j].mtime) {
			return candidates[i].mtime.After(candidates[j].mtime)
		}
		return candidates[i].path < candidates[j].path
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]sessionSummaryFile, 0, len(candidates))
	for _, c := range candidates {
		sum, ok, err := parseSessionSummary(c)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, sum)
		}
	}
	return out, nil
}

func parseSessionSummary(c sessionCandidate) (sessionSummaryFile, bool, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return sessionSummaryFile{}, false, err
	}
	root, err := parseLooseJSON(data)
	if err != nil || root.kind != jsonObject {
		return sessionSummaryFile{}, false, nil
	}
	info := root.get("info")
	dir := filepath.Dir(c.path)
	id := sessionNonEmpty(info.get("id"))
	if id == nil {
		base := filepath.Base(dir)
		id = &base
	}
	cwd := sessionNonEmpty(info.get("cwd"))
	title := "Untitled session"
	if t := sessionNonEmpty(root.get("generated_title")); t != nil {
		title = *t
	} else if t := sessionNonEmpty(root.get("session_summary")); t != nil {
		title = *t
	}
	updated := c.mtime
	if t, ok := sessionDate(root.get("last_active_at")); ok {
		updated = t
	} else if t, ok := sessionDate(root.get("updated_at")); ok {
		updated = t
	}
	messages := int64(0)
	if v, ok := sessionAsInt(root.get("num_chat_messages")); ok {
		messages = v
	} else if v, ok := sessionAsInt(root.get("num_messages")); ok {
		messages = v
	}
	project := ""
	if cwd != nil {
		project = *cwd
	}
	effort := ""
	if e := sessionNonEmpty(root.get("reasoning_effort")); e != nil {
		effort = *e
	}
	return sessionSummaryFile{
		id: *id, title: title, projectName: usage.ProjectDisplayName(project), cwd: cwd,
		model: sessionNonEmpty(root.get("current_model_id")), effort: effort, updatedAt: updated,
		messageCount: messages, directory: dir,
	}, true, nil
}

// sessionNonEmpty is the trimmed string value, nil when absent, not a string
// or blank.
func sessionNonEmpty(n *jsonNode) *string {
	if s, ok := n.str(); ok {
		if t, ok := billingNonEmpty(s); ok {
			return &t
		}
	}
	return nil
}

func sessionDate(n *jsonNode) (time.Time, bool) {
	if s, ok := n.str(); ok {
		return BillingParseDate(s)
	}
	return time.Time{}, false
}

// sessionAsInt is `value as? Int` on a JSON value: a number with an exact
// integral value that fits.
func sessionAsInt(n *jsonNode) (int64, bool) {
	if n == nil {
		return 0, false
	}
	switch n.kind {
	case jsonNumber:
		return billingNumberInt64(n.s)
	case jsonBool:
		if n.b {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// ---- updates.jsonl ----

type sessionParsed struct {
	byDay      map[string]SessionUsageTotals
	byDayModel map[string]map[string]SessionUsageTotals
}

var (
	sessionTurnMarker  = []byte("turn_completed")
	sessionUsageMarker = []byte(`"usage"`)
)

func parseSessionUpdates(path string, fallbackDate time.Time, fallbackModel *string, loc *time.Location, snapshots ...pricing.Snapshot) (sessionParsed, error) {
	snapshot := pricing.Snapshot{}
	if len(snapshots) > 0 {
		snapshot = snapshots[0]
	}
	parsed := sessionParsed{byDay: map[string]SessionUsageTotals{}, byDayModel: map[string]map[string]SessionUsageTotals{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return parsed, nil // A new session may not have produced updates yet.
	}
	if err != nil {
		return sessionParsed{}, err
	}
	for len(data) > 0 {
		line := data
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			data = nil
		}
		if !bytes.Contains(line, sessionTurnMarker) || !bytes.Contains(line, sessionUsageMarker) {
			continue
		}
		obj, err := parseLooseJSON(line)
		if err != nil || obj.kind != jsonObject {
			continue
		}
		update := obj.get("params").get("update")
		if s, _ := update.get("sessionUpdate").str(); update == nil || s != "turn_completed" {
			continue
		}
		usageNode := update.get("usage")
		if usageNode == nil || usageNode.kind != jsonObject {
			continue
		}
		day := sessionDayKey(obj, fallbackDate, loc)
		turn := sessionUsageTotals(usageNode, true, nil)
		priced := accumulateSessionModels(usageNode.get("modelUsage"), usageNode, fallbackModel, day, parsed.byDayModel, snapshot)
		turn.EstimatedUSD, turn.UnpricedTokens, turn.unpricedReasons = priced.EstimatedUSD, priced.UnpricedTokens, priced.unpricedReasons
		dayTotals := parsed.byDay[day]
		dayTotals.Merge(turn)
		parsed.byDay[day] = dayTotals
	}
	return parsed, nil
}

// accumulateSessionModels prices every model row of a turn and returns the
// sum of the priced rows (nil when none was priced), so an unknown model does
// not blank the turn.
func accumulateSessionModels(modelUsage, fallbackUsage *jsonNode, fallbackModel *string, day string,
	byDayModel map[string]map[string]SessionUsageTotals, snapshot pricing.Snapshot) SessionUsageTotals {
	var sum SessionUsageTotals
	saw := false
	if modelUsage != nil && modelUsage.kind == jsonObject {
		for model, value := range modelUsage.obj {
			if value.kind != jsonObject {
				continue
			}
			saw = true
			mergeSessionModelRow(sessionUsageTotals(value, false, &model, snapshot), model, day, byDayModel, &sum)
		}
	}
	if !saw {
		model := "unknown"
		if fallbackModel == nil || *fallbackModel == "" {
			return sessionUsageTotals(fallbackUsage, false, &model, snapshot)
		}
		model = *fallbackModel
		mergeSessionModelRow(sessionUsageTotals(fallbackUsage, false, &model, snapshot), model, day, byDayModel, &sum)
	}
	return sum
}

func mergeSessionModelRow(row SessionUsageTotals, model, day string, byDayModel map[string]map[string]SessionUsageTotals, sum *SessionUsageTotals) {
	models := byDayModel[day]
	if models == nil {
		models = map[string]SessionUsageTotals{}
		byDayModel[day] = models
	}
	cur := models[model]
	cur.Merge(row)
	models[model] = cur
	sum.Merge(row)
}

func sessionUsageTotals(usageNode *jsonNode, countTurn bool, model *string, snapshots ...pricing.Snapshot) SessionUsageTotals {
	snapshot := pricing.Snapshot{}
	if len(snapshots) > 0 {
		snapshot = snapshots[0]
	}
	input := sessionIntValue(usageNode.get("inputTokens"))
	output := sessionIntValue(usageNode.get("outputTokens"))
	cached := sessionIntValue(usageNode.get("cachedReadTokens"))
	total := sessionIntValue(usageNode.get("totalTokens"))
	if total <= 0 {
		total = max(0, input+output)
	}
	t := SessionUsageTotals{
		TotalTokens: total, InputTokens: input, OutputTokens: output, CachedReadTokens: cached,
		ReasoningTokens: sessionIntValue(usageNode.get("reasoningTokens")),
		ModelCalls:      sessionIntValue(usageNode.get("modelCalls")),
	}
	if countTurn {
		t.Turns = 1
	}
	if model != nil {
		if cost, ok, reason := PricingCostWithSnapshot(snapshot, *model, int(input), int(cached), int(output)); ok {
			t.EstimatedUSD = &cost
		} else {
			t.UnpricedTokens = max(0, input-cached) + cached + output
			if t.UnpricedTokens > 0 {
				t.unpricedReasons = []string{reason}
			}
		}
	}
	return t
}

// sessionIntValue reads a token counter: integers and rounded fractions,
// never negative; text, null and anything else count as 0.
func sessionIntValue(n *jsonNode) int64 {
	if n == nil {
		return 0
	}
	switch n.kind {
	case jsonBool:
		if n.b {
			return 1
		}
	case jsonNumber:
		if v, err := strconv.ParseInt(n.s, 10, 64); err == nil {
			return max(0, v)
		}
		f := math.Round(billingNumberDouble(n.s))
		switch {
		case f >= 1<<63:
			return math.MaxInt64
		case f > 0:
			return int64(f)
		}
	}
	return 0
}

// sessionTimestamp reads a timestamp number (a boolean counts as 0 or 1).
func sessionTimestamp(n *jsonNode) (float64, bool) {
	if n == nil {
		return 0, false
	}
	switch n.kind {
	case jsonNumber:
		return billingNumberDouble(n.s), true
	case jsonBool:
		if n.b {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func sessionDayKey(obj *jsonNode, fallback time.Time, loc *time.Location) string {
	if secs, ok := sessionTimestamp(obj.get("timestamp")); ok {
		return sessionDayString(secs, loc)
	}
	meta := obj.get("params").get("_meta")
	for _, key := range []string{"agentTimestampMs", "streamStartMs", "turnStartMs"} {
		if ms, ok := sessionTimestamp(meta.get(key)); ok {
			return sessionDayString(ms/1000, loc)
		}
	}
	return sessionDayOf(fallback, loc)
}

// sessionDayStart is midnight of a local day; a midnight skipped by a clock
// change resolves forward to the first instant of the day.
func sessionDayStart(year, month, day int, loc *time.Location) time.Time {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)
	if y, m, d := t.Date(); y == year && int(m) == month && d == day {
		return t
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)
}

// ---- aggregation ----

func inSessionWindow(day string, w usage.Window, loc *time.Location) bool {
	t, ok := sessionDayDate(day, loc)
	return ok && !t.Before(w.Start) && t.Before(w.End)
}

func mergeSessionWindow(parsed sessionParsed, w usage.Window, loc *time.Location, agg *SessionUsageTotals,
	byModel, byProject, byDay map[string]SessionUsageTotals, project string) bool {
	var matched SessionUsageTotals
	for day, t := range parsed.byDay {
		if !inSessionWindow(day, w, loc) {
			continue
		}
		matched.Merge(t)
		bucket := byDay[day]
		bucket.Merge(t)
		byDay[day] = bucket
		for model, mt := range parsed.byDayModel[day] {
			row := byModel[model]
			row.Merge(mt)
			byModel[model] = row
		}
	}
	if !matched.HasData() {
		return false
	}
	agg.Merge(matched)
	p := byProject[project]
	p.Merge(matched)
	byProject[project] = p
	return true
}

func windowedSessionTotals(parsed sessionParsed, w usage.Window, loc *time.Location) SessionUsageTotals {
	var matched SessionUsageTotals
	for day, t := range parsed.byDay {
		if inSessionWindow(day, w, loc) {
			matched.Merge(t)
		}
	}
	return matched
}

func sortedSessionModelRows(byModel map[string]SessionUsageTotals) []SessionModelRow {
	rows := make([]SessionModelRow, 0, len(byModel))
	for m, t := range byModel {
		rows = append(rows, SessionModelRow{m, t})
	}
	// Names are unique, so the order is total whatever the map order was.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Totals.TotalTokens != rows[j].Totals.TotalTokens {
			return rows[i].Totals.TotalTokens > rows[j].Totals.TotalTokens
		}
		return sessionNameLess(rows[i].Model, rows[j].Model)
	})
	return rows
}

// sessionNameLess is localizedCaseInsensitiveCompare == .orderedAscending,
// breaking ties of names that compare equal by code point so the result does
// not depend on map order.
func sessionNameLess(a, b string) bool {
	if c := billingLocalizedCompare(a, b); c != 0 {
		return c < 0
	}
	return a < b
}

func makeSessionCostSummary(agg SessionUsageTotals, byModel, byProject, byDay map[string]SessionUsageTotals,
	w usage.Window, now time.Time, versions ...string) usage.Summary {
	version := PricingVersion
	if len(versions) > 0 {
		version = versions[0]
	}
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
	for _, r := range sortedSessionModelRows(byModel) {
		models = append(models, usage.BreakdownRow{Name: r.Model, Totals: r.Totals.APITotals(), EstimatedUSD: r.Totals.EstimatedUSD, UnpricedTokens: r.Totals.UnpricedTokens})
	}
	projects := make([]usage.BreakdownRow, 0, len(byProject))
	for name, t := range byProject {
		projects = append(projects, usage.BreakdownRow{Name: name, Totals: t.APITotals(), EstimatedUSD: t.EstimatedUSD, UnpricedTokens: t.UnpricedTokens})
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Totals.TotalTokens != projects[j].Totals.TotalTokens {
			return projects[i].Totals.TotalTokens > projects[j].Totals.TotalTokens
		}
		return sessionNameLess(projects[i].Name, projects[j].Name)
	})
	reasons := map[string]struct{}{}
	for _, reason := range agg.unpricedReasons {
		reasons[reason] = struct{}{}
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
		ProjectRows:    projects,
		ClientRows:     []usage.BreakdownRow{{Name: SessionClientName, Totals: agg.APITotals(), EstimatedUSD: agg.EstimatedUSD, UnpricedTokens: agg.UnpricedTokens}},
		Warnings:       warnings,
		PricingVersion: version,
		CalculatedAt:   now,
	}
}
