package usage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RebuildRequiredError reports that the index database on disk cannot be
// used and, under RebuildReport, was left untouched.
type RebuildRequiredError struct {
	// Reason is one of the Reason* constants.
	Reason string
	Path   string
}

func (e *RebuildRequiredError) Error() string {
	return "usage: index database needs a rebuild (" + e.Reason + "): " + e.Path
}

// worker owns the index store and runs one scan at a time.
type worker struct {
	codexHome     string
	databasePath  string
	parserVersion int64
	timeZoneID    string
	loc           *time.Location
	rebuild       RebuildPolicy
	stream        *logStream
	diag          *diagState

	mu     sync.Mutex
	store  *indexStore
	active int
}

type openedStore struct {
	store    *indexStore
	warnings []string
}

func (w *worker) summaries(ctx context.Context, queries []Query, calculatedAt time.Time, policy RefreshPolicy, book PriceBook, progress *ProgressReporter) (map[string]Summary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.active++
	w.diag.update(func(d *Diagnostics) { d.MaxConcurrentScans = max(d.MaxConcurrentScans, w.active) })
	defer func() { w.active-- }()

	out, err := w.calculate(ctx, queries, calculatedAt, policy, book, nil, progress)
	if err == nil {
		return out, nil
	}
	rebuild, rerr := w.shouldRebuild(ctx, err)
	if rerr != nil {
		return nil, rerr
	}
	if !rebuild {
		return nil, err
	}
	rebuilt, rerr := w.rebuildStore(ctx, ReasonDatabaseCorrupt)
	if rerr != nil {
		return nil, rerr
	}
	return w.calculate(ctx, queries, calculatedAt, policy, book, &rebuilt, progress)
}

func (w *worker) calculate(ctx context.Context, queries []Query, calculatedAt time.Time, policy RefreshPolicy, book PriceBook, supplied *openedStore, progress *ProgressReporter) (map[string]Summary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	progress.Report(ProgressPreparing(), true)
	var opened openedStore
	if supplied != nil {
		opened = *supplied
	} else {
		var err error
		if opened, err = w.openStore(ctx); err != nil {
			return nil, err
		}
	}
	refresher := &indexRefresher{
		codexHome: w.codexHome,
		store:     opened.store,
		indexer:   newSourceIndexer(opened.store, w.parserVersion, w.stream, w.diag),
		diag:      w.diag,
	}
	_ = policy // both policies check file metadata; Force differs only for callers that cache above us
	refreshWarnings, err := refresher.refresh(ctx, progress)
	if err != nil {
		return nil, err
	}
	if err := opened.store.validateEventStorage(ctx); err != nil {
		return nil, err
	}
	warnings := append(append([]string{}, opened.warnings...), refreshWarnings...)
	result := make(map[string]Summary, len(queries))
	for i, q := range queries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		progress.Report(ProgressAggregating(i, len(queries)), true)
		boundary := dayLocal
		if q.DayBoundary == DayUTC {
			boundary = dayUTC
		}
		events, err := opened.store.events(ctx, unixSeconds(q.Window.Start), unixSeconds(q.Window.End), boundary, q.SourcePath)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s, err := buildSummary(events, q.Window, calculatedAt, book, warnings)
		if err != nil {
			return nil, err
		}
		result[q.ID] = s
	}
	if len(queries) > 0 {
		progress.Report(ProgressAggregating(len(queries), len(queries)), true)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (w *worker) openStore(ctx context.Context) (openedStore, error) {
	if w.store != nil {
		if w.store.timeZoneID == w.timeZoneID {
			return openedStore{store: w.store}, nil
		}
		if w.rebuild == RebuildReport {
			return openedStore{}, &RebuildRequiredError{Reason: ReasonTimeZone, Path: w.databasePath}
		}
		return w.rebuildStore(ctx, ReasonTimeZone)
	}
	if err := os.MkdirAll(filepath.Dir(w.databasePath), 0o700); err != nil {
		return openedStore{}, err
	}
	s, err := openIndexStore(ctx, w.databasePath, w.parserVersion, w.timeZoneID)
	if err == nil {
		w.store = s
		return openedStore{store: s}, nil
	}
	var mm *indexMismatch
	switch {
	case errors.As(err, &mm):
		return w.rebuildOrReport(ctx, mm.Reason)
	case isUnambiguousCorruption(err):
		return w.rebuildOrReport(ctx, ReasonDatabaseCorrupt)
	}
	return openedStore{}, err
}

func (w *worker) rebuildOrReport(ctx context.Context, reason string) (openedStore, error) {
	if w.rebuild == RebuildReport {
		return openedStore{}, &RebuildRequiredError{Reason: reason, Path: w.databasePath}
	}
	return w.rebuildStore(ctx, reason)
}

// rebuildStore deletes the database files and creates an empty index.
func (w *worker) rebuildStore(ctx context.Context, reason string) (openedStore, error) {
	if w.rebuild == RebuildReport {
		return openedStore{}, &RebuildRequiredError{Reason: reason, Path: w.databasePath}
	}
	if w.store != nil {
		w.store.Close()
		w.store = nil
	}
	for _, p := range []string{w.databasePath + "-wal", w.databasePath + "-shm", w.databasePath} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return openedStore{}, err
		}
	}
	s, err := openIndexStore(ctx, w.databasePath, w.parserVersion, w.timeZoneID)
	if err != nil {
		return openedStore{}, err
	}
	w.store = s
	w.diag.update(func(d *Diagnostics) { d.DatabaseRebuilds++ })
	return openedStore{store: s, warnings: []string{"usage-index-rebuilt:" + reason}}, nil
}

// shouldRebuild decides whether err means the stored index is unusable, as
// opposed to a transient failure or a cancelled scan.
func (w *worker) shouldRebuild(ctx context.Context, err error) (bool, error) {
	var mm *indexMismatch
	if errors.As(err, &mm) {
		return w.checkReport(ReasonDatabaseCorrupt)
	}
	if isUnambiguousCorruption(err) {
		return w.checkReport(ReasonDatabaseCorrupt)
	}
	if !isSchemaError(err) || w.store == nil {
		return false, nil
	}
	valid, verr := w.store.storageIsValid(ctx)
	if verr != nil {
		if isUnambiguousCorruption(verr) {
			return w.checkReport(ReasonDatabaseCorrupt)
		}
		return false, nil
	}
	if valid {
		return false, nil
	}
	return w.checkReport(ReasonDatabaseCorrupt)
}

func (w *worker) checkReport(reason string) (bool, error) {
	if w.rebuild == RebuildReport {
		return false, &RebuildRequiredError{Reason: reason, Path: w.databasePath}
	}
	return true, nil
}

func (w *worker) close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.store == nil {
		return nil
	}
	err := w.store.Close()
	w.store = nil
	return err
}
