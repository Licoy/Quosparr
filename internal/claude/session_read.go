package claude

import (
	"bufio"
	"bytes"
	"context"
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

	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/usage"
)

const maxJSONLLine = 8 << 20

type usageRecord struct {
	sessionID    string
	at           time.Time
	hasTime      bool
	firstAt      time.Time
	hasFirst     bool
	parent       time.Time
	hasParent    bool
	model        string
	pricingModel string
	effort       string
	parts        TokenParts
	cacheInput   int64
	cacheRead    int64
	cacheKnown   bool
	cacheInvalid bool
	// tier is the usage speed field: fast, standard, or unknown when absent.
	tier speed.Tier
	geo  string
	// toolUse is set when any line of the message asked for a tool. A
	// message without one is the turn's final answer.
	toolUse bool
}

// Title sources, weakest first. A /rename title beats the generated one.
const (
	titleNone = iota
	titleAI
	titleCustom
)

type sessionMeta struct {
	title     string
	titleRank int
	cwd       string
	cwdAt     time.Time
	hasCwd    bool
	updated   time.Time
}

type scanSink struct {
	kept  map[string]usageRecord
	order []string
	metas map[string]*sessionMeta
	bad   int
	// ignored counts the bad lines that are oversized records other than an
	// assistant message, which carry no usage.
	ignored int
	// unread is, per merged file, the bad lines that may carry usage.
	unread []unreadLines
	anon   int
	// prefix keeps anonymous keys of different files apart.
	prefix string
}

type unreadLines struct {
	mtime time.Time
	lines int
}

func newScanSink(prefix string) *scanSink {
	return &scanSink{kept: map[string]usageRecord{}, metas: map[string]*sessionMeta{}, prefix: prefix}
}

// collect reads the newest limit files plus every file changed since since.
// Each file is parsed on its own, through the cache when there is one, and
// the parts are merged so a message split over files still counts once.
func (s SessionScanner) collect(ctx context.Context, limit int, since time.Time) (*scanSink, error) {
	files, err := s.listFiles(ctx, limit, since)
	if err != nil {
		return nil, err
	}
	sink := newScanSink("")
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		part, ok := s.Cache.get(file)
		if !ok {
			part = newScanSink(file.path)
			if err := part.readFile(ctx, file.path, file.mtime); err != nil {
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			s.Cache.put(file, part)
		}
		sink.merge(part)
		if n := part.bad - part.ignored; n > 0 {
			sink.unread = append(sink.unread, unreadLines{mtime: file.mtime, lines: n})
		}
	}
	return sink, nil
}

type sessionFile struct {
	path  string
	size  int64
	mtime time.Time
}

func (s SessionScanner) listFiles(ctx context.Context, limit int, since time.Time) ([]sessionFile, error) {
	var files []sessionFile
	for _, root := range ProjectsDirs(s.ConfigDir, s.Home) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		found, err := walkProjects(ctx, root)
		if err != nil {
			return nil, err
		}
		files = append(files, found...)
	}
	sort.Slice(files, func(i, j int) bool {
		if !files[i].mtime.Equal(files[j].mtime) {
			return files[i].mtime.After(files[j].mtime)
		}
		return files[i].path < files[j].path
	})
	listed := make(map[string]bool, len(files))
	for _, f := range files {
		listed[f.path] = true
	}
	s.Cache.retain(listed)
	out := files[:0]
	for i, f := range files {
		if (limit > 0 && i < limit) || (!since.IsZero() && !f.mtime.Before(since)) {
			out = append(out, f)
		}
	}
	return out, nil
}

func walkProjects(ctx context.Context, root string) ([]sessionFile, error) {
	st, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, nil
	}
	var files []sessionFile
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && excludedSessionDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".jsonl") || strings.EqualFold(d.Name(), "history.jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, sessionFile{path: path, size: info.Size(), mtime: info.ModTime()})
		return nil
	})
	return files, err
}

func excludedSessionDir(name string) bool {
	switch name {
	case "file-history", "paste-cache", "shell-snapshots", "session-env", "sessions":
		return true
	default:
		return false
	}
}

// readFile parses one log. The context is checked every few thousand lines
// so a cancelled refresh does not finish a large file. A last line without a
// newline may still be being written; it is not counted as malformed.
func (sink *scanSink) readFile(ctx context.Context, path string, mtime time.Time) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	var parent time.Time
	var hasParent bool
	for n := 1; ; n++ {
		if n%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		line, tooLong, err := readLogicalLine(br, maxJSONLLine)
		if err != nil && err != io.EOF {
			return err
		}
		if tooLong {
			sink.bad++
			if claudeIgnorableHead(line) {
				sink.ignored++
			}
		} else if len(bytes.TrimSpace(line)) > 0 {
			sink.consume(line, mtime, &parent, &hasParent, err == io.EOF)
		}
		if err != nil {
			return nil
		}
	}
}

// readLogicalLine reads one line. A line longer than max is skipped; it
// returns its first usage.LineHeadBytes with tooLong set.
func readLogicalLine(br *bufio.Reader, max int) ([]byte, bool, error) {
	var out []byte
	tooLong := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !tooLong {
			if len(out)+len(chunk) > max {
				tooLong = true
				out = usage.LineHead(out, chunk)
			} else {
				out = append(out, chunk...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if tooLong {
			return out, true, err
		}
		return trimNL(out), false, err
	}
}

// claudeIgnorableHead tells from its start that an oversized line is a record
// other than an assistant message, so it has no usage.
func claudeIgnorableHead(head []byte) bool {
	typ, ok := usage.HeadString(head, "type")
	return ok && typ != "assistant"
}

func trimNL(line []byte) []byte {
	return bytes.TrimRight(line, "\r\n")
}

type jsonlLine struct {
	Type         string `json:"type"`
	Timestamp    string `json:"timestamp"`
	SessionID    string `json:"sessionId"`
	Cwd          string `json:"cwd"`
	AITitle      string `json:"aiTitle"`
	CustomTitle  string `json:"customTitle"`
	Effort       string `json:"effort"`
	RequestID    string `json:"requestId"`
	InferenceGeo string `json:"inference_geo"`
	Message      *struct {
		ID         string          `json:"id"`
		Model      string          `json:"model"`
		StopReason string          `json:"stop_reason"`
		RequestID  string          `json:"requestId"`
		Usage      *rawUsage       `json:"usage"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
}

type rawUsage struct {
	Input         *float64 `json:"input_tokens"`
	CacheCreate   *float64 `json:"cache_creation_input_tokens"`
	CacheRead     *float64 `json:"cache_read_input_tokens"`
	Output        *float64 `json:"output_tokens"`
	CacheCreation *struct {
		Five *float64 `json:"ephemeral_5m_input_tokens"`
		Hour *float64 `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	Speed        string `json:"speed"`
	InferenceGeo string `json:"inference_geo"`
}

func (sink *scanSink) consume(line []byte, mtime time.Time, parent *time.Time, hasParent *bool, unterminated bool) {
	var row jsonlLine
	if err := json.Unmarshal(line, &row); err != nil {
		if !unterminated {
			sink.bad++
		}
		return
	}
	at, hasTime := parseTimestamp(row.Timestamp)
	if !hasTime {
		at = mtime
	}
	sessionID := strings.TrimSpace(row.SessionID)
	if sessionID == "" {
		sessionID = "unknown"
	}
	meta := sink.meta(sessionID)
	if hasTime && (meta.updated.IsZero() || at.After(meta.updated)) {
		meta.updated = at
	}
	if meta.updated.IsZero() {
		meta.updated = mtime
	}
	sink.noteContext(meta, row, at, hasTime)
	if row.Type == "user" || row.Type == "tool_result" {
		if hasTime {
			*parent, *hasParent = at, true
		}
		return
	}
	if row.Type != "assistant" || row.Message == nil || row.Message.Usage == nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(row.Message.Model), "<synthetic>") {
		return
	}
	parts, ok := partsFrom(row.Message.Usage)
	if !ok {
		sink.bad++
		return
	}
	cacheInput, cacheRead, cacheKnown, cacheInvalid := cacheSample(row.Message.Usage, parts)
	rec := usageRecord{
		sessionID: sessionID, at: at, hasTime: hasTime, firstAt: at, hasFirst: hasTime,
		parent: *parent, hasParent: *hasParent,
		model: NormalizeModel(row.Message.Model), pricingModel: row.Message.Model, effort: strings.TrimSpace(row.Effort),
		parts: parts, cacheInput: cacheInput, cacheRead: cacheRead, cacheKnown: cacheKnown, cacheInvalid: cacheInvalid,
		tier: speedTier(row.Message.Usage.Speed), geo: geoOf(row),
		toolUse: strings.TrimSpace(row.Message.StopReason) == "tool_use" || bytes.Contains(row.Message.Content, []byte(`"type":"tool_use"`)),
	}
	sink.keep(rec, requestOf(row))
}

func cacheSample(u *rawUsage, parts TokenParts) (int64, int64, bool, bool) {
	if u.Input == nil || u.CacheRead == nil {
		return 0, 0, false, false
	}
	writeKnown := u.CacheCreate != nil
	if u.CacheCreation != nil && (u.CacheCreation.Five != nil || u.CacheCreation.Hour != nil) {
		writeKnown = u.CacheCreation.Five != nil && u.CacheCreation.Hour != nil
	}
	if !writeKnown {
		return 0, 0, false, false
	}
	if parts.Input > math.MaxInt64-parts.CacheWrite5m || parts.Input+parts.CacheWrite5m > math.MaxInt64-parts.CacheWrite1h ||
		parts.Input+parts.CacheWrite5m+parts.CacheWrite1h > math.MaxInt64-parts.CacheRead {
		return 0, 0, false, true
	}
	return parts.Input + parts.CacheWrite5m + parts.CacheWrite1h + parts.CacheRead, parts.CacheRead, true, false
}

// noteContext records the title and working directory. Title lines carry no
// timestamp, so the last one in the file wins within a source, and a
// /rename title wins over a generated one.
func (sink *scanSink) noteContext(meta *sessionMeta, row jsonlLine, at time.Time, hasTime bool) {
	switch row.Type {
	case "ai-title":
		meta.setTitle(row.AITitle, titleAI)
	case "custom-title":
		meta.setTitle(row.CustomTitle, titleCustom)
	}
	cwd := strings.TrimSpace(row.Cwd)
	if cwd == "" {
		return
	}
	if !meta.hasCwd || (hasTime && at.Before(meta.cwdAt)) {
		meta.cwd, meta.cwdAt, meta.hasCwd = cwd, at, true
	}
}

func (m *sessionMeta) setTitle(title string, rank int) {
	title = strings.TrimSpace(title)
	if title != "" && rank >= m.titleRank {
		m.title, m.titleRank = title, rank
	}
}

// merge adds a parsed file. part is cached and is not changed.
func (sink *scanSink) merge(part *scanSink) {
	sink.bad += part.bad
	for _, key := range part.order {
		sink.keepKey(key, part.kept[key])
	}
	for id, src := range part.metas {
		dst := sink.meta(id)
		if src.updated.After(dst.updated) {
			dst.updated = src.updated
		}
		if src.titleRank > dst.titleRank || (src.titleRank == dst.titleRank && dst.title == "") {
			dst.title, dst.titleRank = src.title, src.titleRank
		}
		if src.hasCwd && (!dst.hasCwd || src.cwdAt.Before(dst.cwdAt)) {
			dst.cwd, dst.cwdAt, dst.hasCwd = src.cwd, src.cwdAt, true
		}
	}
}

func (sink *scanSink) meta(id string) *sessionMeta {
	if sink.metas[id] == nil {
		sink.metas[id] = &sessionMeta{}
	}
	return sink.metas[id]
}

func requestOf(row jsonlLine) string {
	if id := strings.TrimSpace(row.RequestID); id != "" {
		return row.Message.ID + "\x00" + id
	}
	if row.Message != nil && strings.TrimSpace(row.Message.RequestID) != "" {
		return row.Message.ID + "\x00" + strings.TrimSpace(row.Message.RequestID)
	}
	if row.Message != nil && strings.TrimSpace(row.Message.ID) != "" {
		return row.Message.ID
	}
	return ""
}

func preferRecord(next, prev usageRecord) bool {
	if next.parts.Output != prev.parts.Output {
		return next.parts.Output > prev.parts.Output
	}
	if next.hasTime && prev.hasTime && !next.at.Equal(prev.at) {
		return next.at.After(prev.at)
	}
	return true
}

func recordInWindow(rec usageRecord, window usage.Window) bool {
	if window.Start.IsZero() && window.End.IsZero() {
		return true
	}
	if !rec.hasTime {
		return false
	}
	if !window.Start.IsZero() && rec.at.Before(window.Start) {
		return false
	}
	if !window.End.IsZero() && !rec.at.Before(window.End) {
		return false
	}
	return true
}

func partsFrom(u *rawUsage) (TokenParts, bool) {
	in, ok1 := exactInt(u.Input)
	create, ok2 := exactInt(u.CacheCreate)
	read, ok3 := exactInt(u.CacheRead)
	out, ok4 := exactInt(u.Output)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return TokenParts{}, false
	}
	parts := TokenParts{Input: in, CacheRead: read, Output: out, Fast: strings.EqualFold(strings.TrimSpace(u.Speed), "fast")}
	if u.CacheCreation != nil && (u.CacheCreation.Five != nil || u.CacheCreation.Hour != nil) {
		five, ok5 := exactInt(u.CacheCreation.Five)
		hour, ok6 := exactInt(u.CacheCreation.Hour)
		if !ok5 || !ok6 {
			return TokenParts{}, false
		}
		parts.CacheWrite5m, parts.CacheWrite1h = five, hour
		return parts, true
	}
	parts.CacheWrite5m = create
	return parts, true
}

func exactInt(f *float64) (int64, bool) {
	if f == nil {
		return 0, true
	}
	if math.IsNaN(*f) || math.IsInf(*f, 0) || *f < 0 || *f != math.Trunc(*f) || *f > float64(math.MaxInt64) {
		return 0, false
	}
	return int64(*f), true
}

func geoOf(row jsonlLine) string {
	if row.Message != nil && row.Message.Usage != nil && strings.TrimSpace(row.Message.Usage.InferenceGeo) != "" {
		return row.Message.Usage.InferenceGeo
	}
	return row.InferenceGeo
}

func (sink *scanSink) keep(rec usageRecord, requestID string) {
	key := requestID
	if key == "" {
		sink.anon++
		key = "anon-" + sink.prefix + "-" + strconv.Itoa(sink.anon)
	}
	sink.keepKey(key, rec)
}

// keepKey stores one record per message. The fullest line wins, the earliest
// assistant line supplies request timing, and tool use is merged across lines.
func (sink *scanSink) keepKey(key string, rec usageRecord) {
	prev, ok := sink.kept[key]
	if !ok {
		sink.order = append(sink.order, key)
		sink.kept[key] = rec
		return
	}
	toolUse := prev.toolUse || rec.toolUse
	firstAt, hasFirst, parent, hasParent := earliestRequest(prev, rec)
	if preferRecord(rec, prev) {
		prev = rec
	}
	prev.toolUse = toolUse
	prev.firstAt, prev.hasFirst = firstAt, hasFirst
	prev.parent, prev.hasParent = parent, hasParent
	sink.kept[key] = prev
}

func earliestRequest(a, b usageRecord) (time.Time, bool, time.Time, bool) {
	switch {
	case a.hasFirst && b.hasFirst && a.firstAt.Before(b.firstAt):
		return a.firstAt, true, a.parent, a.hasParent
	case a.hasFirst && b.hasFirst && b.firstAt.Before(a.firstAt):
		return b.firstAt, true, b.parent, b.hasParent
	case a.hasFirst && b.hasFirst:
		parent, ok := earliestKnownTime(a.parent, a.hasParent, b.parent, b.hasParent)
		return a.firstAt, true, parent, ok
	case a.hasFirst:
		return a.firstAt, true, a.parent, a.hasParent
	case b.hasFirst:
		return b.firstAt, true, b.parent, b.hasParent
	default:
		return time.Time{}, false, time.Time{}, false
	}
}

func earliestKnownTime(a time.Time, hasA bool, b time.Time, hasB bool) (time.Time, bool) {
	if !hasA {
		return b, hasB
	}
	if !hasB || a.Before(b) {
		return a, true
	}
	return b, true
}
