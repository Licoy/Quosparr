package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/licoy/quosparr/internal/sqliteruntime"
)

// sessionMaxDBBytes bounds one database (plus its WAL) copied for reading.
const sessionMaxDBBytes = 512 << 20

var sessionErrChanged = errors.New("gemini: session database changed while reading")

// sessionFileState is the size and modification time of a database and its
// WAL; the WAL is absent when its size is -1.
type sessionFileState struct {
	dbSize, walSize   int64
	dbMtime, walMtime time.Time
}

func sessionStateOf(path string) (sessionFileState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return sessionFileState{}, err
	}
	if !info.Mode().IsRegular() {
		return sessionFileState{}, fmt.Errorf("gemini: %s is not a regular file", filepath.Base(path))
	}
	state := sessionFileState{dbSize: info.Size(), dbMtime: info.ModTime(), walSize: -1}
	wal, err := os.Lstat(path + "-wal")
	switch {
	case err == nil && wal.Mode().IsRegular():
		state.walSize, state.walMtime = wal.Size(), wal.ModTime()
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return sessionFileState{}, err
	}
	return state, nil
}

func (s sessionFileState) same(o sessionFileState) bool {
	return s.dbSize == o.dbSize && s.walSize == o.walSize && s.dbMtime.Equal(o.dbMtime) && s.walMtime.Equal(o.walMtime)
}

func (s sessionFileState) updatedAt() time.Time {
	if s.walSize > 0 && s.walMtime.After(s.dbMtime) {
		return s.walMtime
	}
	return s.dbMtime
}

// sessionWithCopy runs read on a private copy of a database and its WAL, so
// the live files the clients write are never opened, locked or checkpointed.
// The shared-memory index is not copied: SQLite rebuilds it from the WAL.
// A database that changes while it is copied is retried, then reported.
func sessionWithCopy(ctx context.Context, path string, read func(db *sqlite.DB, copyPath string) error) (sessionFileState, error) {
	if err := sqliteruntime.Load(); err != nil {
		return sessionFileState{}, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return sessionFileState{}, err
		}
		before, err := sessionStateOf(path)
		if err != nil {
			return sessionFileState{}, err
		}
		if before.dbSize+max(0, before.walSize) > sessionMaxDBBytes {
			return sessionFileState{}, fmt.Errorf("gemini: %s is too large", filepath.Base(path))
		}
		dir, err := os.MkdirTemp("", "quosparr-gemini-")
		if err != nil {
			return sessionFileState{}, err
		}
		state, err := sessionCopyAndRead(ctx, path, dir, before, read)
		_ = os.RemoveAll(dir)
		if errors.Is(err, sessionErrChanged) {
			continue
		}
		return state, err
	}
	return sessionFileState{}, sessionErrChanged
}

func sessionCopyAndRead(ctx context.Context, path, dir string, before sessionFileState, read func(db *sqlite.DB, copyPath string) error) (sessionFileState, error) {
	copyPath := filepath.Join(dir, "copy.db")
	if err := sessionCopyFile(path, copyPath); err != nil {
		return sessionFileState{}, err
	}
	if before.walSize >= 0 {
		if err := sessionCopyFile(path+"-wal", copyPath+"-wal"); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return sessionFileState{}, err
		}
	}
	after, err := sessionStateOf(path)
	if err != nil {
		return sessionFileState{}, err
	}
	if !after.same(before) {
		return sessionFileState{}, sessionErrChanged
	}
	db, err := sqlite.Open(ctx, copyPath, sqlite.OpenOptions{BusyTimeout: time.Second})
	if err != nil {
		return sessionFileState{}, err
	}
	err = read(db, copyPath)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	return after, err
}

func sessionCopyFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("gemini: %s is not a regular file", filepath.Base(from))
	}
	dst, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, io.LimitReader(src, sessionMaxDBBytes+1))
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	return err
}

// sessionTables lists the tables of a database.
func sessionTables(ctx context.Context, db *sqlite.DB) (map[string]bool, error) {
	rows, err := db.Query(ctx, "SELECT name FROM sqlite_master WHERE type = 'table'")
	if err != nil {
		return nil, err
	}
	tables := map[string]bool{}
	for _, row := range rows.Values {
		if name, ok := row[0].(string); ok {
			tables[name] = true
		}
	}
	return tables, nil
}

func sessionColumns(ctx context.Context, db *sqlite.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, err
	}
	columns := map[string]bool{}
	for _, row := range rows.Values {
		if name, ok := row[0].(string); ok {
			columns[name] = true
		}
	}
	return columns, nil
}

func sessionInt(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}

func sessionString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}

func sessionBlob(value any) []byte {
	switch v := value.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	}
	return nil
}
