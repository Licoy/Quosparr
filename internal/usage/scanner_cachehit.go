package usage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/cachehit"
)

// CacheScanCache keeps window-independent parsed samples for ScanCache.
type CacheScanCache struct {
	mu    sync.Mutex
	files map[string]cacheScanFile
}

type cacheScanFile struct {
	meta      fileMeta
	samples   []cacheScanSample
	sessionID string
	// unread counts the complete lines that may hold usage but could not be
	// read; they are skipped.
	unread int
}

type cacheScanSample struct {
	at     float64
	model  string
	input  int64
	cached int64
	output int64
	known  bool
}

// ScanCache summarizes cache reads from all deduplicated local Codex logs in
// the half-open range [window.Start, window.End).
func (s Scanner) ScanCache(ctx context.Context, window Window) (cachehit.Report, error) {
	if !window.End.After(window.Start) {
		return cachehit.Report{}, fmt.Errorf("usage: invalid cache-hit window")
	}
	if err := ctx.Err(); err != nil {
		return cachehit.Report{}, err
	}
	sources, err := s.cacheSources(ctx)
	if err != nil {
		return cachehit.Report{}, err
	}
	report := cachehit.Report{Start: window.Start, End: window.End, CalculatedAt: time.Now()}
	models, sessions := map[string]cachehit.Totals{}, map[string]cachehit.Totals{}
	names, err := readCacheSessionNames(ctx, filepath.Join(s.CodexHome, "session_index.jsonl"))
	if err != nil {
		return cachehit.Report{}, err
	}
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return cachehit.Report{}, err
		}
		parsed, err := s.cacheSamples(ctx, src)
		if err != nil {
			return cachehit.Report{}, err
		}
		// A log last modified before the range cannot hold a line of it.
		if parsed.unread > 0 && parsed.meta.MtimeNS >= window.Start.UnixNano() {
			report.UnreadLines += parsed.unread
		}
		id := strings.TrimSuffix(src.Basename, ".jsonl")
		if parsed.sessionID != "" {
			id = parsed.sessionID
		}
		for _, sample := range parsed.samples {
			if err := ctx.Err(); err != nil {
				return cachehit.Report{}, err
			}
			if sample.at < unixSeconds(window.Start) || sample.at >= unixSeconds(window.End) {
				continue
			}
			model := sample.model
			if model == "" {
				model = unknownModel
			}
			mt, st := models[model], sessions[id]
			if sample.known {
				if err := report.Totals.AddSample(sample.input, sample.cached); err != nil {
					return cachehit.Report{}, fmt.Errorf("usage: %w", err)
				}
				if err := mt.AddSample(sample.input, sample.cached); err != nil {
					return cachehit.Report{}, fmt.Errorf("usage: %w", err)
				}
				if err := st.AddSample(sample.input, sample.cached); err != nil {
					return cachehit.Report{}, fmt.Errorf("usage: %w", err)
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
			models[model], sessions[id] = mt, st
		}
	}
	report.Models = usageCacheRows(models)
	report.Sessions = usageCacheNamedRows(sessions, names, true)
	return report, nil
}

func (s Scanner) cacheSources(ctx context.Context) ([]sourceFile, error) {
	files, err := listSources(ctx, s.CodexHome)
	if err != nil {
		return nil, err
	}
	groups := map[string][]sourceFile{}
	for _, f := range files {
		groups[f.Basename] = append(groups[f.Basename], f)
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]sourceFile, 0, len(names))
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		group := groups[name]
		chosen := group[0]
		if len(group) > 1 {
			first, err := hashFull(ctx, chosen)
			if err != nil {
				return nil, err
			}
			for _, candidate := range group[1:] {
				next, err := hashFull(ctx, candidate)
				if err != nil {
					return nil, err
				}
				if !bytes.Equal(first.Digest[:], next.Digest[:]) {
					return nil, &DuplicateConflictError{Basename: name}
				}
			}
		}
		out = append(out, chosen)
	}
	return out, nil
}

func (s Scanner) cacheSamples(ctx context.Context, src sourceFile) (cacheScanFile, error) {
	if s.Cache != nil {
		s.Cache.mu.Lock()
		entry, ok := s.Cache.files[src.Path]
		s.Cache.mu.Unlock()
		if ok && entry.meta.sameSnapshot(src.meta) {
			return entry, nil
		}
	}
	current := unknownModel
	var samples []cacheScanSample
	var previousTotal *TokenUsage
	var sessionID string
	// logStream snapshots the size at open and reads only that prefix. Appends
	// remain for the next scan and do not invalidate this complete snapshot.
	stream := s.stream()
	stream.parser.cacheFields = true
	res, err := stream.read(ctx, readRequest{Path: src.Path, Expected: &src, IgnoreOversized: codexCacheIgnorable}, func(line parsedLine) error {
		rec := line.Record
		if rec.SessionID != nil && sessionID == "" {
			sessionID = strings.TrimSpace(*rec.SessionID)
		}
		if rec.ContextModel != nil {
			current = *rec.ContextModel
		}
		if rec.Usage == nil {
			return nil
		}
		if rec.Timestamp == distantPast {
			return fmt.Errorf("usage: cache-hit sample has no valid timestamp")
		}
		model := current
		if rec.Model != nil {
			model = *rec.Model
		}
		sample := cacheScanSample{at: rec.Timestamp, model: model, input: rec.Usage.InputTokens, cached: rec.Usage.CachedInputTokens, output: rec.Usage.OutputTokens, known: rec.CacheKnown}
		if rec.TotalUsage != nil && rec.TotalUsage.input != nil && rec.TotalUsage.cached != nil && rec.TotalUsage.output != nil {
			total, err := rec.TotalUsage.decoded()
			if err != nil {
				return err
			}
			if previousTotal != nil && total == *previousTotal {
				return nil
			}
			previousTotal = &total
		} else {
			previousTotal = nil
		}
		samples = append(samples, sample)
		return nil
	})
	if err != nil {
		return cacheScanFile{}, err
	}
	// Unreadable lines are skipped and counted, so one damaged log does not
	// hide every other one; a trailing line may still be being written.
	unread := res.MalformedLines - res.IncompleteMalformed + res.OversizedLines - res.IncompleteOversized - res.IgnoredOversized
	entry := cacheScanFile{meta: res.Meta, samples: samples, sessionID: sessionID, unread: unread}
	if s.Cache != nil {
		s.Cache.mu.Lock()
		if s.Cache.files == nil {
			s.Cache.files = map[string]cacheScanFile{}
		}
		s.Cache.files[src.Path] = entry
		s.Cache.mu.Unlock()
	}
	return entry, nil
}

// codexCacheIgnorable tells from its start that an oversized line is a model
// item, a compaction summary or an event other than token_count, none of which
// the cache scan reads.
func codexCacheIgnorable(head []byte) bool {
	switch typ, _ := HeadString(head, "type"); typ {
	case "response_item", "compacted":
		return true
	case "event_msg":
		kind, ok := HeadString(head, "payload", "type")
		return ok && kind != "token_count"
	}
	return false
}

func usageCacheRows(values map[string]cachehit.Totals) []cachehit.Row {
	return usageCacheNamedRows(values, nil, false)
}

func usageCacheNamedRows(values map[string]cachehit.Totals, names map[string]string, shorten bool) []cachehit.Row {
	rows := make([]cachehit.Row, 0, len(values))
	for id, totals := range values {
		name := id
		if shorten && len(name) > 12 {
			name = name[:8]
		}
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

func readCacheSessionNames(ctx context.Context, path string) (map[string]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names := map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, defaultMaxLineBytes)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var row struct {
			ID   string `json:"id"`
			Name string `json:"thread_name"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			continue
		}
		if row.ID != "" && row.Name != "" {
			names[row.ID] = row.Name
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return names, nil
}
