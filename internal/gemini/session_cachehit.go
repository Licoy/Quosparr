package gemini

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/licoy/quosparr/internal/cachehit"
	"github.com/licoy/quosparr/internal/usage"
)

var errSessionCacheWindow = errors.New("gemini: invalid cache-hit window")

// ScanCache summarizes the cache reads of every conversation in the
// half-open range [window.Start, window.End). A call's input is its whole
// prompt (uncached input, cache reads and cache writes) and its cached part
// the cache reads; a call without its own time belongs to the moment the
// conversation was last updated. A conversation whose database and summary
// were both last changed before Start is not read. UnreadLines counts the
// damaged databases changed since Start; the older encrypted .pb
// conversations hold no readable counters and are not counted.
func (s SessionScanner) ScanCache(ctx context.Context, window usage.Window, now time.Time) (cachehit.Report, error) {
	if !window.End.After(window.Start) {
		return cachehit.Report{}, errSessionCacheWindow
	}
	if err := ctx.Err(); err != nil {
		return cachehit.Report{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	entries, _, err := s.sessionList(ctx)
	if err != nil {
		return cachehit.Report{}, err
	}
	report := cachehit.Report{Start: window.Start, End: window.End, CalculatedAt: now}
	models, convs := map[string]cachehit.Totals{}, map[string]cachehit.Totals{}
	names := map[string]string{}
	for _, entry := range entries {
		if entry.updatedAt().Before(window.Start) && entry.state.updatedAt().Before(window.Start) {
			continue
		}
		conv, err := s.sessionConversationOf(ctx, entry)
		if err != nil {
			if ctx.Err() != nil {
				return cachehit.Report{}, ctx.Err()
			}
			report.UnreadLines++
			continue
		}
		for _, rec := range conv.records {
			at := rec.at
			if at.IsZero() {
				at = entry.updatedAt()
			}
			if at.Before(window.Start) || !at.Before(window.End) {
				continue
			}
			model := rec.model
			if model == "" {
				model = "unknown"
			}
			u := rec.usage
			input := u.input + u.cacheRead + u.cacheWrite
			mt, ct := models[model], convs[entry.id]
			for _, t := range []*cachehit.Totals{&report.Totals, &mt, &ct} {
				if err := t.AddSample(input, u.cacheRead); err != nil {
					return cachehit.Report{}, err
				}
			}
			models[model], convs[entry.id] = mt, ct
			names[entry.id] = entry.title()
		}
	}
	report.Models = sessionCacheRows(models, nil)
	report.Sessions = sessionCacheRows(convs, names)
	return report, nil
}

func sessionCacheRows(values map[string]cachehit.Totals, names map[string]string) []cachehit.Row {
	rows := make([]cachehit.Row, 0, len(values))
	for id, totals := range values {
		name := id
		if names[id] != "" {
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
