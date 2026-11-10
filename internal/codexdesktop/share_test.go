package codexdesktop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/licoy/quosparr/internal/platform"
)

func TestOwnerKeyMatchesDesktopApp(t *testing.T) {
	if alice.Key() != "a4515b3f87fe32de6cea9d6e7591c4efd6ac44581c64240d12c1e50f410911db" {
		t.Fatal("wrong partition hash")
	}
	if string(encode([]string{"<a>&", "b"})) != `["<a>&","b"]` {
		t.Fatal("escaped identity JSON")
	}
}
func TestPrepareReadOnlyAndShareLocalOnly(t *testing.T) {
	f := newFixture(t)
	doc := f.active(nil)
	doc["app-server-pending-project-deletions-by-host"] = map[string]any{"local": []string{"p1"}}
	f.write(doc)
	before, _ := os.ReadFile(f.sharer.statePath())
	p, err := f.sharer.Prepare(Request{alice.Key(), bob})
	must(t, err)
	if p.Summary.Projects != 1 {
		t.Fatalf("summary=%+v", p.Summary)
	}
	after, _ := os.ReadFile(f.sharer.statePath())
	if !bytes.Equal(before, after) || len(f.backups()) != 0 {
		t.Fatal("preview wrote files")
	}
	got, err := f.sharer.Apply(p)
	must(t, err)
	if got != Shared {
		t.Fatal(got)
	}
	target := f.target()
	if !hasProject(target, "p1") {
		t.Fatal("project missing")
	}
	for _, private := range []string{"remote-projects", "theme", "app-server-pending-project-deletions-by-host"} {
		if target.Globals[private] != nil {
			t.Fatal("private field copied", private)
		}
	}
	if !sameJSON(target.Atoms["unified-sidebar-project-order-v1"], encode([]string{"codex:project:p1"})) {
		t.Fatalf("unsafe sidebar %s", target.Atoms["unified-sidebar-project-order-v1"])
	}
	projects, _ := decodeObject(target.Globals["local-projects"])
	item, _ := decodeObject(projects["p1"])
	if !sameJSON(item["createdAt"], encode(12)) || !sameJSON(item["updatedAt"], encode(15)) {
		t.Fatal("project timestamps lost")
	}
	bak, err := os.ReadFile(f.sharer.statePath() + ".bak")
	must(t, err)
	main, _ := os.ReadFile(f.sharer.statePath())
	if !bytes.Equal(bak, main) {
		t.Fatal("bak differs")
	}
	for _, path := range []string{filepath.Join(f.backups()[0], "state.json"), filepath.Join(f.backups()[0], "undo.json")} {
		must(t, platform.VerifyOwnerOnly(path, 0o600))
	}
	if f.share() != Unchanged || len(f.backups()) != 1 {
		t.Fatal("second share changed data")
	}
}
func TestExplicitSourceAcrossPartitionLocations(t *testing.T) {
	for _, owner := range []string{"", bob.Key(), alice.Key()} {
		t.Run(owner, func(t *testing.T) {
			f := newFixture(t)
			doc := f.active(nil)
			if owner != alice.Key() {
				moveOwner(doc, owner)
			}
			f.write(doc)
			if f.share() != Shared || !hasProject(f.target(), "p1") {
				t.Fatal("source/target location was mistaken")
			}
		})
	}
}
func TestAdditiveMergePreservesHostMappingsAndSettings(t *testing.T) {
	f := newFixture(t)
	target := f.globals("p2")
	target["theme"] = "bob-theme"
	f.write(f.active(target))
	f.share()
	p := f.target()
	if !hasProject(p, "p1") || !hasProject(p, "p2") {
		t.Fatal("project lost")
	}
	hosts, _ := decodeObject(p.Globals["app-server-project-id-by-legacy-project-id-by-host"])
	mapping, _ := decodeObject(hosts["local:"+f.sharer.CodexHome])
	if len(mapping) != 2 {
		t.Fatal("host mapping overwritten")
	}
	if !sameJSON(p.Globals["theme"], encode("bob-theme")) || !sameJSON(p.Atoms["thread-sort-key"], encode("bob")) {
		t.Fatal("target settings changed")
	}
	doc := f.read()
	doc["local-projects"] = map[string]any{}
	delete(doc, "project-order")
	f.write(doc)
	f.share()
	if !hasProject(f.target(), "p1") || !hasProject(f.target(), "p2") {
		t.Fatal("source deletion propagated")
	}
}
func TestProjectAndMappingConflicts(t *testing.T) {
	for _, kind := range []string{"roots", "mapping", "cloud"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			target := f.globals("p1")
			switch kind {
			case "roots":
				target["local-projects"].(map[string]any)["p1"].(map[string]any)["rootPaths"] = []string{f.roots["p2"]}
			case "mapping":
				target["app-server-project-id-by-legacy-project-id-by-host"].(map[string]any)["local:"+f.sharer.CodexHome].(map[string]any)["p1"] = "different"
			case "cloud":
				target["local-projects"].(map[string]any)["p1"].(map[string]any)["chatGptBacking"] = map[string]any{"id": "secret"}
			}
			f.write(f.active(target))
			before, _ := os.ReadFile(f.sharer.statePath())
			_, err := f.sharer.Prepare(Request{alice.Key(), bob})
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("err=%v", err)
			}
			after, _ := os.ReadFile(f.sharer.statePath())
			if !bytes.Equal(before, after) {
				t.Fatal("conflict wrote data")
			}
		})
	}
}
func TestCloudProjectsNeverCrossAccounts(t *testing.T) {
	for _, field := range []string{"chatGptBacking", "chatGptBackingCreation", "retiredChatGptBacking", "chatGptBackingRecoveryPending", "g-p-"} {
		t.Run(field, func(t *testing.T) {
			f := newFixture(t)
			doc := f.active(nil)
			id := "p1"
			if field == "g-p-" {
				id = "g-p-mirror"
				doc["local-projects"] = map[string]any{id: map[string]any{"id": id}}
			} else {
				doc["local-projects"].(map[string]any)[id].(map[string]any)[field] = true
			}
			f.write(doc)
			p, err := f.sharer.Prepare(Request{alice.Key(), bob})
			must(t, err)
			if p.Summary.Projects != 0 || p.Summary.Skipped != 1 || !reflect.DeepEqual(p.Summary.Reasons, []string{"cloud_project"}) {
				t.Fatalf("%+v", p.Summary)
			}
			got, err := f.sharer.Apply(p)
			must(t, err)
			if got != Unchanged {
				t.Fatal("cloud project shared")
			}
		})
	}
}
func TestThreadValidationAndIsolation(t *testing.T) {
	f := newFixture(t)
	doc := f.active(nil)
	assignment := func(project, kind string) map[string]any {
		return map[string]any{"projectId": project, "projectKind": kind, "hostId": "local"}
	}
	doc["thread-project-assignments"] = map[string]any{"t1": assignment("p1", "local"), "missing": assignment("p1", "local"), "projectless": assignment("p1", "local"), "t2": assignment("p1", "remote")}
	doc["thread-project-membership-host-ids"] = map[string]any{"t1": "local"}
	doc["projectless-thread-ids"] = []string{"projectless"}
	doc["pinned-thread-ids"] = []string{"t1", "missing", "projectless", "t2"}
	doc["sidebar-project-thread-orders"] = map[string]any{"p1": map[string]any{"sortKey": "created_at", "threadIds": []string{"t1", "missing", "t2"}}}
	f.write(doc)
	p, err := f.sharer.Prepare(Request{alice.Key(), bob})
	must(t, err)
	if p.Summary.Threads != 1 || p.Summary.Skipped != 3 {
		t.Fatalf("%+v", p.Summary)
	}
	_, err = f.sharer.Apply(p)
	must(t, err)
	target := f.target()
	assignments, _ := decodeObject(target.Globals["thread-project-assignments"])
	if len(assignments) != 1 || assignments["t1"] == nil {
		t.Fatal("invalid chat copied")
	}
	if !sameJSON(target.Globals["pinned-thread-ids"], encode([]string{"t1"})) {
		t.Fatal("invalid pinned chat")
	}
	orders, _ := decodeObject(target.Globals["sidebar-project-thread-orders"])
	if !sameJSON(orders["p1"], encode(map[string]any{"sortKey": "created_at", "threadIds": []string{"t1"}})) {
		t.Fatal("invalid chat order")
	}
}
