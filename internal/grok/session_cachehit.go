package grok

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/cachehit"
	"github.com/licoy/quosparr/internal/usage"
)

const grokCacheMaxLine = 8 << 20

var (
	errGrokCacheSyntax = errors.New("grok: malformed cache-hit log line")
	// errGrokNotTurn marks a line that only mentions the turn markers.
	errGrokNotTurn = errors.New("grok: log line is not a completed turn")
)

// CacheScanCache keeps window-independent parsed turns for ScanCache.
type CacheScanCache struct {
	mu    sync.Mutex
	files map[string]grokCacheFile
}

type grokCacheFile struct {
	info          os.FileInfo
	fallbackModel string
	turns         []grokCacheTurn
	// unread counts the complete lines that may hold a turn but could not be
	// read; they are skipped.
	unread int
}

type grokCacheSample struct {
	model         string
	input, cached int64
	known         bool
}

type grokCacheTurn struct {
	at     float64
	total  grokCacheSample
	models []grokCacheSample
}

// ScanCache summarizes cache reads from every local Grok session in the
// half-open range [window.Start, window.End).
func (s SessionScanner) ScanCache(ctx context.Context, window usage.Window, now time.Time) (cachehit.Report, error) {
	if !window.End.After(window.Start) {
		return cachehit.Report{}, fmt.Errorf("grok: invalid cache-hit window")
	}
	if err := ctx.Err(); err != nil {
		return cachehit.Report{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	summaries, err := s.listSummaries(ctx, math.MaxInt)
	if err != nil {
		return cachehit.Report{}, err
	}
	report := cachehit.Report{Start: window.Start, End: window.End, CalculatedAt: now}
	models, sessions := map[string]cachehit.Totals{}, map[string]cachehit.Totals{}
	names := map[string]string{}
	for _, summary := range summaries {
		if err := ctx.Err(); err != nil {
			return cachehit.Report{}, err
		}
		file, err := s.grokCacheLog(ctx, filepath.Join(summary.directory, "updates.jsonl"), summary.model)
		if err != nil {
			return cachehit.Report{}, err
		}
		// A log last modified before the range cannot hold a line of it.
		if file.unread > 0 && !file.info.ModTime().Before(window.Start) {
			report.UnreadLines += file.unread
		}
		for _, turn := range file.turns {
			if err := ctx.Err(); err != nil {
				return cachehit.Report{}, err
			}
			if turn.at < float64(window.Start.UnixNano())/1e9 || turn.at >= float64(window.End.UnixNano())/1e9 {
				continue
			}
			st := sessions[summary.id]
			if err := addGrokCache(&report.Totals, turn.total); err != nil {
				return cachehit.Report{}, err
			}
			if err := addGrokCache(&st, turn.total); err != nil {
				return cachehit.Report{}, err
			}
			sessions[summary.id], names[summary.id] = st, summary.title
			for _, sample := range turn.models {
				model := sample.model
				if model == "" {
					model = "unknown"
				}
				mt := models[model]
				if err := addGrokCache(&mt, sample); err != nil {
					return cachehit.Report{}, err
				}
				models[model] = mt
			}
		}
	}
	report.Models = grokCacheRows(models, nil)
	report.Sessions = grokCacheRows(sessions, names)
	return report, nil
}

func addGrokCache(t *cachehit.Totals, sample grokCacheSample) error {
	if !sample.known {
		return t.AddMissing()
	}
	if err := t.AddSample(sample.input, sample.cached); err != nil {
		return fmt.Errorf("grok: %w", err)
	}
	return nil
}

// grokCacheLog parses the turns of one updates log. Unreadable lines are
// skipped and counted, so one damaged log does not hide every other one.
func (s SessionScanner) grokCacheLog(ctx context.Context, path string, fallbackModel *string) (grokCacheFile, error) {
	model := ""
	if fallbackModel != nil {
		model = *fallbackModel
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return grokCacheFile{}, nil
	}
	if err != nil {
		return grokCacheFile{}, err
	}
	if s.Cache != nil {
		s.Cache.mu.Lock()
		entry, ok := s.Cache.files[path]
		s.Cache.mu.Unlock()
		if ok && entry.fallbackModel == model && os.SameFile(entry.info, info) && entry.info.Size() == info.Size() && entry.info.ModTime().Equal(info.ModTime()) {
			return entry, nil
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return grokCacheFile{}, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return grokCacheFile{}, err
	}
	br := bufio.NewReader(&io.LimitedReader{R: f, N: opened.Size()})
	seen := map[string]struct{}{}
	file := grokCacheFile{info: opened, fallbackModel: model}
	for {
		if err := ctx.Err(); err != nil {
			return grokCacheFile{}, err
		}
		line, complete, tooLong, err := readGrokCacheLine(br, grokCacheMaxLine)
		if err != nil && err != io.EOF {
			return grokCacheFile{}, err
		}
		if !complete && tooLong {
			break
		}
		line = bytes.TrimSpace(line)
		switch {
		case tooLong:
			if !grokCacheIgnorable(line) {
				file.unread++
			}
		case len(line) > 0 && bytes.Contains(line, sessionTurnMarker) && bytes.Contains(line, sessionUsageMarker):
			key := string(line)
			if _, ok := seen[key]; ok {
				break
			}
			turn, parseErr := parseGrokCacheTurn(line, fallbackModel)
			switch {
			case parseErr == nil:
				seen[key] = struct{}{}
				file.turns = append(file.turns, turn)
			case errors.Is(parseErr, errGrokNotTurn):
				// Another update whose text only mentions the turn markers.
			case errors.Is(parseErr, errGrokCacheSyntax):
				// A final line without newline may still be being written.
				if complete {
					file.unread++
				}
			default:
				return grokCacheFile{}, parseErr
			}
		}
		if !complete || err == io.EOF {
			break
		}
	}
	if s.Cache != nil {
		s.Cache.mu.Lock()
		if s.Cache.files == nil {
			s.Cache.files = map[string]grokCacheFile{}
		}
		s.Cache.files[path] = file
		s.Cache.mu.Unlock()
	}
	return file, nil
}

// grokCacheIgnorable tells from its start that an oversized line is an update
// other than a completed turn, so it has no usage.
func grokCacheIgnorable(head []byte) bool {
	kind, ok := usage.HeadString(head, "params", "update", "sessionUpdate")
	return ok && kind != "turn_completed"
}

func readGrokCacheLine(br *bufio.Reader, max int) ([]byte, bool, bool, error) {
	var line []byte
	tooLong := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !tooLong {
			if len(line)+len(chunk) > max {
				tooLong, line = true, usage.LineHead(line, chunk)
			} else {
				line = append(line, chunk...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		complete := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		return line, complete, tooLong, err
	}
}

func parseGrokCacheTurn(line []byte, fallbackModel *string) (grokCacheTurn, error) {
	obj, err := parseLooseJSON(line)
	if err != nil {
		return grokCacheTurn{}, errGrokCacheSyntax
	}
	if obj.kind != jsonObject {
		return grokCacheTurn{}, fmt.Errorf("grok: invalid cache-hit log record")
	}
	update := obj.get("params").get("update")
	if kind, _ := update.get("sessionUpdate").str(); update == nil || kind != "turn_completed" {
		return grokCacheTurn{}, errGrokNotTurn
	}
	usageNode := update.get("usage")
	if usageNode == nil || usageNode.kind != jsonObject {
		return grokCacheTurn{}, fmt.Errorf("grok: cache-hit turn has no usage")
	}
	at, ok := grokCacheTimestamp(obj)
	if !ok {
		return grokCacheTurn{}, fmt.Errorf("grok: cache-hit turn has no valid timestamp")
	}
	total, err := grokCacheCounters(usageNode, "")
	if err != nil {
		return grokCacheTurn{}, err
	}
	turn := grokCacheTurn{at: at, total: total}
	modelUsage := usageNode.get("modelUsage")
	if modelUsage != nil && modelUsage.kind == jsonObject {
		keys := make([]string, 0, len(modelUsage.obj))
		for model := range modelUsage.obj {
			keys = append(keys, model)
		}
		sort.Strings(keys)
		for _, model := range keys {
			node := modelUsage.obj[model]
			if node.kind != jsonObject {
				return grokCacheTurn{}, fmt.Errorf("grok: invalid model usage for %s", model)
			}
			sample, err := grokCacheCounters(node, model)
			if err != nil {
				return grokCacheTurn{}, err
			}
			turn.models = append(turn.models, sample)
		}
	}
	if len(turn.models) == 0 {
		model := ""
		if fallbackModel != nil {
			model = *fallbackModel
		}
		sample := total
		sample.model = model
		turn.models = []grokCacheSample{sample}
	}
	return turn, nil
}

func grokCacheTimestamp(obj *jsonNode) (float64, bool) {
	if at, ok := grokCacheNumber(obj.get("timestamp")); ok {
		return at, true
	}
	meta := obj.get("params").get("_meta")
	for _, key := range []string{"agentTimestampMs", "streamStartMs", "turnStartMs"} {
		if ms, ok := grokCacheNumber(meta.get(key)); ok {
			return ms / 1000, true
		}
	}
	return 0, false
}

func grokCacheNumber(node *jsonNode) (float64, bool) {
	if node == nil || node.kind != jsonNumber {
		return 0, false
	}
	value := billingNumberDouble(node.s)
	return value, !math.IsNaN(value) && !math.IsInf(value, 0)
}

func grokCacheCounters(node *jsonNode, model string) (grokCacheSample, error) {
	inNode, cachedNode := node.get("inputTokens"), node.get("cachedReadTokens")
	if inNode == nil || cachedNode == nil {
		return grokCacheSample{model: model}, nil
	}
	if inNode.kind != jsonNumber || cachedNode.kind != jsonNumber {
		return grokCacheSample{}, fmt.Errorf("grok: invalid cache-hit token counters")
	}
	input, ok1 := billingNumberInt64(inNode.s)
	cached, ok2 := billingNumberInt64(cachedNode.s)
	if !ok1 || !ok2 || input < 0 || cached < 0 || cached > input {
		return grokCacheSample{}, fmt.Errorf("grok: invalid cache-hit token counters")
	}
	return grokCacheSample{model: model, input: input, cached: cached, known: true}, nil
}

func grokCacheRows(values map[string]cachehit.Totals, names map[string]string) []cachehit.Row {
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
