package gemini

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/sqliteruntime"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// Synthetic fixtures: every table and protobuf blob below is built here, in
// the shape the clients write, with made-up values.

func sessionMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func sessionPBKey(num, wire int) []byte {
	return binary.AppendUvarint(nil, uint64(num)<<3|uint64(wire))
}

func sessionPBVarint(num int, v uint64) []byte {
	return binary.AppendUvarint(sessionPBKey(num, 0), v)
}

func sessionPBBytes(num int, data []byte) []byte {
	out := binary.AppendUvarint(sessionPBKey(num, 2), uint64(len(data)))
	return append(out, data...)
}

func sessionPBCat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func sessionPBTime(num int, t time.Time) []byte {
	return sessionPBBytes(num, sessionPBCat(sessionPBVarint(1, uint64(t.Unix())), sessionPBVarint(2, uint64(t.Nanosecond()))))
}

func sessionPBDuration(num int, d time.Duration) []byte {
	return sessionPBBytes(num, sessionPBCat(sessionPBVarint(1, uint64(d/time.Second)), sessionPBVarint(2, uint64(d%time.Second))))
}

type sessionTestUsage struct{ input, cacheWrite, cacheRead, thinking, response int64 }

func (u sessionTestUsage) encode(num int) []byte {
	return sessionPBBytes(num, sessionPBCat(
		sessionPBVarint(1, 1318), sessionPBVarint(2, uint64(u.input)), sessionPBVarint(3, uint64(u.thinking+u.response)),
		sessionPBVarint(4, uint64(u.cacheWrite)), sessionPBVarint(5, uint64(u.cacheRead)), sessionPBVarint(6, 2),
		sessionPBVarint(9, uint64(u.thinking)), sessionPBVarint(10, uint64(u.response)),
	))
}

type sessionTestGen struct {
	model        string
	lastStep     int // -1 for none
	claude       bool
	usage        *sessionTestUsage
	ttft, stream time.Duration
}

func (g sessionTestGen) encode() []byte {
	call := sessionPBCat(sessionPBBytes(1, []byte("system")), sessionPBVarint(3, 1318))
	if g.usage != nil {
		call = append(call, g.usage.encode(4)...)
	} else {
		call = append(call, sessionPBBytes(4, sessionPBVarint(1, 1318))...)
	}
	if g.ttft > 0 {
		call = append(call, sessionPBDuration(11, g.ttft)...)
	}
	if g.stream > 0 {
		call = append(call, sessionPBDuration(12, g.stream)...)
	}
	if g.model != "" {
		call = append(call, sessionPBBytes(19, []byte(g.model))...)
	}
	entry := func(k, v string) []byte {
		return sessionPBBytes(20, sessionPBCat(sessionPBBytes(1, []byte(k)), sessionPBBytes(2, []byte(v))))
	}
	if g.lastStep >= 0 {
		call = append(call, entry("last_step_index", strconv.Itoa(g.lastStep))...)
	}
	call = append(call, entry("used_claude", strconv.FormatBool(g.claude))...)
	return sessionPBCat(sessionPBBytes(1, call), sessionPBBytes(3, sessionPBBytes(28, []byte(g.model+"-high"))))
}

type sessionTestStep struct {
	kind  int
	at    time.Time
	usage *sessionTestUsage
}

func (s sessionTestStep) encode() []byte {
	meta := sessionPBCat(sessionPBTime(1, s.at), sessionPBVarint(3, 1))
	if s.usage != nil {
		meta = append(meta, s.usage.encode(9)...)
	}
	return meta
}

type sessionTestConversation struct {
	steps []sessionTestStep
	gens  []sessionTestGen
	raw   [][]byte // extra gen_metadata rows
}

func sessionOpen(t *testing.T, path string) *sqlite.DB {
	t.Helper()
	sessionMust(t, sqliteruntime.Load())
	sessionMust(t, os.MkdirAll(filepath.Dir(path), 0o700))
	db, err := sqlite.Open(context.Background(), path, sqlite.OpenOptions{})
	sessionMust(t, err)
	return db
}

func sessionExec(t *testing.T, db *sqlite.DB, sql string, args ...any) {
	t.Helper()
	_, err := db.Execute(context.Background(), sql, args...)
	sessionMust(t, err)
}

// sessionWriteConversation creates <root>/conversations/<id>.db. With keepWAL
// the connection stays open in WAL mode, so the rows live only in the WAL.
func sessionWriteConversation(t *testing.T, root, id string, conv sessionTestConversation, keepWAL bool) string {
	t.Helper()
	path := filepath.Join(root, "conversations", id+".db")
	db := sessionOpen(t, path)
	if keepWAL {
		_, err := db.Query(context.Background(), "PRAGMA journal_mode=WAL")
		sessionMust(t, err)
		_, err = db.Query(context.Background(), "PRAGMA wal_autocheckpoint=0")
		sessionMust(t, err)
	}
	sessionExec(t, db, "CREATE TABLE steps(idx INTEGER PRIMARY KEY, step_type INTEGER, status INTEGER, has_subtrajectory INTEGER, metadata BLOB, step_payload BLOB, step_format INTEGER)")
	sessionExec(t, db, "CREATE TABLE gen_metadata(idx INTEGER PRIMARY KEY, data BLOB, size INTEGER)")
	sessionExec(t, db, "CREATE TABLE trajectory_meta(trajectory_id TEXT, cascade_id TEXT, trajectory_type INTEGER, source INTEGER)")
	for i, step := range conv.steps {
		sessionExec(t, db, "INSERT INTO steps(idx, step_type, status, metadata) VALUES(?, ?, 3, ?)", i, step.kind, step.encode())
	}
	idx := 0
	for _, gen := range conv.gens {
		data := gen.encode()
		sessionExec(t, db, "INSERT INTO gen_metadata VALUES(?, ?, ?)", idx, data, len(data))
		idx++
	}
	for _, data := range conv.raw {
		sessionExec(t, db, "INSERT INTO gen_metadata VALUES(?, ?, ?)", idx, data, len(data))
		idx++
	}
	if keepWAL {
		t.Cleanup(func() { _ = db.Close() })
	} else {
		sessionMust(t, db.Close())
	}
	return path
}

type sessionTestSummary struct {
	id, title, preview, workspace string
	steps                         int
	modified                      string
}

func sessionWriteSummaries(t *testing.T, root string, rows ...sessionTestSummary) string {
	t.Helper()
	path := filepath.Join(root, sessionSummariesFile)
	db := sessionOpen(t, path)
	defer func() { sessionMust(t, db.Close()) }()
	sessionExec(t, db, `CREATE TABLE IF NOT EXISTS conversation_summaries(conversation_id TEXT PRIMARY KEY, title TEXT, preview TEXT,
		step_count INTEGER, last_modified_time DATETIME, workspace_uris TEXT, status INTEGER, source INTEGER,
		last_user_input_time DATETIME, app_data_dir TEXT, raw_summary BLOB)`)
	sessionExec(t, db, "CREATE TABLE IF NOT EXISTS conversation_parents(conversation_id TEXT PRIMARY KEY, parent_id TEXT)")
	sessionExec(t, db, "CREATE VIRTUAL TABLE IF NOT EXISTS conversations_fts USING fts5(conversation_id UNINDEXED, chunk_index UNINDEXED, title, workspace, user_body)")
	for _, row := range rows {
		uris := "[]"
		if row.workspace != "" {
			uris = `["file://` + filepath.ToSlash(row.workspace) + `"]`
		}
		sessionExec(t, db, "INSERT INTO conversation_summaries VALUES(?, ?, ?, ?, ?, ?, 1, 1, '0001-01-01 00:00:00+00:00', ?, x'0a00')",
			row.id, row.title, row.preview, row.steps, row.modified, uris, filepath.Base(root))
		sessionExec(t, db, "INSERT INTO conversation_parents VALUES(?, 'parent')", row.id)
		sessionExec(t, db, "INSERT INTO conversations_fts VALUES(?, 0, ?, 'w', 'body')", row.id, row.title)
	}
	return path
}

func sessionCount(t *testing.T, path, table, id string) int64 {
	t.Helper()
	db := sessionOpen(t, path)
	defer db.Close()
	rows, err := db.Query(context.Background(), "SELECT count(*) FROM "+table+" WHERE conversation_id = ?", id)
	sessionMust(t, err)
	return sessionInt(rows.Values[0][0])
}

func sessionDec(s string) *decimal.Decimal {
	d := decimal.MustParse(s)
	return &d
}

func sessionTestSnapshot(t *testing.T) pricing.Snapshot {
	t.Helper()
	snapshot, err := pricing.NewSnapshot([]pricing.Model{
		{ProviderID: "google", ID: "gemini-3.8-flash", Rate: pricing.Rate{Input: sessionDec("0.5"), Output: sessionDec("3"), CacheRead: sessionDec("0.05")}},
		{ProviderID: "google", ID: "gemini-3.8-pro", Rate: pricing.Rate{Input: sessionDec("2"), Output: sessionDec("12"), CacheRead: sessionDec("0.2")},
			Tiers: []pricing.Tier{{Size: 200_000, Type: "context", Rate: pricing.Rate{Input: sessionDec("4"), Output: sessionDec("18"), CacheRead: sessionDec("0.4")}}}},
		{ProviderID: "anthropic", ID: "claude-sonnet-5", Rate: pricing.Rate{Input: sessionDec("3"), Output: sessionDec("15"), CacheRead: sessionDec("0.3"), CacheWrite: sessionDec("3.75")}},
	})
	sessionMust(t, err)
	return snapshot
}

type sessionTestHome struct {
	home, app, cli string
}

func sessionNewTestHome(t *testing.T) sessionTestHome {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	sessionMust(t, err)
	gemini := filepath.Join(home, ".gemini")
	return sessionTestHome{home: gemini, app: filepath.Join(gemini, "antigravity"), cli: filepath.Join(gemini, "antigravity-cli")}
}

func (h sessionTestHome) scanner(t *testing.T) SessionScanner {
	snapshot := sessionTestSnapshot(t)
	return SessionScanner{GeminiHome: h.home, Location: time.UTC, PricingSnapshot: func() pricing.Snapshot { return snapshot }}
}
