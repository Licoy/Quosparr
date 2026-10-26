package usage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/sqlite"
)

// schemaDump lists every object of the database as the sqlite3 shell query
//
//	select 'TYPE '||type||' NAME '||name||' TBL '||tbl_name||char(10)||coalesce(sql,'<null>')||char(10)
//	from sqlite_master order by name
//
// would print it.
func schemaDump(t *testing.T, path string) string {
	t.Helper()
	db, err := sqlite.Open(context.Background(), path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(context.Background(), "SELECT type, name, tbl_name, sql FROM sqlite_master ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, r := range rows.Values {
		sql := "<null>"
		if s, ok := r[3].(string); ok {
			sql = s
		}
		fmt.Fprintf(&b, "TYPE %v NAME %v TBL %v\n%s\n\n", r[0], r[1], r[2], sql)
	}
	return b.String()
}

func metadataRow(t *testing.T, path string) string {
	t.Helper()
	db, err := sqlite.Open(context.Background(), path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(context.Background(), "SELECT singleton, schema_version, parser_version, aggregation_time_zone FROM index_metadata")
	if err != nil || len(rows.Values) != 1 {
		t.Fatalf("metadata: %v %v", rows, err)
	}
	r := rows.Values[0]
	return fmt.Sprintf("%v %v %v %v", r[0], r[1], r[2], r[3])
}

// TestIndexSchemaMatchesSwift compares the database created by Go with the
// schema dump taken from a database created by the Swift implementation
// (testdata/swift_index_schema.txt, produced by the sqlite3 shell).
func TestIndexSchemaMatchesSwift(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 10), "rollout-x.jsonl")
	r := f.repo(withLoc(mustLoc("Asia/Shanghai")), withParser(CurrentParserVersion))
	sum(t, r, fullQuery())
	r.Close()

	want, err := os.ReadFile("testdata/swift_index_schema.txt")
	if err != nil {
		t.Fatal(err)
	}
	// The shell prints one newline after the SQL; the dump above adds a blank
	// separator line, so compare line by line ignoring blank lines.
	norm := func(s string) []string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			if strings.TrimSpace(l) != "" {
				out = append(out, l)
			}
		}
		return out
	}
	got, exp := norm(schemaDump(t, f.dbPath)), norm(string(want))
	if strings.Join(got, "\n") != strings.Join(exp, "\n") {
		t.Fatalf("schema differs from the Swift-created database\n--- go ---\n%s\n--- swift ---\n%s", strings.Join(got, "\n"), strings.Join(exp, "\n"))
	}
	if m := metadataRow(t, f.dbPath); m != "1 3 4 Asia/Shanghai" {
		t.Fatalf("metadata = %q", m)
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestOpensSwiftCreatedIndexInPlace opens a database written by the Swift
// implementation (schema 3, parser 4) without rebuilding it: with the
// rebuild-report policy any incompatibility would be an error.
func TestOpensSwiftCreatedIndexInPlace(t *testing.T) {
	requireSQLite(t)
	root := t.TempDir()
	home := filepath.Join(root, ".codex")
	for _, d := range []string{"sessions", "archived_sessions"} {
		copyTree(t, filepath.Join("testdata", "swift_compat", d), filepath.Join(home, d))
	}
	db := filepath.Join(root, ".quosparr", "usage-cost-index-v1.sqlite3")
	if err := os.MkdirAll(filepath.Dir(db), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/swift_compat/swift_index.sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sh := mustLoc("Asia/Shanghai")
	r, err := NewRepository(Config{CodexHome: home, DatabasePath: db, Location: sh, Rebuild: RebuildReport})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	q := Query{ID: "full", Window: Window{Start: mustTime("2026-06-27T00:00:00Z"), End: mustTime("2026-06-30T00:00:00Z")}}
	res, err := r.Summaries(context.Background(), []Query{q}, mustTime("2026-06-30T12:00:00Z"), RefreshIfChanged, nil)
	if err != nil {
		t.Fatalf("Swift-created index was not accepted: %v", err)
	}
	s := res["full"]
	if d := r.Diagnostics(); d.DatabaseRebuilds != 0 {
		t.Fatalf("diagnostics %+v", d)
	}
	for _, w := range s.Warnings {
		if strings.HasPrefix(w, "usage-index-rebuilt:") {
			t.Fatalf("index rebuilt: %v", s.Warnings)
		}
	}
	// The numbers the Swift implementation printed for the same input.
	if s.Confidence != ConfidencePartiallyPriced {
		t.Fatalf("confidence %s", s.Confidence)
	}
	if s.Totals.TotalTokens != 4000 || s.Totals.Turns != 2 || s.EstimatedUSD == nil || s.EstimatedUSD.String() != "0.021" {
		t.Fatalf("totals %+v usd %v", s.Totals, s.EstimatedUSD)
	}
	if len(s.Warnings) != 1 || s.Warnings[0] != "unknown-model:mystery-model" {
		t.Fatalf("warnings %v", s.Warnings)
	}
	if len(s.DailyRows) != 2 || s.DailyRows[0].Date != "2026-06-28" || s.DailyRows[0].Totals.TotalTokens != 330 ||
		s.DailyRows[0].EstimatedUSD.String() != "0.0024" || s.DailyRows[1].Totals.TotalTokens != 3670 || s.DailyRows[1].EstimatedUSD.String() != "0.0186" {
		t.Fatalf("days %+v", s.DailyRows)
	}
	if len(s.ProjectRows) != 2 {
		t.Fatalf("projects %+v", s.ProjectRows)
	}
}

// TestCrossImplementationIndexReuse is run by hand against a directory
// prepared by the Swift side (see the USAGE_COMPAT_DIR notes in the report):
// Go must reuse the database Swift wrote (re-reading only an incomplete last line),
// and must leave behind a database Swift can reuse in turn.
func TestCrossImplementationIndexReuse(t *testing.T) {
	dir := os.Getenv("USAGE_COMPAT_DIR")
	if dir == "" {
		t.Skip("set USAGE_COMPAT_DIR to a directory prepared by the Swift probe")
	}
	requireSQLite(t)
	home := filepath.Join(dir, "home", ".codex")
	q := Query{ID: "full", Window: Window{Start: mustTime("2026-06-27T00:00:00Z"), End: mustTime("2026-06-30T00:00:00Z")}}
	run := func(db string) (Summary, Diagnostics) {
		r, err := NewRepository(Config{CodexHome: home, DatabasePath: db, Location: mustLoc("Asia/Shanghai"), Rebuild: RebuildReport})
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		res, err := r.Summaries(context.Background(), []Query{q}, mustTime("2026-06-30T12:00:00Z"), RefreshIfChanged, nil)
		if err != nil {
			t.Fatalf("%s: %v", db, err)
		}
		return res["full"], r.Diagnostics()
	}
	swiftDB := filepath.Join(dir, "swift.sqlite3")
	s, d := run(swiftDB)
	t.Logf("go over swift db: tokens=%d usd=%v diag=%+v", s.Totals.TotalTokens, s.EstimatedUSD, d)
	// Only the incomplete last line of one log (105 bytes) is read again, as
	// in the Swift implementation's own second pass.
	if d.BytesRead > 105 || d.DatabaseRebuilds != 0 || d.RebuiltFiles != 0 || d.AdoptedFiles != 0 || d.CacheHits == 0 {
		t.Fatalf("Swift-written index was not reused as is: %+v", d)
	}
	goDB := filepath.Join(dir, "go.sqlite3")
	for _, p := range []string{goDB, goDB + "-wal", goDB + "-shm"} {
		os.Remove(p)
	}
	s2, d2 := run(goDB)
	t.Logf("go fresh db: tokens=%d diag=%+v", s2.Totals.TotalTokens, d2)
	if d2.BytesRead == 0 {
		t.Fatal("fresh database must read the logs")
	}
}
