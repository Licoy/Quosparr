package usage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/licoy/quosparr/internal/sqliteruntime"
)

// Index schema and parser versions this package reads and writes. A database
// with other values is reported as incompatible and rebuilt.
const (
	indexSchemaVersion = 3
	// CurrentParserVersion is the log parser version stored with each indexed
	// source. Bump it whenever parsing changes what is extracted from a line.
	CurrentParserVersion = 4

	hashWindowBytes = 16 * 1024
	unknownModel    = "unknown-model"
)

// Reasons an index database has to be rebuilt.
const (
	ReasonSchemaVersion   = "schema-version"
	ReasonParserVersion   = "parser-version"
	ReasonTimeZone        = "time-zone"
	ReasonDatabaseCorrupt = "database-corrupt"
)

// indexMismatch reports that an existing database cannot be used as is.
type indexMismatch struct {
	Reason string
	Detail string
}

func (e *indexMismatch) Error() string {
	return fmt.Sprintf("usage: index incompatible (%s): %s", e.Reason, e.Detail)
}

func corruptRow(field string) error {
	return &indexMismatch{Reason: ReasonDatabaseCorrupt, Detail: "corrupt row: " + field}
}

var emptyHash = func() []byte { h := sha256.Sum256(nil); return h[:] }()

// indexedSource mirrors a row of source_files.
type indexedSource struct {
	ID                   int64 // 0 when not stored yet
	Basename             string
	Root                 string
	Path                 string
	Device               int64
	Inode                int64
	BirthNS              int64
	MtimeNS              int64
	CtimeNS              int64
	Size                 uint64
	CompleteOffset       uint64
	CurrentModel         string
	CurrentProject       string
	FirstHash            []byte
	FirstHashLength      int64
	CheckpointHash       []byte
	CheckpointHashLength int64
	ParserVersion        int64
	MalformedLines       int64
	OversizedLines       int64
	ContentFingerprint   []byte // nil is stored as NULL
}

func (s indexedSource) meta() fileMeta {
	return fileMeta{Device: s.Device, Inode: s.Inode, BirthNS: s.BirthNS, MtimeNS: s.MtimeNS, CtimeNS: s.CtimeNS, Size: s.Size, Regular: true}
}

// applyMetadata points the row at the file as it is now.
func (s *indexedSource) applyMetadata(f sourceFile) {
	s.Root = string(f.Root)
	s.Path = f.Path
	s.Device = f.meta.Device
	s.Inode = f.meta.Inode
	s.BirthNS = f.meta.BirthNS
	s.MtimeNS = f.meta.MtimeNS
	s.CtimeNS = f.meta.CtimeNS
	s.Size = f.meta.Size
}

// indexedEvent is one usage_events row, or after aggregation a group of them.
type indexedEvent struct {
	ByteOffset uint64
	Timestamp  float64
	DayKey     string
	Model      string
	Project    string
	Uncached   int64
	Cached     int64
	Output     int64
	Turns      int64
}

// sourceIdentity identifies a file independent of its path.
type sourceIdentity struct {
	Device  int64
	Inode   int64
	BirthNS int64
}

func identityOf(f sourceFile) sourceIdentity {
	return sourceIdentity{f.meta.Device, f.meta.Inode, f.meta.BirthNS}
}

// cachedFullHash is a row of source_hash_cache.
type cachedFullHash struct {
	Identity sourceIdentity
	Size     uint64
	MtimeNS  int64
	CtimeNS  int64
	Digest   []byte
}

func cachedHashOf(f sourceFile, digest []byte) cachedFullHash {
	return cachedFullHash{identityOf(f), f.meta.Size, f.meta.MtimeNS, f.meta.CtimeNS, digest}
}

func (c cachedFullHash) matches(f sourceFile) bool {
	return c.Identity == identityOf(f) && c.Size == f.meta.Size && c.MtimeNS == f.meta.MtimeNS && c.CtimeNS == f.meta.CtimeNS
}

const sourceColumns = `basename, root, path, device, inode, birth_ns, mtime_ns, ctime_ns, size,
complete_offset, current_model, current_project, first_hash, first_hash_length,
checkpoint_hash, checkpoint_hash_length, parser_version, malformed_lines,
oversized_lines, full_hash`

const sourceAssignments = `root = excluded.root, path = excluded.path, device = excluded.device,
inode = excluded.inode, birth_ns = excluded.birth_ns, mtime_ns = excluded.mtime_ns,
ctime_ns = excluded.ctime_ns, size = excluded.size,
complete_offset = excluded.complete_offset, current_model = excluded.current_model,
current_project = excluded.current_project, first_hash = excluded.first_hash,
first_hash_length = excluded.first_hash_length,
checkpoint_hash = excluded.checkpoint_hash,
checkpoint_hash_length = excluded.checkpoint_hash_length,
parser_version = excluded.parser_version, malformed_lines = excluded.malformed_lines,
oversized_lines = excluded.oversized_lines, full_hash = excluded.full_hash`

var upsertSourceSQL = "INSERT INTO source_files(" + sourceColumns + ")\nVALUES (" +
	strings.TrimSuffix(strings.Repeat("?, ", 20), ", ") +
	")\nON CONFLICT(basename) DO UPDATE SET " + sourceAssignments

const eventConflictSQL = `
ON CONFLICT(file_id, byte_offset) DO UPDATE SET
    timestamp = excluded.timestamp, day_key = excluded.day_key,
    model = excluded.model, project = excluded.project,
    uncached_input_tokens = excluded.uncached_input_tokens,
    cached_input_tokens = excluded.cached_input_tokens,
    output_tokens = excluded.output_tokens,
    turns = excluded.turns`

const eventInsertHead = `INSERT INTO usage_events(
    file_id, byte_offset, timestamp, day_key, model, project,
    uncached_input_tokens, cached_input_tokens, output_tokens, turns)
VALUES `

const eventRowsPerStatement = 400

func sourceArgs(s indexedSource) ([]any, error) {
	if s.Size > 1<<63-1 || s.CompleteOffset > 1<<63-1 {
		return nil, fmt.Errorf("usage: integer overflow: source size")
	}
	var fp any
	if s.ContentFingerprint != nil {
		fp = s.ContentFingerprint
	}
	return []any{
		s.Basename, s.Root, s.Path, s.Device, s.Inode, s.BirthNS, s.MtimeNS, s.CtimeNS,
		int64(s.Size), int64(s.CompleteOffset), s.CurrentModel, s.CurrentProject,
		s.FirstHash, s.FirstHashLength, s.CheckpointHash, s.CheckpointHashLength,
		s.ParserVersion, s.MalformedLines, s.OversizedLines, fp,
	}, nil
}

// indexStore is the SQLite index of per-event usage.
type indexStore struct {
	db            *sqlite.DB
	path          string
	parserVersion int64
	timeZoneID    string
}

// sqliteCode returns the primary result code of err, or 0.
func sqliteCode(err error) int {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code & 0xFF
	}
	return 0
}

const (
	sqliteError   = 1
	sqliteCorrupt = 11
	sqliteNotADB  = 26
	sqliteSchema  = 17
)

func isSchemaError(err error) bool {
	c := sqliteCode(err)
	return c == sqliteError || c == sqliteSchema
}

func isUnambiguousCorruption(err error) bool {
	c := sqliteCode(err)
	return c == sqliteCorrupt || c == sqliteNotADB
}

// openIndexStore opens (creating when missing) the index at path. When the
// database exists but does not match the expected schema, parser version or
// time zone, it returns an *indexMismatch and leaves the file alone.
func openIndexStore(ctx context.Context, path string, parserVersion int64, tzID string) (*indexStore, error) {
	if err := sqliteruntime.Load(); err != nil {
		return nil, err
	}
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{BusyTimeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	s := &indexStore{db: db, path: path, parserVersion: parserVersion, timeZoneID: tzID}
	if err := s.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.initializeSchema(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *indexStore) Close() error { return s.db.Close() }

func (s *indexStore) configure(ctx context.Context) error {
	if _, err := s.db.Execute(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	if _, err := s.db.Query(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return err
	}
	_, err := s.db.Execute(ctx, "PRAGMA synchronous = NORMAL")
	return err
}

func (s *indexStore) initializeSchema(ctx context.Context) error {
	has, err := s.hasTable(ctx, "index_metadata")
	if err != nil {
		return err
	}
	if !has {
		user, err := s.hasUserTables(ctx)
		if err != nil {
			return err
		}
		if user {
			return &indexMismatch{Reason: ReasonSchemaVersion, Detail: "database has tables but no index metadata"}
		}
		return s.createSchema(ctx)
	}
	schema, parser, tz, err := s.storedMetadata(ctx)
	if err != nil {
		if isSchemaError(err) {
			return &indexMismatch{Reason: ReasonSchemaVersion, Detail: "index metadata unreadable"}
		}
		return err
	}
	if schema == nil || *schema != indexSchemaVersion {
		return &indexMismatch{Reason: ReasonSchemaVersion, Detail: fmt.Sprintf("expected schema %d", indexSchemaVersion)}
	}
	if parser == nil || *parser != s.parserVersion {
		return &indexMismatch{Reason: ReasonParserVersion, Detail: fmt.Sprintf("expected parser %d", s.parserVersion)}
	}
	if tz == nil || *tz != s.timeZoneID {
		return &indexMismatch{Reason: ReasonTimeZone, Detail: "aggregation time zone differs"}
	}
	if err := s.validateStorage(ctx); err != nil {
		if isSchemaError(err) {
			return &indexMismatch{Reason: ReasonSchemaVersion, Detail: "index tables do not match"}
		}
		return err
	}
	return nil
}

func (s *indexStore) createSchema(ctx context.Context) error {
	stmts := make([]sqlite.Statement, 0, len(indexSchemaStatements)+1)
	for _, q := range indexSchemaStatements {
		stmts = append(stmts, sqlite.Statement{SQL: q})
	}
	stmts = append(stmts, sqlite.Statement{
		SQL: `INSERT INTO index_metadata(
    singleton, schema_version, parser_version, aggregation_time_zone
) VALUES (1, ?, ?, ?)`,
		Args: []any{int64(indexSchemaVersion), s.parserVersion, s.timeZoneID},
	})
	_, err := s.db.Transaction(ctx, stmts)
	return err
}

func (s *indexStore) hasTable(ctx context.Context, name string) (bool, error) {
	rows, err := s.db.Query(ctx, "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1", name)
	return len(rows.Values) > 0, err
}

func (s *indexStore) hasUserTables(ctx context.Context) (bool, error) {
	rows, err := s.db.Query(ctx, "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' LIMIT 1")
	return len(rows.Values) > 0, err
}

func (s *indexStore) storedMetadata(ctx context.Context) (schema, parser *int64, tz *string, err error) {
	rows, err := s.db.Query(ctx, `SELECT schema_version, parser_version, aggregation_time_zone
FROM index_metadata WHERE singleton = 1`)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(rows.Values) == 0 {
		return nil, nil, nil, nil
	}
	r := rows.Values[0]
	sv, err := requiredInt64(r[0], "schema version")
	if err != nil {
		return nil, nil, nil, err
	}
	pv, err := requiredInt64(r[1], "parser version")
	if err != nil {
		return nil, nil, nil, err
	}
	z, err := requiredString(r[2], "aggregation time zone")
	if err != nil {
		return nil, nil, nil, err
	}
	return &sv, &pv, &z, nil
}

// storageIsValid reports whether every table decodes cleanly.
func (s *indexStore) storageIsValid(ctx context.Context) (bool, error) {
	err := s.validateStorage(ctx)
	if err == nil {
		return true, nil
	}
	var mm *indexMismatch
	if errors.As(err, &mm) || isSchemaError(err) {
		return false, nil
	}
	return false, err
}

func (s *indexStore) validateStorage(ctx context.Context) error {
	if _, err := s.sourceRows(ctx); err != nil {
		return err
	}
	if _, err := s.hashCacheRows(ctx); err != nil {
		return err
	}
	return s.validateEventStorage(ctx)
}

// ---- row decoding ----------------------------------------------------------

func requiredInt64(v any, field string) (int64, error) {
	n, ok := v.(int64)
	if !ok {
		return 0, corruptRow(field)
	}
	return n, nil
}

func requiredString(v any, field string) (string, error) {
	t, ok := v.(string)
	if !ok {
		return "", corruptRow(field)
	}
	return t, nil
}

func requiredHash(v any, field string) ([]byte, error) {
	b, ok := v.([]byte)
	if !ok || len(b) != sha256.Size {
		return nil, corruptRow(field)
	}
	return b, nil
}

func nonnegative(v any, field string) (int64, error) {
	n, err := requiredInt64(v, field)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, corruptRow(field)
	}
	return n, nil
}

func requiredDouble(v any, field string) (float64, error) {
	switch x := v.(type) {
	case float64:
		if x != x || x > 1.7976931348623157e308 || x < -1.7976931348623157e308 {
			return 0, corruptRow(field)
		}
		return x, nil
	case int64:
		return float64(x), nil
	}
	return 0, corruptRow(field)
}

func decodeSource(r []any, parserVersion int64) (indexedSource, error) {
	var s indexedSource
	var err error
	if s.ID, err = requiredInt64(r[0], "source id"); err != nil {
		return s, err
	}
	size, err := nonnegative(r[9], "source size")
	if err != nil {
		return s, err
	}
	complete, err := nonnegative(r[10], "complete offset")
	if err != nil {
		return s, err
	}
	if s.FirstHash, err = requiredHash(r[13], "first hash"); err != nil {
		return s, err
	}
	if s.FirstHashLength, err = nonnegative(r[14], "first hash length"); err != nil {
		return s, err
	}
	if s.CheckpointHash, err = requiredHash(r[15], "checkpoint hash"); err != nil {
		return s, err
	}
	if s.CheckpointHashLength, err = nonnegative(r[16], "checkpoint hash length"); err != nil {
		return s, err
	}
	if complete > size || s.FirstHashLength > size || s.CheckpointHashLength > complete {
		return s, corruptRow("source checkpoint")
	}
	if s.ContentFingerprint, err = requiredHash(r[20], "content fingerprint"); err != nil {
		return s, err
	}
	if s.MalformedLines, err = nonnegative(r[18], "malformed lines"); err != nil {
		return s, err
	}
	if s.OversizedLines, err = nonnegative(r[19], "oversized lines"); err != nil {
		return s, err
	}
	if s.MalformedLines > complete || s.OversizedLines > complete {
		return s, corruptRow("source anomaly counts")
	}
	s.Size, s.CompleteOffset = uint64(size), uint64(complete)
	if s.Basename, err = requiredString(r[1], "source basename"); err != nil {
		return s, err
	}
	if s.Root, err = requiredString(r[2], "source root"); err != nil {
		return s, err
	}
	if s.Path, err = requiredString(r[3], "source path"); err != nil {
		return s, err
	}
	if s.Device, err = requiredInt64(r[4], "source device"); err != nil {
		return s, err
	}
	if s.Inode, err = requiredInt64(r[5], "source inode"); err != nil {
		return s, err
	}
	if s.BirthNS, err = requiredInt64(r[6], "source birth time"); err != nil {
		return s, err
	}
	if s.MtimeNS, err = requiredInt64(r[7], "source mtime"); err != nil {
		return s, err
	}
	if s.CtimeNS, err = requiredInt64(r[8], "source ctime"); err != nil {
		return s, err
	}
	if s.CurrentModel, err = requiredString(r[11], "current model"); err != nil {
		return s, err
	}
	if s.CurrentProject, err = requiredString(r[12], "current project"); err != nil {
		return s, err
	}
	if s.ParserVersion, err = nonnegative(r[17], "source parser version"); err != nil {
		return s, err
	}
	if !sourceRoot(s.Root).valid() {
		return s, corruptRow("source root")
	}
	if s.ParserVersion != parserVersion {
		return s, corruptRow("source parser version")
	}
	return s, nil
}

func decodeAggregate(r []any) (indexedEvent, error) {
	flag, err := requiredInt64(r[8], "event storage class")
	if err != nil || flag != 0 {
		return indexedEvent{}, corruptRow("event storage class")
	}
	var e indexedEvent
	if e.Timestamp, err = requiredDouble(r[0], "event timestamp"); err != nil {
		return e, err
	}
	if e.DayKey, err = requiredString(r[1], "event day key"); err != nil {
		return e, err
	}
	if e.Model, err = requiredString(r[2], "event model"); err != nil {
		return e, err
	}
	if e.Project, err = requiredString(r[3], "event project"); err != nil {
		return e, err
	}
	if e.Uncached, err = nonnegative(r[4], "uncached tokens"); err != nil {
		return e, err
	}
	if e.Cached, err = nonnegative(r[5], "cached tokens"); err != nil {
		return e, err
	}
	if e.Output, err = nonnegative(r[6], "output tokens"); err != nil {
		return e, err
	}
	if e.Turns, err = nonnegative(r[7], "turn count"); err != nil {
		return e, err
	}
	return e, nil
}

func decodeCachedHash(r []any) (cachedFullHash, error) {
	var c cachedFullHash
	var err error
	size, err := nonnegative(r[3], "cached hash size")
	if err != nil {
		return c, err
	}
	c.Size = uint64(size)
	if c.Identity.Device, err = requiredInt64(r[0], "cached hash device"); err != nil {
		return c, err
	}
	if c.Identity.Inode, err = requiredInt64(r[1], "cached hash inode"); err != nil {
		return c, err
	}
	if c.Identity.BirthNS, err = requiredInt64(r[2], "cached hash birth time"); err != nil {
		return c, err
	}
	if c.MtimeNS, err = requiredInt64(r[4], "cached hash mtime"); err != nil {
		return c, err
	}
	if c.CtimeNS, err = requiredInt64(r[5], "cached hash ctime"); err != nil {
		return c, err
	}
	if c.Digest, err = requiredHash(r[6], "cached full hash"); err != nil {
		return c, err
	}
	return c, nil
}

// ---- reads -----------------------------------------------------------------

func (s *indexStore) sourceRows(ctx context.Context) ([]indexedSource, error) {
	rows, err := s.db.Query(ctx, "SELECT id, "+sourceColumns+" FROM source_files ORDER BY basename")
	if err != nil {
		return nil, err
	}
	out := make([]indexedSource, 0, len(rows.Values))
	for _, r := range rows.Values {
		src, err := decodeSource(r, s.parserVersion)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, nil
}

func (s *indexStore) hashCacheRows(ctx context.Context) ([]cachedFullHash, error) {
	rows, err := s.db.Query(ctx, `SELECT device, inode, birth_ns, size, mtime_ns, ctime_ns, full_hash
FROM source_hash_cache`)
	if err != nil {
		return nil, err
	}
	out := make([]cachedFullHash, 0, len(rows.Values))
	for _, r := range rows.Values {
		c, err := decodeCachedHash(r)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// dayBoundary chooses how events are grouped into days.
type dayBoundary int

const (
	dayLocal dayBoundary = iota
	dayUTC
)

const eventsSQLFormat = `SELECT timestamp, %s AS aggregate_day_key, model, project,
       uncached_input_tokens, cached_input_tokens, output_tokens, turns,
       CASE WHEN
           typeof(timestamp) NOT IN ('real', 'integer') OR
           typeof(day_key) != 'text' OR typeof(model) != 'text' OR
           typeof(project) != 'text' OR
           typeof(uncached_input_tokens) != 'integer' OR
           typeof(cached_input_tokens) != 'integer' OR
           typeof(output_tokens) != 'integer' OR
           typeof(turns) != 'integer'
       THEN 1 ELSE 0 END
FROM usage_events
WHERE timestamp >= ? AND timestamp <= ?%s
ORDER BY aggregate_day_key, model, project, timestamp`

// events returns individual events so request-sized tiers are priced before aggregation.
func (s *indexStore) events(ctx context.Context, start, end float64, b dayBoundary, sourcePath string) ([]indexedEvent, error) {
	key := "day_key"
	if b == dayUTC {
		key = "strftime('%Y-%m-%d', timestamp, 'unixepoch')"
	}
	filter := ""
	args := []any{start, end}
	if sourcePath != "" {
		filter = " AND file_id IN (SELECT id FROM source_files WHERE path = ?)"
		args = append(args, sourcePath)
	}
	rows, err := s.db.Query(ctx, fmt.Sprintf(eventsSQLFormat, key, filter), args...)
	if err != nil {
		return nil, err
	}
	out := make([]indexedEvent, 0, len(rows.Values))
	for _, r := range rows.Values {
		e, err := decodeAggregate(r)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *indexStore) validateEventStorage(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT 1 FROM usage_events
WHERE typeof(file_id) != 'integer'
   OR file_id <= 0
   OR typeof(byte_offset) != 'integer'
   OR byte_offset < 0
   OR typeof(timestamp) NOT IN ('real', 'integer')
   OR timestamp NOT BETWEEN -1.7976931348623157e308 AND 1.7976931348623157e308
   OR typeof(day_key) != 'text'
   OR typeof(model) != 'text'
   OR typeof(project) != 'text'
   OR typeof(uncached_input_tokens) != 'integer'
   OR uncached_input_tokens < 0
   OR typeof(cached_input_tokens) != 'integer'
   OR cached_input_tokens < 0
   OR typeof(output_tokens) != 'integer'
   OR output_tokens < 0
   OR typeof(turns) != 'integer'
   OR turns < 0
LIMIT 1`)
	if err != nil {
		return err
	}
	if len(rows.Values) > 0 {
		return corruptRow("usage event storage")
	}
	return nil
}

// ---- writes ----------------------------------------------------------------

// replaceSource stores final as the row for its basename, deletes the events
// of that file at or after offset and inserts events, all in one transaction.
// It returns the stored row with its id.
func (s *indexStore) replaceSource(ctx context.Context, final indexedSource, offset uint64, events []indexedEvent) (indexedSource, error) {
	if offset > 1<<63-1 {
		return final, fmt.Errorf("usage: integer overflow: event replacement offset")
	}
	args, err := sourceArgs(final)
	if err != nil {
		return final, err
	}
	stmts := []sqlite.Statement{
		{SQL: upsertSourceSQL, Args: args},
		{
			SQL:  "DELETE FROM usage_events WHERE file_id = (SELECT id FROM source_files WHERE basename = ?) AND byte_offset >= ?",
			Args: []any{final.Basename, int64(offset)},
		},
	}
	for i := 0; i < len(events); i += eventRowsPerStatement {
		end := min(i+eventRowsPerStatement, len(events))
		st, err := eventInsert(final.Basename, events[i:end])
		if err != nil {
			return final, err
		}
		stmts = append(stmts, st)
	}
	if _, err := s.db.Transaction(ctx, stmts); err != nil {
		return final, err
	}
	id, err := s.sourceID(ctx, final.Basename)
	if err != nil {
		return final, err
	}
	final.ID = id
	return final, nil
}

func eventInsert(basename string, events []indexedEvent) (sqlite.Statement, error) {
	var b strings.Builder
	b.WriteString(eventInsertHead)
	args := make([]any, 0, len(events)*10)
	for i, e := range events {
		if e.ByteOffset > 1<<63-1 {
			return sqlite.Statement{}, fmt.Errorf("usage: integer overflow: event byte offset")
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("((SELECT id FROM source_files WHERE basename = ?), ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args, basename, int64(e.ByteOffset), e.Timestamp, e.DayKey, e.Model, e.Project,
			e.Uncached, e.Cached, e.Output, e.Turns)
	}
	b.WriteString(eventConflictSQL)
	return sqlite.Statement{SQL: b.String(), Args: args}, nil
}

func (s *indexStore) sourceID(ctx context.Context, basename string) (int64, error) {
	rows, err := s.db.Query(ctx, "SELECT id FROM source_files WHERE basename = ?", basename)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, fmt.Errorf("usage: missing source id for %s", basename)
	}
	return requiredInt64(rows.Values[0][0], "source id")
}

// adoptSource rewrites the metadata of an existing row (same id, same
// basename) without touching its events.
func (s *indexStore) adoptSource(ctx context.Context, src indexedSource) error {
	if src.ID == 0 {
		return fmt.Errorf("usage: missing source id for %s", src.Basename)
	}
	args, err := sourceArgs(src)
	if err != nil {
		return err
	}
	// args[0] is the basename; the update is keyed on id and basename.
	set := make([]string, 0, 19)
	for _, c := range strings.Split(strings.ReplaceAll(sourceColumns, "\n", " "), ",")[1:] {
		set = append(set, strings.TrimSpace(c)+" = ?")
	}
	sql := "UPDATE source_files SET " + strings.Join(set, ", ") + " WHERE id = ? AND basename = ?"
	upd := append(append([]any{}, args[1:]...), src.ID, src.Basename)
	res, err := s.db.Execute(ctx, sql, upd...)
	if err != nil {
		return err
	}
	if res.Changes != 1 {
		return fmt.Errorf("usage: source identity mismatch for %s", src.Basename)
	}
	return nil
}

// removeSources deletes rows (and, through the cascade, their events) whose
// basename is not in retained and returns how many were removed.
func (s *indexStore) removeSources(ctx context.Context, retained map[string]struct{}) (int, error) {
	rows, err := s.sourceRows(ctx)
	if err != nil {
		return 0, err
	}
	var stmts []sqlite.Statement
	for _, r := range rows {
		if _, keep := retained[r.Basename]; !keep {
			stmts = append(stmts, sqlite.Statement{SQL: "DELETE FROM source_files WHERE id = ?", Args: []any{r.ID}})
		}
	}
	if len(stmts) == 0 {
		return 0, nil
	}
	res, err := s.db.Transaction(ctx, stmts)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, r := range res {
		removed += int(r.Changes)
	}
	return removed, nil
}

// replaceHashCache replaces the full-hash cache with rows.
func (s *indexStore) replaceHashCache(ctx context.Context, rows []cachedFullHash) error {
	sorted := append([]cachedFullHash{}, rows...)
	sortCachedHashes(sorted)
	stmts := []sqlite.Statement{{SQL: "DELETE FROM source_hash_cache"}}
	for _, r := range sorted {
		if r.Size > 1<<63-1 {
			return fmt.Errorf("usage: integer overflow: cached hash size")
		}
		stmts = append(stmts, sqlite.Statement{
			SQL: `INSERT INTO source_hash_cache(
    device, inode, birth_ns, size, mtime_ns, ctime_ns, full_hash)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{r.Identity.Device, r.Identity.Inode, r.Identity.BirthNS, int64(r.Size), r.MtimeNS, r.CtimeNS, r.Digest},
		})
	}
	_, err := s.db.Transaction(ctx, stmts)
	return err
}

func sortCachedHashes(rows []cachedFullHash) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].Identity, rows[j].Identity
		if a.Device != b.Device {
			return a.Device < b.Device
		}
		if a.Inode != b.Inode {
			return a.Inode < b.Inode
		}
		return a.BirthNS < b.BirthNS
	})
}
