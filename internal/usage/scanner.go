package usage

import (
	"context"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/usage/decimal"
)

// ScanDiagnostics counts the work of one Scanner pass.
type ScanDiagnostics struct {
	BytesRead               int
	CandidateLines          int
	DecodedLines            int
	MaxBufferedBytes        int
	CandidateFiles          int
	MalformedCandidateLines int
	OversizedLines          int
}

// Scanner streams the session logs once, without any index. It is the simple
// reference path (and the oracle the indexed Repository is tested against):
// every call reads every candidate file in full.
//
// Unlike the Repository it does not deduplicate a session file that exists
// under both sessions/ and archived_sessions/.
type Scanner struct {
	// CodexHome holds sessions/ and archived_sessions/.
	CodexHome string
	// Location is the time zone of local day keys and of the dated-path
	// pre-filter. Nil means time.Local.
	Location *time.Location
	// Prices defaults to the built-in table.
	Prices PriceBook
	// ChunkSize and MaxLineBytes tune the reader; zero uses the defaults.
	ChunkSize    int
	MaxLineBytes int
	// Cache reuses window-independent cache-hit samples across range changes.
	Cache *CacheScanCache
}

func (s Scanner) loc() *time.Location {
	if s.Location == nil {
		return time.Local
	}
	return s.Location
}

func (s Scanner) book() PriceBook {
	if s.Prices.valid() {
		return s.Prices
	}
	return CurrentPriceBook()
}

func (s Scanner) stream() *logStream {
	return newLogStream(s.ChunkSize, s.MaxLineBytes, s.loc())
}

// Scan totals the token usage of window per model.
func (s Scanner) Scan(ctx context.Context, w Window) (CostSummary, error) {
	out, _, err := s.ScanReport(ctx, w)
	return out, err
}

// ScanReport is Scan plus the work counters.
func (s Scanner) ScanReport(ctx context.Context, w Window) (CostSummary, ScanDiagnostics, error) {
	files, err := s.files(ctx, w)
	if err != nil {
		return CostSummary{}, ScanDiagnostics{}, err
	}
	book := s.book()
	byModel := map[string]TokenUsage{}
	costs := map[string]decimal.Decimal{}
	unknown := map[string]struct{}{}
	stream := s.stream()
	diag := ScanDiagnostics{CandidateFiles: len(files)}
	for _, file := range files {
		current := unknownModel
		res, err := stream.read(ctx, readRequest{Path: file}, func(l parsedLine) error {
			rec := l.Record
			if rec.ContextModel != nil {
				current = *rec.ContextModel
			}
			if !w.Contains(rec.Timestamp) || rec.Usage == nil {
				return nil
			}
			model := current
			if rec.Model != nil {
				model = *rec.Model
			}
			sum, err := byModel[model].add(*rec.Usage)
			if err != nil {
				return err
			}
			byModel[model] = sum
			tokens, err := totalsFromUsage(*rec.Usage, 0, 0)
			if err != nil {
				return err
			}
			cost, ok := book.Cost(model, tokens)
			if ok {
				costs[model] = costs[model].Add(cost)
			} else if tokens.HasTokenParts() {
				unknown[model] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return CostSummary{}, diag, err
		}
		diag.record(res)
	}
	models := sortedKeys(byModel)
	breakdown := make([]ModelCostBreakdown, 0, len(models))
	total := decimal.Zero
	var usages []TokenUsage
	for _, m := range models {
		u := byModel[m]
		usages = append(usages, u)
		cost := costs[m]
		total = total.Add(cost)
		breakdown = append(breakdown, ModelCostBreakdown{Model: m, Usage: u, EstimatedUSD: cost})
	}
	var totals TokenUsage
	for _, u := range usages {
		if totals, err = totals.add(u); err != nil {
			return CostSummary{}, diag, err
		}
	}
	unknownList := make([]string, 0, len(unknown))
	for m := range unknown {
		unknownList = append(unknownList, m)
	}
	sort.Strings(unknownList)
	return CostSummary{
		Window:         w,
		Totals:         totals,
		ModelBreakdown: breakdown,
		EstimatedUSD:   total,
		PricingVersion: book.Version(),
		UnknownModels:  unknownList,
	}, diag, nil
}

// ScanAPIEquivalent summarizes window the way Repository.Summaries does, but
// straight from the files.
func (s Scanner) ScanAPIEquivalent(ctx context.Context, w Window, calculatedAt time.Time) (Summary, error) {
	out, _, err := s.ScanAPIEquivalentReport(ctx, w, calculatedAt)
	return out, err
}

// ScanAPIEquivalentReport is ScanAPIEquivalent plus the work counters.
func (s Scanner) ScanAPIEquivalentReport(ctx context.Context, w Window, calculatedAt time.Time) (Summary, ScanDiagnostics, error) {
	files, err := s.files(ctx, w)
	if err != nil {
		return Summary{}, ScanDiagnostics{}, err
	}
	book := s.book()
	g := newEventGroups()
	stream := s.stream()
	diag := ScanDiagnostics{CandidateFiles: len(files)}
	add := func(t Totals, model, project, day string) error {
		if err := g.add(model, project, day, t); err != nil {
			return err
		}

		return nil
	}
	for _, file := range files {
		model, project := unknownModel, UnknownProject
		res, err := stream.read(ctx, readRequest{Path: file}, func(l parsedLine) error {
			rec := l.Record
			if rec.SessionCWD != nil {
				project = ProjectDisplayName(*rec.SessionCWD)
			}
			if rec.ContextModel != nil {
				model = *rec.ContextModel
			}
			if !w.Contains(rec.Timestamp) {
				return nil
			}
			m := model
			if rec.Model != nil {
				m = *rec.Model
			}
			var turns int64
			if rec.CountsAsTurn {
				turns = 1
			}
			switch {
			case rec.Usage != nil:
				t, err := totalsFromUsage(*rec.Usage, turns, 0)
				if err != nil {
					return err
				}
				return add(t, m, project, rec.DayKey)
			case rec.CountsAsTurn:
				return add(Totals{Turns: 1}, m, project, rec.DayKey)
			}
			return nil
		})
		if err != nil {
			return Summary{}, diag, err
		}
		diag.record(res)
	}
	var warnings []string
	if diag.OversizedLines > 0 {
		warnings = append(warnings, "oversized-jsonl-lines:"+strconv.Itoa(diag.OversizedLines))
	}
	out, err := summaryFromGroups(g, w, calculatedAt, book, nil, warnings)
	return out, diag, err
}

func (d *ScanDiagnostics) record(r streamResult) {
	d.BytesRead += r.BytesRead
	d.CandidateLines += r.CandidateLines
	d.DecodedLines += r.DecodedLines
	d.MalformedCandidateLines += r.MalformedLines
	d.OversizedLines += r.OversizedLines
	d.MaxBufferedBytes = max(d.MaxBufferedBytes, r.MaxBufferedBytes)
}

// files lists the candidate session files whose dated path or modification
// time is near the window.
func (s Scanner) files(ctx context.Context, w Window) ([]string, error) {
	sources, err := listSources(ctx, s.CodexHome)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range sources {
		if s.IsLikelyRelevant(f.Path, w) {
			out = append(out, f.Path)
		}
	}
	return out, nil
}

// IsLikelyRelevant is the cheap pre-filter on a session file: a date in its
// path (.../2026/06/29/... or a YYYY-MM-DD in the name) must lie within one
// calendar day of the window; without a date the modification time must.
func (s Scanner) IsLikelyRelevant(path string, w Window) bool {
	loc := s.loc()
	if day, ok := dayFromPath(path, loc); ok {
		start := w.Start.In(loc).AddDate(0, 0, -1)
		end := w.End.In(loc).AddDate(0, 0, 1)
		return !day.Before(start) && !day.After(end)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return true
	}
	mod := fi.ModTime()
	return !mod.Before(w.Start.Add(-24*time.Hour)) && !mod.After(w.End.Add(24*time.Hour))
}

var nameDate = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}`)

// dayFromPath finds the date a session path encodes. Like the system
// calendar it accepts out-of-range month and day numbers and rolls them over
// for a YYYY/MM/DD directory triple; a YYYY-MM-DD in the file name must have a
// month of 1...12 and a day of 1...31, and means UTC midnight.
func dayFromPath(path string, loc *time.Location) (time.Time, bool) {
	comps := strings.Split(strings.ReplaceAll(path, `\`, "/"), "/")
	for i := 0; i+2 < len(comps); i++ {
		if len([]rune(comps[i])) != 4 {
			continue
		}
		y, err1 := strconv.Atoi(comps[i])
		m, err2 := strconv.Atoi(comps[i+1])
		d, err3 := strconv.Atoi(comps[i+2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		return time.Date(y, time.Month(m), d, 0, 0, 0, 0, loc), true
	}
	name := comps[len(comps)-1]
	match := nameDate.FindString(name)
	if match == "" {
		return time.Time{}, false
	}
	y, _ := strconv.Atoi(match[0:4])
	m, _ := strconv.Atoi(match[5:7])
	d, _ := strconv.Atoi(match[8:10])
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), true
}
