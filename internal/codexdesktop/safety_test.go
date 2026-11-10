package codexdesktop

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/plugins/sqlite"
)

func TestApplyRejectsProcessAndContentRaces(t *testing.T) {
	for _, kind := range []string{"running", "source", "target", "database"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.write(f.active(nil))
			p, err := f.sharer.Prepare(Request{alice.Key(), bob})
			must(t, err)
			switch kind {
			case "running":
				f.running = true
			case "source":
				doc := f.read()
				doc["project-order"] = []string{}
				f.write(doc)
			case "target":
				doc := f.read()
				renderer := doc[rendererKey].(map[string]any)
				renderer["inactive"].(map[string]any)[bob.Key()] = map[string]any{"globals": f.globals("p2"), "atoms": map[string]any{}}
				f.write(doc)
			case "database":
				db, err := sqlite.Open(context.Background(), filepath.Join(f.sharer.CodexHome, "state_5.sqlite"), sqlite.OpenOptions{})
				must(t, err)
				_, err = db.Execute(context.Background(), "DELETE FROM threads WHERE id=?", "t1")
				must(t, err)
				must(t, db.Close())
			}
			before, _ := os.ReadFile(f.sharer.statePath())
			_, err = f.sharer.Apply(p)
			expected := ErrStateChanged
			if kind == "running" {
				expected = ErrDesktopRunning
			}
			if !errors.Is(err, expected) {
				t.Fatalf("err=%v", err)
			}
			after, _ := os.ReadFile(f.sharer.statePath())
			if !bytes.Equal(before, after) || len(f.backups()) != 0 {
				t.Fatal("race wrote data")
			}
		})
	}
}
func TestApplyAllowsAccountPartitionMigration(t *testing.T) {
	f := newFixture(t)
	f.write(f.active(nil))
	p, err := f.sharer.Prepare(Request{alice.Key(), bob})
	must(t, err)
	doc := f.read()
	moveOwner(doc, bob.Key())
	f.write(doc)
	got, err := f.sharer.Apply(p)
	must(t, err)
	if got != Shared || !hasProject(f.target(), "p1") {
		t.Fatal("migration prevented restoration")
	}
}
func TestRestoreLatestPreservesUnrelatedUpdatesAfterMigration(t *testing.T) {
	f := newFixture(t)
	f.write(f.active(f.globals("p2")))
	f.share()
	doc := f.read()
	moveOwner(doc, bob.Key())
	doc["theme"] = "new-theme"
	doc["unrelated-update"] = true
	f.write(doc)
	must(t, f.sharer.RestoreLatest())
	p := f.target()
	if hasProject(p, "p1") || !hasProject(p, "p2") {
		t.Fatal("undo project delta wrong")
	}
	if !sameJSON(p.Globals["theme"], encode("new-theme")) || !sameJSON(p.Globals["unrelated-update"], encode(true)) {
		t.Fatal("unrelated update overwritten")
	}
	d, err := f.sharer.readState()
	must(t, err)
	source, _, err := d.partition(alice.Key())
	must(t, err)
	if !hasProject(source, "p1") {
		t.Fatal("source overwritten")
	}
}
func TestRestoreRejectsChangedSharedContent(t *testing.T) {
	f := newFixture(t)
	f.write(f.active(nil))
	f.share()
	doc := f.read()
	moveOwner(doc, bob.Key())
	doc["project-order"] = []string{"p1", "p3"}
	f.write(doc)
	before, _ := os.ReadFile(f.sharer.statePath())
	if err := f.sharer.RestoreLatest(); !errors.Is(err, ErrStateChanged) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(f.sharer.statePath())
	if !bytes.Equal(before, after) {
		t.Fatal("undo overwrote user projects")
	}
}
func TestRestoreEmptyTargetAndMissingBackup(t *testing.T) {
	f := newFixture(t)
	if err := f.sharer.RestoreLatest(); !errors.Is(err, ErrNoBackup) {
		t.Fatal(err)
	}
	f.write(f.active(nil))
	f.share()
	must(t, f.sharer.RestoreLatest())
	d, err := f.sharer.readState()
	must(t, err)
	_, found, err := d.partition(bob.Key())
	must(t, err)
	if found {
		t.Fatal("empty target partition not removed")
	}
	f.running = true
	if err := f.sharer.RestoreLatest(); !errors.Is(err, ErrDesktopRunning) {
		t.Fatal(err)
	}
}
func TestOriginalMainBakAndSharedRecordBackedUp(t *testing.T) {
	f := newFixture(t)
	f.write(f.active(nil))
	main, _ := os.ReadFile(f.sharer.statePath())
	bak := []byte(`{"original":"bak"}`)
	record := []byte(`{"written":{"legacy":"fingerprint"}}`)
	must(t, os.WriteFile(f.sharer.statePath()+".bak", bak, 0o600))
	must(t, os.MkdirAll(f.sharer.Dir, 0o700))
	must(t, os.WriteFile(f.sharer.writtenPath(), record, 0o600))
	f.share()
	for name, want := range map[string][]byte{"state.json": main, "state.bak": bak, "shared-state.json": record} {
		got, err := os.ReadFile(filepath.Join(f.backups()[0], name))
		must(t, err)
		if !bytes.Equal(got, want) {
			t.Fatal("original not backed up", name)
		}
	}
}
func TestRejectsBackupFailureAndSymlinkTargets(t *testing.T) {
	for _, kind := range []string{"backup", "main-link", "bak-link", "record-link"} {
		t.Run(kind, func(t *testing.T) {
			if kind != "backup" && runtime.GOOS == "windows" {
				t.Skip("symlink creation needs privileges on Windows")
			}
			f := newFixture(t)
			f.write(f.active(nil))
			p, err := f.sharer.Prepare(Request{alice.Key(), bob})
			must(t, err)
			switch kind {
			case "backup":
				must(t, os.MkdirAll(f.sharer.Dir, 0o700))
				must(t, os.WriteFile(filepath.Join(f.sharer.Dir, "backups"), []byte("blocked"), 0o600))
			case "main-link":
				must(t, os.Rename(f.sharer.statePath(), f.sharer.statePath()+".real"))
				must(t, os.Symlink(f.sharer.statePath()+".real", f.sharer.statePath()))
			case "bak-link":
				must(t, os.Symlink(f.sharer.statePath(), f.sharer.statePath()+".bak"))
			case "record-link":
				must(t, os.MkdirAll(f.sharer.Dir, 0o700))
				must(t, os.Symlink(f.sharer.statePath(), f.sharer.writtenPath()))
			}
			before, _ := os.ReadFile(f.sharer.statePath())
			_, err = f.sharer.Apply(p)
			if err == nil {
				t.Fatal("unsafe write accepted")
			}
			after, _ := os.ReadFile(f.sharer.statePath())
			if !bytes.Equal(before, after) {
				t.Fatal("original changed after backup failure")
			}
		})
	}
}
func TestMissingOrUnknownDatabaseFailsWithoutCreation(t *testing.T) {
	for _, kind := range []string{"missing", "schema"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.write(f.active(nil))
			path := filepath.Join(f.sharer.CodexHome, "state_5.sqlite")
			if kind == "missing" {
				must(t, os.Remove(path))
			} else {
				db, err := sqlite.Open(context.Background(), path, sqlite.OpenOptions{})
				must(t, err)
				_, err = db.Execute(context.Background(), "DROP TABLE project_roots")
				must(t, err)
				must(t, db.Close())
			}
			_, err := f.sharer.Prepare(Request{alice.Key(), bob})
			if !errors.Is(err, ErrDatabase) {
				t.Fatal(err)
			}
			if kind == "missing" {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing db created")
				}
			}
		})
	}
}
func TestProjectValidationRequiresMappingAndUsableDirectory(t *testing.T) {
	for _, kind := range []string{"mapping", "database-project", "mismatch", "missing-directory"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			doc := f.active(nil)
			switch kind {
			case "mapping":
				delete(doc, "app-server-project-id-by-legacy-project-id-by-host")
			case "database-project":
				doc["app-server-project-id-by-legacy-project-id-by-host"].(map[string]any)["local:"+f.sharer.CodexHome].(map[string]any)["p1"] = "absent"
			case "mismatch":
				doc["local-projects"].(map[string]any)["p1"].(map[string]any)["rootPaths"] = []string{f.roots["p2"]}
			case "missing-directory":
				must(t, os.Remove(f.roots["p1"]))
			}
			f.write(doc)
			p, err := f.sharer.Prepare(Request{alice.Key(), bob})
			must(t, err)
			if p.Summary.Projects != 0 || p.Summary.Skipped != 1 {
				t.Fatalf("%+v", p.Summary)
			}
		})
	}
}
func TestShareNeverGuessesInactiveSource(t *testing.T) {
	f := newFixture(t)
	doc := f.active(nil)
	moveOwner(doc, "")
	f.write(doc)
	_, err := f.sharer.Share(bob)
	if !errors.Is(err, ErrNoSource) {
		t.Fatal(err)
	}
	sources, err := f.sharer.Sources()
	must(t, err)
	if len(sources) != 1 || sources[0].Key != alice.Key() {
		t.Fatal("explicit source not listed")
	}
	f.sharer.Running = nil
	if got, err := f.sharer.Share(bob); err != nil || got != Unsupported {
		t.Fatalf("%s %v", got, err)
	}
}
