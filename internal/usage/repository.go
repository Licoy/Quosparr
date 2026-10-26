package usage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DayBoundary chooses how events are grouped into calendar days.
type DayBoundary int

const (
	// DayLocal groups by the day in the repository's time zone.
	DayLocal DayBoundary = iota
	// DayUTC groups by UTC days (what the online analytics API reports).
	DayUTC
)

// RefreshPolicy says how aggressively the index is refreshed.
type RefreshPolicy int

const (
	// RefreshIfChanged re-reads only files whose size or time changed.
	RefreshIfChanged RefreshPolicy = iota
	// RefreshForce is a request to refresh now; the index is still
	// incremental, it only bypasses callers' own caches.
	RefreshForce
)

// RebuildPolicy says what to do when the index database on disk is
// incompatible or damaged.
type RebuildPolicy int

const (
	// RebuildAuto deletes and recreates the database, which is derived data,
	// and notes it in the summary warnings ("usage-index-rebuilt:<reason>").
	RebuildAuto RebuildPolicy = iota
	// RebuildReport leaves the database alone and fails with a
	// *RebuildRequiredError, so the caller can ask first.
	RebuildReport
)

// Query is one window to summarize.
type Query struct {
	ID          string
	SourcePath  string
	Window      Window
	DayBoundary DayBoundary
}

// PriceSource supplies the price book for a request. Implementations must be
// safe for concurrent use.
type PriceSource interface {
	PriceBook(ctx context.Context) PriceBook
}

type fixedPrices struct{ book PriceBook }

func (f fixedPrices) PriceBook(context.Context) PriceBook { return f.book }

// FixedPrices returns a PriceSource that always yields book.
func FixedPrices(book PriceBook) PriceSource { return fixedPrices{book} }

// DuplicateQueryError reports two queries with the same ID.
type DuplicateQueryError struct{ ID string }

func (e *DuplicateQueryError) Error() string { return "usage: duplicate query id " + e.ID }

// InvalidQueryWindowError reports a window whose end precedes its start.
type InvalidQueryWindowError struct{ ID string }

func (e *InvalidQueryWindowError) Error() string { return "usage: invalid window for query " + e.ID }

// Config configures a Repository. All paths are injectable; nothing reads the
// real home directory unless the caller passes it.
type Config struct {
	// CodexHome is the Codex data directory that holds sessions/ and
	// archived_sessions/ (normally ~/.codex).
	CodexHome string
	// DatabasePath is the SQLite index file (normally
	// ~/.quosparr/usage-cost-index-v1.sqlite3).
	DatabasePath string
	// Location is the time zone of local day keys. Nil means time.Local.
	Location *time.Location
	// TimeZoneID is the identifier stored with the index; a database written
	// for another identifier is rebuilt. Empty derives it from Location.
	TimeZoneID string
	// ParserVersion defaults to CurrentParserVersion.
	ParserVersion int64
	// Prices defaults to the built-in table.
	Prices PriceSource
	// Rebuild defaults to RebuildAuto.
	Rebuild RebuildPolicy

	// ChunkSize and MaxLineBytes tune the reader; zero uses the defaults.
	ChunkSize    int
	MaxLineBytes int
}

// Repository computes cost summaries from the local session logs, keeping an
// incremental SQLite index so a refresh reads only what changed. It is safe
// for concurrent use; identical concurrent requests share one scan.
type Repository struct {
	w      *worker
	prices PriceSource

	// beforeFlight, when set by tests, runs at the start of every new flight.
	beforeFlight func(ctx context.Context)

	mu              sync.Mutex
	flights         map[string]*flight
	nextFlightID    uint64
	sharedFlightHit int
	cancelledFlight int
}

type flight struct {
	id      uint64
	cancel  context.CancelFunc
	done    chan struct{}
	result  map[string]Summary
	err     error
	waiters int
}

// NewRepository validates cfg and returns a Repository. It does not touch the
// filesystem until the first request.
func NewRepository(cfg Config) (*Repository, error) {
	if cfg.CodexHome == "" || cfg.DatabasePath == "" {
		return nil, errors.New("usage: CodexHome and DatabasePath are required")
	}
	loc := cfg.Location
	if loc == nil {
		loc = time.Local
	}
	tz := cfg.TimeZoneID
	if tz == "" {
		tz = LocationID(loc)
	}
	pv := cfg.ParserVersion
	if pv == 0 {
		pv = CurrentParserVersion
	}
	prices := cfg.Prices
	if prices == nil {
		prices = FixedPrices(CurrentPriceBook())
	}
	diag := &diagState{}
	return &Repository{
		prices:  prices,
		flights: map[string]*flight{},
		w: &worker{
			codexHome:     cfg.CodexHome,
			databasePath:  cfg.DatabasePath,
			parserVersion: pv,
			timeZoneID:    tz,
			loc:           loc,
			rebuild:       cfg.Rebuild,
			stream:        newLogStream(cfg.ChunkSize, cfg.MaxLineBytes, loc),
			diag:          diag,
		},
	}, nil
}

// Close releases the database. The repository may be used again afterwards;
// it reopens the index.
func (r *Repository) Close() error { return r.w.close() }

// Diagnostics returns counters of the work done so far.
func (r *Repository) Diagnostics() Diagnostics {
	d := r.w.diag.snapshot()
	r.mu.Lock()
	d.SharedFlightHits = r.sharedFlightHit
	d.CancelledFlights = r.cancelledFlight
	r.mu.Unlock()
	return d
}

// Summaries refreshes the index and returns one Summary per query, keyed by
// query ID. The returned maps and summaries belong to the caller. ctx
// cancels this caller's wait; the shared scan itself stops only when every
// waiter has gone.
func (r *Repository) Summaries(ctx context.Context, queries []Query, calculatedAt time.Time, policy RefreshPolicy, progress *ProgressReporter) (map[string]Summary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(queries) == 0 {
		return map[string]Summary{}, nil
	}
	if err := validateQueries(queries); err != nil {
		return nil, err
	}
	book := r.prices.PriceBook(ctx)
	canonical := append([]Query{}, queries...)
	sort.SliceStable(canonical, func(i, j int) bool { return queryLess(canonical[i], canonical[j]) })
	key := requestKey(canonical, calculatedAt, policy, book.Version())

	r.mu.Lock()
	f, ok := r.flights[key]
	if ok {
		r.sharedFlightHit++
		f.waiters++
		r.mu.Unlock()
		progress.Report(ProgressPreparing(), true)
	} else {
		fctx, cancel := context.WithCancel(context.Background())
		r.nextFlightID++
		f = &flight{id: r.nextFlightID, cancel: cancel, done: make(chan struct{}), waiters: 1}
		r.flights[key] = f
		before := r.beforeFlight
		r.mu.Unlock()
		go r.runFlight(fctx, f, key, before, canonical, calculatedAt, policy, book, progress)
	}

	select {
	case <-f.done:
		r.leave(key, f, false)
		if f.err != nil {
			return nil, f.err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return cloneSummaries(f.result), nil
	case <-ctx.Done():
		r.leave(key, f, true)
		return nil, ctx.Err()
	}
}

func (r *Repository) runFlight(ctx context.Context, f *flight, key string, before func(context.Context), queries []Query, calculatedAt time.Time, policy RefreshPolicy, book PriceBook, progress *ProgressReporter) {
	defer close(f.done)
	defer f.cancel()
	if before != nil {
		before(ctx)
	}
	if err := ctx.Err(); err != nil {
		f.err = err
		return
	}
	f.result, f.err = r.w.summaries(ctx, queries, calculatedAt, policy, book, progress)
}

// leave removes one waiter; the last one to leave a still-running flight
// cancels it.
func (r *Repository) leave(key string, f *flight, cancelled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f.waiters--
	if r.flights[key] != f {
		return
	}
	if f.waiters > 0 {
		return
	}
	delete(r.flights, key)
	if cancelled {
		select {
		case <-f.done:
		default:
			f.cancel()
			r.cancelledFlight++
		}
	}
}

func cloneSummaries(in map[string]Summary) map[string]Summary {
	out := make(map[string]Summary, len(in))
	for k, v := range in {
		out[k] = v.Clone()
	}
	return out
}

func validateQueries(queries []Query) error {
	seen := make(map[string]struct{}, len(queries))
	for _, q := range queries {
		s, e := unixSeconds(q.Window.Start), unixSeconds(q.Window.End)
		if math.IsNaN(s) || math.IsInf(s, 0) || math.IsNaN(e) || math.IsInf(e, 0) || e < s {
			return &InvalidQueryWindowError{ID: q.ID}
		}
		if _, dup := seen[q.ID]; dup {
			return &DuplicateQueryError{ID: q.ID}
		}
		seen[q.ID] = struct{}{}
	}
	return nil
}

func queryLess(a, b Query) bool {
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	if !a.Window.Start.Equal(b.Window.Start) {
		return a.Window.Start.Before(b.Window.Start)
	}
	return a.Window.End.Before(b.Window.End)
}

func requestKey(queries []Query, calculatedAt time.Time, policy RefreshPolicy, pricing string) string {
	var b strings.Builder
	for _, q := range queries {
		fmt.Fprintf(&b, "%q|%d|%d|%d|%q;", q.ID, q.Window.Start.UnixNano(), q.Window.End.UnixNano(), q.DayBoundary, q.SourcePath)
	}
	b.WriteString(strconv.FormatInt(calculatedAt.UnixNano(), 10))
	fmt.Fprintf(&b, "|%d|%s", policy, pricing)
	return b.String()
}

// DefaultDatabasePath returns <home>/.quosparr/usage-cost-index-v1.sqlite3
// for an explicit home directory.
func DefaultDatabasePath(home string) string {
	return filepath.Join(home, ".quosparr", "usage-cost-index-v1.sqlite3")
}

// LocationID returns a stable identifier for loc: its IANA name, or for
// time.Local the zone the operating system is configured with (TZ, or the
// /etc/localtime link target). It falls back to "Local" when unknown.
func LocationID(loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	if name := loc.String(); name != "Local" && name != "" {
		return name
	}
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" && tz != "Local" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(target, "zoneinfo/"); i >= 0 {
			return target[i+len("zoneinfo/"):]
		}
	}
	return "Local"
}
