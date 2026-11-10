package codexdesktop

import (
	"context"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/licoy/quosparr/internal/sqliteruntime"
)

type dbProject struct {
	Name  string
	Roots []string
}
type catalog struct {
	Projects map[string]dbProject
	Threads  map[string]bool
}

// catalog opens only the existing local database, using one compound SELECT
// for a consistent snapshot; query errors never become an empty catalog.
func (s Sharer) catalog() (catalog, error) {
	out := catalog{map[string]dbProject{}, map[string]bool{}}
	path := filepath.Join(s.CodexHome, "state_5.sqlite")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return out, ErrDatabase
	}
	if err := safePath(path); err != nil {
		return out, ErrDatabase
	}
	if err := sqliteruntime.Load(); err != nil {
		return out, ErrDatabase
	}
	ctx := context.Background()
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{ReadOnly: true})
	if err != nil {
		return out, ErrDatabase
	}
	rows, queryErr := db.Query(ctx, `SELECT 'project' AS kind, id, name AS value, NULL AS position FROM projects
 UNION ALL SELECT 'root', project_id, path, position FROM project_roots
 UNION ALL SELECT 'thread', id, NULL, NULL FROM threads
 ORDER BY kind, id, position`)
	closeErr := db.Close()
	if queryErr != nil || closeErr != nil {
		return out, ErrDatabase
	}
	for _, row := range rows.Values {
		kind, kok := row[0].(string)
		id, iok := row[1].(string)
		if !kok || !iok || id == "" {
			return out, ErrDatabase
		}
		if kind == "project" {
			name, ok := row[2].(string)
			if !ok {
				return out, ErrDatabase
			}
			out.Projects[id] = dbProject{Name: name, Roots: []string{}}
		}
	}
	for _, row := range rows.Values {
		kind := row[0].(string)
		id := row[1].(string)
		switch kind {
		case "root":
			path, ok := row[2].(string)
			_, pok := row[3].(int64)
			p, found := out.Projects[id]
			if !ok || !pok || !found || !filepath.IsAbs(path) {
				return out, ErrDatabase
			}
			p.Roots = append(p.Roots, filepath.Clean(path))
			out.Projects[id] = p
		case "thread":
			out.Threads[id] = true
		}
	}
	return out, nil
}
