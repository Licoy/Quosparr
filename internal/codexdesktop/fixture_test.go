package codexdesktop

import (
	"context"
	"encoding/json"
	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/licoy/quosparr/internal/sqliteruntime"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var (
	alice = Owner{UserID: "user-alice", AccountID: "acct-a"}
	bob   = Owner{UserID: "user-bob", AccountID: "acct-b"}
)

type fixture struct {
	t       *testing.T
	sharer  Sharer
	running bool
	roots   map[string]string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	f := &fixture{t: t, roots: map[string]string{}}
	clock := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	f.sharer = Sharer{CodexHome: filepath.Join(root, "codex"), Dir: filepath.Join(root, "quosparr", "codex-desktop"), Running: func() bool { return f.running }, Now: func() time.Time { clock = clock.Add(time.Second); return clock }}
	must(t, os.MkdirAll(f.sharer.CodexHome, 0o700))
	must(t, sqliteruntime.Load())
	db, err := sqlite.Open(context.Background(), filepath.Join(f.sharer.CodexHome, "state_5.sqlite"), sqlite.OpenOptions{})
	must(t, err)
	for _, sql := range []string{"CREATE TABLE projects(id TEXT PRIMARY KEY,name TEXT)", "CREATE TABLE project_roots(project_id TEXT,position INTEGER,path TEXT)", "CREATE TABLE threads(id TEXT PRIMARY KEY)"} {
		_, err = db.Execute(context.Background(), sql)
		must(t, err)
	}
	for _, id := range []string{"p1", "p2", "p3"} {
		path := filepath.Join(root, id)
		must(t, os.Mkdir(path, 0o700))
		f.roots[id] = path
		_, err = db.Execute(context.Background(), "INSERT INTO projects VALUES(?,?)", "db-"+id, "Project "+id)
		must(t, err)
		_, err = db.Execute(context.Background(), "INSERT INTO project_roots VALUES(?,?,?)", "db-"+id, 0, path)
		must(t, err)
	}
	for _, id := range []string{"t1", "t2"} {
		_, err = db.Execute(context.Background(), "INSERT INTO threads VALUES(?)", id)
		must(t, err)
	}
	must(t, db.Close())
	return f
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) project(id string) map[string]any {
	return map[string]any{"id": id, "name": "Project " + id, "rootPaths": []string{f.roots[id]}, "createdAt": float64(12), "updatedAt": float64(15)}
}
func (f *fixture) globals(ids ...string) map[string]any {
	projects, mapping := map[string]any{}, map[string]any{}
	order, sidebar := []string{}, []string{}
	for _, id := range ids {
		projects[id] = f.project(id)
		mapping[id] = "db-" + id
		order = append(order, id)
		sidebar = append(sidebar, "codex:project:"+id)
	}
	return map[string]any{"local-projects": projects, "project-order": order, "app-server-project-id-by-legacy-project-id-by-host": map[string]any{"local:" + f.sharer.CodexHome: mapping}}
}
func (f *fixture) active(target map[string]any) map[string]any {
	inactive := map[string]any{}
	if target != nil {
		inactive[bob.Key()] = map[string]any{"globals": target, "atoms": map[string]any{"thread-sort-key": "bob"}}
	}
	d := f.globals("p1")
	d[rendererKey] = map[string]any{"owner": alice.Key(), "inactive": inactive}
	d[atomsKey] = map[string]any{"unified-sidebar-project-order-v1": []string{"codex:project:p1", "chatgpt:project:secret", "space:secret"}, "private-setting": true}
	d["remote-projects"] = []any{"private-host"}
	d["theme"] = "dark"
	return d
}
func (f *fixture) write(doc map[string]any) {
	f.t.Helper()
	must(f.t, os.WriteFile(f.sharer.statePath(), encode(doc), 0o600))
}
func (f *fixture) read() map[string]any {
	f.t.Helper()
	data, err := os.ReadFile(f.sharer.statePath())
	must(f.t, err)
	var doc map[string]any
	must(f.t, json.Unmarshal(data, &doc))
	return doc
}
func (f *fixture) target() partition {
	f.t.Helper()
	d, err := f.sharer.readState()
	must(f.t, err)
	p, _, err := d.partition(bob.Key())
	must(f.t, err)
	return p
}
func (f *fixture) share() Outcome {
	f.t.Helper()
	p, err := f.sharer.Prepare(Request{alice.Key(), bob})
	must(f.t, err)
	o, err := f.sharer.Apply(p)
	must(f.t, err)
	return o
}
func (f *fixture) backups() []string {
	f.t.Helper()
	matches, err := filepath.Glob(filepath.Join(f.sharer.Dir, "backups", "*"))
	must(f.t, err)
	return matches
}
func hasProject(p partition, id string) bool {
	o, _ := decodeObject(p.Globals["local-projects"])
	return o[id] != nil
}
func moveOwner(doc map[string]any, next string) {
	renderer := doc[rendererKey].(map[string]any)
	inactive := renderer["inactive"].(map[string]any)
	if owner, ok := renderer["owner"].(string); ok {
		globals := map[string]any{}
		for key, v := range doc {
			if key != rendererKey && key != atomsKey {
				globals[key] = v
				delete(doc, key)
			}
		}
		inactive[owner] = map[string]any{"globals": globals, "atoms": doc[atomsKey]}
	}
	delete(doc, atomsKey)
	if next == "" {
		renderer["owner"] = nil
		return
	}
	if raw, ok := inactive[next].(map[string]any); ok {
		for k, v := range raw["globals"].(map[string]any) {
			doc[k] = v
		}
		doc[atomsKey] = raw["atoms"]
		delete(inactive, next)
	}
	renderer["owner"] = next
}
