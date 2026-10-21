package claude

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/licoy/quosparr/internal/cachehit"
	"github.com/licoy/quosparr/internal/usage"
)

// ScanCache summarizes cache reads from every local Claude session in the
// half-open range [window.Start, window.End).
func (s SessionScanner) ScanCache(ctx context.Context, window usage.Window, now time.Time) (cachehit.Report, error) {
	if !window.End.After(window.Start) {
		return cachehit.Report{}, fmt.Errorf("claude: invalid cache-hit window")
	}
	if err := ctx.Err(); err != nil {
		return cachehit.Report{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	sink, err := s.collect(ctx, math.MaxInt, time.Time{})
	if err != nil {
		return cachehit.Report{}, err
	}
	report := cachehit.Report{Start: window.Start, End: window.End, CalculatedAt: now}
	// Unreadable lines are skipped and counted, so one damaged log does not
	// hide every other one. A log last modified before the range cannot hold
	// a line of it.
	for _, u := range sink.unread {
		if !u.mtime.Before(window.Start) {
			report.UnreadLines += u.lines
		}
	}
	models, sessions := map[string]cachehit.Totals{}, map[string]cachehit.Totals{}
	for _, key := range sink.order {
		if err := ctx.Err(); err != nil {
			return cachehit.Report{}, err
		}
		rec := sink.kept[key]
		if !rec.hasTime {
			return cachehit.Report{}, fmt.Errorf("claude: cache-hit sample has no valid timestamp")
		}
		if rec.at.Before(window.Start) || !rec.at.Before(window.End) {
			continue
		}
		if rec.cacheInvalid {
			return cachehit.Report{}, fmt.Errorf("claude: invalid cache-hit token counters")
		}
		model := rec.model
		if model == "" {
			model = "unknown"
		}
		mt, st := models[model], sessions[rec.sessionID]
		if rec.cacheKnown {
			if err := addCacheSample(&report.Totals, rec.cacheInput, rec.cacheRead); err != nil {
				return cachehit.Report{}, err
			}
			if err := addCacheSample(&mt, rec.cacheInput, rec.cacheRead); err != nil {
				return cachehit.Report{}, err
			}
			if err := addCacheSample(&st, rec.cacheInput, rec.cacheRead); err != nil {
				return cachehit.Report{}, err
			}
		} else {
			if err := report.Totals.AddMissing(); err != nil {
				return cachehit.Report{}, err
			}
			if err := mt.AddMissing(); err != nil {
				return cachehit.Report{}, err
			}
			if err := st.AddMissing(); err != nil {
				return cachehit.Report{}, err
			}
		}
		models[model], sessions[rec.sessionID] = mt, st
	}
	report.Models = cacheRows(models, nil)
	names := make(map[string]string, len(sessions))
	for id := range sessions {
		names[id] = sink.meta(id).title
	}
	report.Sessions = cacheRows(sessions, names)
	return report, nil
}

func addCacheSample(t *cachehit.Totals, input, cached int64) error {
	if err := t.AddSample(input, cached); err != nil {
		return fmt.Errorf("claude: %w", err)
	}
	return nil
}

func cacheRows(values map[string]cachehit.Totals, names map[string]string) []cachehit.Row {
	rows := make([]cachehit.Row, 0, len(values))
	for id, totals := range values {
		name := id
		if names != nil && names[id] != "" {
			name = names[id]
		}
		rows = append(rows, cachehit.Row{ID: id, Name: name, Totals: totals})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Totals.InputTokens != rows[j].Totals.InputTokens {
			return rows[i].Totals.InputTokens > rows[j].Totals.InputTokens
		}
		return rows[i].ID < rows[j].ID
	})
	return rows
}
