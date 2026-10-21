package usage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"time"
)

const (
	defaultChunkSize    = 256 * 1024
	defaultMaxLineBytes = 8 * 1024 * 1024
)

// fingerprintSeed starts the rolling content fingerprint. The fingerprint is
// SHA256(previous || line) over every LF-terminated candidate line, so it
// identifies a file prefix without keeping the bytes.
var fingerprintSeed = func() []byte {
	s := sha256.Sum256([]byte("usage-cost-content-fingerprint-v3"))
	return s[:]
}()

func extendFingerprint(prev, line []byte) []byte {
	h := sha256.New()
	h.Write(prev)
	h.Write(line)
	return h.Sum(nil)
}

// parsedLine is one successfully decoded candidate line.
type parsedLine struct {
	Offset     uint64
	LFComplete bool
	Record     logRecord
}

// streamResult summarizes one pass over a file.
type streamResult struct {
	Meta                fileMeta
	BytesRead           int
	CandidateLines      int
	DecodedLines        int
	MalformedLines      int
	IncompleteMalformed int
	OversizedLines      int
	IncompleteOversized int
	// IgnoredOversized counts the complete oversized lines that
	// readRequest.IgnoreOversized accepted; they are also in OversizedLines.
	IgnoredOversized   int
	MaxBufferedBytes   int
	LastCompleteOffset uint64
	TrailingLineStart  uint64
	HasTrailingLine    bool
	ContentFingerprint []byte
}

// readRequest describes one streaming pass.
type readRequest struct {
	Path          string
	FromOffset    uint64
	InitialFP     []byte // nil starts from the seed
	Expected      *sourceFile
	RequireStable bool
	// IgnoreOversized, when set, tells from the first LineHeadBytes of a
	// complete oversized line that dropping it loses nothing the caller reads.
	IgnoreOversized func(head []byte) bool
}

// logStream reads session logs line by line without holding a whole file in
// memory.
type logStream struct {
	chunkSize    int
	maxLineBytes int
	parser       *recordParser
}

func newLogStream(chunkSize, maxLineBytes int, loc *time.Location) *logStream {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if maxLineBytes <= 0 {
		maxLineBytes = defaultMaxLineBytes
	}
	return &logStream{chunkSize: chunkSize, maxLineBytes: maxLineBytes, parser: newRecordParser(loc)}
}

// read streams the candidate lines of a file from req.FromOffset up to the
// size observed when the file was opened. onRecord receives every decoded
// line; lines that fail to decode are counted.
func (s *logStream) read(ctx context.Context, req readRequest, onRecord func(parsedLine) error) (streamResult, error) {
	fp := req.InitialFP
	if fp == nil {
		fp = fingerprintSeed
	}
	var res streamResult
	acc := newLineAccumulator(req.FromOffset, s.maxLineBytes)
	acc.ignore = req.IgnoreOversized
	onCandidate := func(c candidateLine) error {
		if c.LFComplete {
			fp = extendFingerprint(fp, c.Data)
		}
		rec, err := s.parser.parse(c.Data)
		if err != nil {
			if errors.Is(err, errInvalidCacheCounters) && (c.LFComplete || syntacticallyCompleteJSON(c.Data)) {
				return err
			}
			res.MalformedLines++
			if !c.LFComplete {
				res.IncompleteMalformed++
			}
			return nil
		}
		res.DecodedLines++
		return onRecord(parsedLine{Offset: c.Offset, LFComplete: c.LFComplete, Record: rec})
	}

	f, err := os.Open(req.Path)
	if err != nil {
		return res, err
	}
	defer f.Close()
	meta, err := fstatMeta(f)
	if err != nil {
		return res, err
	}
	if !meta.Regular {
		return res, ErrInvalidSourceFile
	}
	if req.Expected != nil {
		if !meta.sameIdentity(req.Expected.meta) {
			return res, ErrSourceIdentityChanged
		}
		if req.RequireStable && !meta.sameSnapshot(req.Expected.meta) {
			return res, ErrSourceMetadataChanged
		}
	}
	size := meta.Size
	if req.FromOffset > size {
		return res, ErrOffsetBeyondSnapshot
	}
	if _, err := f.Seek(int64(req.FromOffset), io.SeekStart); err != nil {
		return res, err
	}

	buf := make([]byte, s.chunkSize)
	pos := req.FromOffset
	maxBuffered := 0
	for pos < size {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		want := size - pos
		if want > uint64(len(buf)) {
			want = uint64(len(buf))
		}
		n, rerr := f.Read(buf[:want])
		if n == 0 {
			if rerr == nil || errors.Is(rerr, io.EOF) {
				return res, ErrUnexpectedEOF
			}
			return res, rerr
		}
		chunk := buf[:n]
		if n > maxBuffered {
			maxBuffered = n
		}
		if err := acc.consume(chunk, pos, onCandidate); err != nil {
			return res, err
		}
		if acc.maxBuffered > maxBuffered {
			maxBuffered = acc.maxBuffered
		}
		pos += uint64(n)
		res.BytesRead += n
	}
	if err := acc.finish(onCandidate); err != nil {
		return res, err
	}
	if req.RequireStable {
		after, err := fstatMeta(f)
		if err != nil {
			return res, err
		}
		if !after.sameSnapshot(meta) {
			return res, ErrSourceMetadataChanged
		}
	}
	if acc.maxBuffered > maxBuffered {
		maxBuffered = acc.maxBuffered
	}
	res.Meta = meta
	res.CandidateLines = acc.candidateLines
	res.OversizedLines = acc.oversizedLines
	res.IgnoredOversized = acc.ignoredOversized
	if acc.discarding {
		res.IncompleteOversized = 1
	}
	res.MaxBufferedBytes = maxBuffered
	res.LastCompleteOffset = acc.lastComplete
	if acc.lastComplete < size {
		res.TrailingLineStart = acc.lastComplete
		res.HasTrailingLine = true
	}
	res.ContentFingerprint = fp
	return res, nil
}

func syntacticallyCompleteJSON(line []byte) bool {
	d := &jsonDec{b: line}
	if bytes.HasPrefix(d.b, []byte{0xEF, 0xBB, 0xBF}) {
		d.i = 3
	}
	if err := d.skip(); err != nil {
		return false
	}
	d.skipSpace()
	return d.i == len(d.b)
}

// candidateLine is a line that passed the prefilter.
type candidateLine struct {
	Offset     uint64
	LFComplete bool
	Data       []byte // valid only during the callback
}

// lineAccumulator splits chunks into lines, drops lines over the size limit and
// forwards only lines that mention a marker we care about.
type lineAccumulator struct {
	lineStart      uint64
	maxLineBytes   int
	buffer         []byte
	discarding     bool
	candidateLines int
	oversizedLines int
	maxBuffered    int
	lastComplete   uint64
	// ignore classifies an oversized line by head, its start, once the line
	// is complete.
	ignore           func([]byte) bool
	head             []byte
	ignoredOversized int
}

func newLineAccumulator(start uint64, maxLineBytes int) *lineAccumulator {
	return &lineAccumulator{lineStart: start, lastComplete: start, maxLineBytes: maxLineBytes}
}

func (a *lineAccumulator) consume(chunk []byte, chunkOffset uint64, on func(candidateLine) error) error {
	start := 0
	for {
		rel := bytes.IndexByte(chunk[start:], '\n')
		if rel < 0 {
			break
		}
		nl := start + rel
		if err := a.segment(chunk[start:nl], true, on); err != nil {
			return err
		}
		a.lineStart = chunkOffset + uint64(nl+1)
		a.lastComplete = a.lineStart
		start = nl + 1
	}
	if start < len(chunk) {
		return a.segment(chunk[start:], false, on)
	}
	return nil
}

func (a *lineAccumulator) finish(on func(candidateLine) error) error {
	if a.discarding || len(a.buffer) == 0 {
		return nil
	}
	err := a.emit(a.buffer, false, on)
	a.buffer = a.buffer[:0]
	return err
}

func (a *lineAccumulator) segment(seg []byte, lf bool, on func(candidateLine) error) error {
	if a.discarding {
		if lf {
			a.discarding = false
			a.oversizedEnd()
		}
		return nil
	}
	if len(a.buffer)+len(seg) > a.maxLineBytes {
		a.oversizedLines++
		if a.ignore != nil {
			a.head = LineHead(a.buffer, seg)
		}
		a.buffer = nil
		a.discarding = !lf
		if lf {
			a.oversizedEnd()
		}
		return nil
	}
	if len(a.buffer) == 0 && lf {
		return a.emit(seg, true, on)
	}
	a.buffer = append(a.buffer, seg...)
	if len(a.buffer) > a.maxBuffered {
		a.maxBuffered = len(a.buffer)
	}
	if !lf {
		return nil
	}
	err := a.emit(a.buffer, true, on)
	a.buffer = a.buffer[:0]
	return err
}

// oversizedEnd classifies a complete oversized line by its start.
func (a *lineAccumulator) oversizedEnd() {
	if a.head != nil && a.ignore(a.head) {
		a.ignoredOversized++
	}
	a.head = nil
}

var linePrefilterMarkers = [][]byte{
	[]byte("token_count"),
	[]byte("task_complete"),
	[]byte("turn_context"),
	[]byte("session_meta"),
	[]byte(`"cwd"`),
}

func isCandidateLine(b []byte) bool {
	for _, m := range linePrefilterMarkers {
		if bytes.Contains(b, m) {
			return true
		}
	}
	return false
}

func (a *lineAccumulator) emit(b []byte, lf bool, on func(candidateLine) error) error {
	if !isCandidateLine(b) {
		return nil
	}
	a.candidateLines++
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return on(candidateLine{Offset: a.lineStart, LFComplete: lf, Data: b})
}
