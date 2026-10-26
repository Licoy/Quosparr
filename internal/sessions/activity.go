package sessions

import (
	"encoding/json"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/export/swiftjson"
)

const (
	// Only the first 512 KiB and the last 64 KiB of a larger file are read:
	// a session file can be hundreds of megabytes, and the recent list needs
	// the identity from the head and the latest activity and token totals
	// (total_token_usage is cumulative) from the tail.
	headProbeBytes = 512 * 1024
	tailProbeBytes = 64 * 1024
	// A small file that showed no cumulative usage is read in full and its
	// per-turn last_token_usage events are added up instead.
	smallFullScanBytes = 256 * 1024

	unknownModel = "unknown-model"
)

// Scanner builds the recent sessions list from a Codex home (~/.codex).
type Scanner struct {
	CodexHome string
	// Pricer prices each observed request; cumulative-only usage stays unpriced.
	Pricer Pricer
}

// Scan returns the limit most recently active sessions. It looks at the newest
// max(3*limit, 15) session files by modification time, plus the files of the
// newest limit-ish index entries, so a session the index lists is found even
// when many newer files exist.
func (s Scanner) Scan(limit int) (Summary, error) {
	if limit < 0 {
		limit = 0
	}
	if limit == 0 {
		return Summary{Items: []Item{}}, nil
	}
	index, err := readIndex(s.CodexHome)
	if err != nil {
		return Summary{}, err
	}
	titles := map[string]string{}
	for _, e := range index {
		titles[e.ID] = e.ThreadName
	}
	latest := map[string]Item{}
	for _, file := range s.candidateFiles(index, max(limit*3, 15)) {
		item, ok, err := s.parseSession(file)
		if err != nil {
			return Summary{}, err
		}
		if !ok {
			continue
		}
		if title := cleanTitle(titles[item.ID]); title != "" {
			item.Title = title
		}
		if existing, found := latest[item.ID]; found && !existing.UpdatedAt.Before(item.UpdatedAt) {
			continue
		}
		latest[item.ID] = item
	}
	items := make([]Item, 0, len(latest))
	for _, item := range latest {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return Summary{Items: items}, nil
}

type fileCandidate struct {
	path         string
	id           string
	activityDate time.Time
}

func (c fileCandidate) newerThan(o fileCandidate) bool {
	if c.activityDate.Equal(o.activityDate) {
		return c.path < o.path
	}
	return c.activityDate.After(o.activityDate)
}

func (s Scanner) candidateFiles(index []indexEntry, limit int) []string {
	files := jsonlFiles(s.CodexHome)
	byID := map[string]fileCandidate{}
	for _, f := range files {
		if f.id == "" {
			continue
		}
		if existing, ok := byID[f.id]; ok && !existing.activityDate.Before(f.activityDate) {
			continue
		}
		byID[f.id] = f
	}
	var result []string
	seen := map[string]bool{}
	appendFile := func(f fileCandidate) {
		if !seen[f.path] {
			seen[f.path] = true
			result = append(result, f.path)
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].newerThan(files[j]) })
	for i := 0; i < len(files) && i < limit; i++ {
		appendFile(files[i])
	}
	byUpdate := append([]indexEntry(nil), index...)
	sort.SliceStable(byUpdate, func(i, j int) bool { return byUpdate[i].UpdatedAt.After(byUpdate[j].UpdatedAt) })
	for i := 0; i < len(byUpdate) && i < limit; i++ {
		if f, ok := byID[byUpdate[i].ID]; ok {
			appendFile(f)
		}
	}
	return result
}

// jsonlFiles lists the *.jsonl files under sessions/ and archived_sessions/.
func jsonlFiles(codexHome string) []fileCandidate {
	var files []fileCandidate
	for _, folder := range []string{"sessions", "archived_sessions"} {
		_ = filepath.WalkDir(filepath.Join(codexHome, folder), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			files = append(files, fileCandidate{
				path:         path,
				id:           sessionID(filepath.Base(path)),
				activityDate: fileActivityDate(path),
			})
			return nil
		})
	}
	return files
}

func fileActivityDate(path string) time.Time {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	if day, ok := dayFromPath(path); ok {
		return day
	}
	return time.Time{}
}

// parseState is what one pass over a session file collects.
type parseState struct {
	id           string
	cwd          *string
	title        string
	updatedAt    time.Time
	hasUpdatedAt bool
	state        State
	currentModel string
	byModel      map[string]Totals
	turnCount    int
	requests     []sessionRequest
}

func (s Scanner) parseSession(path string) (Item, bool, error) {
	st := &parseState{state: StateRecent, currentModel: unknownModel, byModel: map[string]Totals{}}
	size := int64(math.MaxInt64)
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	lines, err := probeLines(path, size)
	if err != nil {
		return Item{}, false, err
	}
	for _, line := range lines {
		if err := st.apply(line, false); err != nil {
			return Item{}, false, err
		}
	}
	if len(st.byModel) == 0 && size <= smallFullScanBytes {
		st.byModel = map[string]Totals{}
		st.turnCount = 0
		st.requests = nil
		data, err := os.ReadFile(path)
		if err != nil {
			return Item{}, false, err
		}
		for _, line := range splitLines(data) {
			if err := st.apply(line, true); err != nil {
				return Item{}, false, err
			}
		}
	}
	if st.id == "" || !st.hasUpdatedAt {
		return Item{}, false, nil
	}
	var totals Totals
	for _, t := range st.byModel {
		if totals, err = totals.Add(t); err != nil {
			return Item{}, false, err
		}
	}
	totals.Turns = st.turnCount
	title := st.title
	if title == "" {
		title = untitled
	}
	cost, unpriced, err := s.estimatedCost(st.requests, totals)
	if err != nil {
		return Item{}, false, err
	}
	return Item{
		SourcePath:     path,
		UnpricedTokens: unpriced,
		ID:             st.id,
		Title:          title,
		ProjectName:    ProjectName(st.cwd),
		Cwd:            st.cwd,
		UpdatedAt:      st.updatedAt,
		State:          st.state,
		Totals:         totals,
		EstimatedUSD:   cost,
	}, true, nil
}

// probeLines returns the lines worth reading: all of a file up to 576 KiB,
// otherwise its first 512 KiB and its last 64 KiB. The first line of the tail
// is dropped (it is cut in the middle); the last line of the head may be cut,
// and then simply fails to parse.
func probeLines(path string, size int64) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if size <= headProbeBytes+tailProbeBytes {
		data, err := readAt(f, 0, end)
		return splitLines(data), err
	}
	head, err := readAt(f, 0, min(end, headProbeBytes))
	if err != nil {
		return nil, err
	}
	tailStart := max(end-tailProbeBytes, 0)
	tail, err := readAt(f, tailStart, end-tailStart)
	if err != nil {
		return nil, err
	}
	tailLines := splitLines(tail)
	if tailStart > 0 && len(tailLines) > 0 {
		tailLines = tailLines[1:]
	}
	return append(splitLines(head), tailLines...), nil
}

func readAt(f *os.File, offset, length int64) ([]byte, error) {
	buf := make([]byte, length)
	n, err := f.ReadAt(buf, offset)
	if err == io.EOF {
		err = nil
	}
	return buf[:n], err
}

// apply folds one JSONL line into the state. includeLast says whether
// last_token_usage events (per turn) are added up; cumulative
// total_token_usage always replaces the model's total.
func (st *parseState) apply(line string, includeLast bool) error {
	obj, ok := decodeLine(line)
	if !ok {
		return nil
	}
	payload := objectOf(obj["payload"])
	objType, _ := stringOf(obj, "type")
	payloadType, _ := stringOf(payload, "type")
	if stamp, ok := stringOf(obj, "timestamp"); ok {
		if date, ok := parseTimestamp(stamp); ok {
			if !st.hasUpdatedAt || date.After(st.updatedAt) {
				st.updatedAt = date
			}
			st.hasUpdatedAt = true
		}
	}
	if objType == "session_meta" {
		if st.id == "" {
			if id, ok := stringOf(payload, "id"); ok {
				st.id = id
			} else if id, ok := stringOf(payload, "session_id"); ok {
				st.id = id
			}
		}
		if st.cwd == nil {
			if cwd, ok := stringOf(payload, "cwd"); ok {
				st.cwd = &cwd
			}
		}
	}
	if st.title == "" {
		st.title = userTitle(payload)
	}
	st.state = stronger(st.state, detectedState(objType, payloadType, payload))
	if objType == "turn_context" {
		if model, ok := stringOf(payload, "model"); ok {
			st.currentModel = model
		}
	}
	model := st.currentModel
	if m, ok := stringOf(objectOf(obj["turn_context"]), "model"); ok {
		model = m
	} else if m, ok := stringOf(payload, "model"); ok {
		model = m
	}
	if payloadType == "task_complete" {
		st.turnCount++
	}
	info := objectOf(payload["info"])
	if last, ok := info["last_token_usage"].(map[string]any); ok {
		t, err := totalsFromUsage(last)
		if err != nil {
			return err
		}
		st.requests = append(st.requests, sessionRequest{model: model, totals: t})
	}

	if total, ok := info["total_token_usage"].(map[string]any); ok {
		t, err := totalsFromUsage(total)
		if err != nil {
			return err
		}
		st.byModel[model] = t
	} else if last, ok := info["last_token_usage"].(map[string]any); includeLast && ok {
		t, err := totalsFromUsage(last)
		if err != nil {
			return err
		}
		sum, err := st.byModel[model].Add(t)
		if err != nil {
			return err
		}
		st.byModel[model] = sum
	}
	return nil
}

// totalsFromUsage reads the four token counters of a usage object. A missing
// counter is 0; one that is not an integer is an error, not a silent zero.
func totalsFromUsage(usage map[string]any) (Totals, error) {
	var n [4]int
	for i, key := range []string{"input_tokens", "cached_input_tokens", "output_tokens", "reasoning_output_tokens"} {
		v, err := tokenInteger(usage, key)
		if err != nil {
			return Totals{}, err
		}
		n[i] = v
	}
	if n[0] < 0 || n[1] < 0 || n[2] < 0 || n[3] < 0 || n[1] > n[0] {
		return Totals{}, &ArithmeticError{Field: "session token usage"}
	}
	return NewTotals(n[0], n[1], n[2], 0, 0)
}

func tokenInteger(usage map[string]any, key string) (int, error) {
	raw, ok := usage[key]
	if !ok {
		return 0, nil
	}
	if num, ok := raw.(json.Number); ok {
		if v, err := strconv.ParseInt(num.String(), 10, 64); err == nil {
			return int(v), nil
		}
		// 5.0 or 5e2 count as the integers 5 and 500, as in Swift.
		if f, err := strconv.ParseFloat(num.String(), 64); err == nil && f == math.Trunc(f) && math.Abs(f) < 1<<63 {
			return int(f), nil
		}
	}
	return 0, &ArithmeticError{Field: "session token usage"}
}

func detectedState(objType, payloadType string, payload map[string]any) State {
	status, _ := stringOf(payload, "status")
	var parts []string
	for _, p := range []string{objType, payloadType, status} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	text := strings.ToLower(strings.Join(parts, " "))
	if strings.Contains(text, "error") || strings.Contains(text, "failed") {
		return StateFailed
	}
	for _, w := range []string{"approval", "permission", "waiting"} {
		if strings.Contains(text, w) {
			return StateNeedsAttention
		}
	}
	return StateRecent
}

func stronger(a, b State) State {
	if a == StateFailed || b == StateFailed {
		return StateFailed
	}
	if a == StateNeedsAttention || b == StateNeedsAttention {
		return StateNeedsAttention
	}
	return StateRecent
}

type sessionRequest struct {
	model  string
	totals Totals
}

// estimatedCost never prices a cumulative total as if it were one prompt
// The bounded recent scan can miss requests in the middle of a large file;
// any remaining tokens are unpriced until the event index supplies their cost
func (s Scanner) estimatedCost(requests []sessionRequest, totals Totals) (*swiftjson.Decimal, int64, error) {
	var sum swiftjson.Decimal
	var observed Totals
	var unpriced int64
	priced := false
	for _, request := range requests {
		t := request.totals
		var err error
		if observed, err = observed.Add(t); err != nil {
			return nil, 0, err
		}
		if t.TotalTokens == 0 {
			continue
		}
		if s.Pricer != nil {
			if usd, ok := s.Pricer(request.model, t); ok {
				sum = sum.Add(usd)
				priced = true
				continue
			}
		}
		unpriced += int64(t.TotalTokens)
	}
	// A request sample may duplicate cumulative notifications or exceed the
	// trusted total; report the total as unpriced rather than double-count it
	if observed.TotalTokens > totals.TotalTokens {
		return nil, int64(totals.TotalTokens), nil
	}
	unpriced += int64(totals.TotalTokens - observed.TotalTokens)
	if !priced {
		return nil, unpriced, nil
	}
	return &sum, unpriced, nil
}
