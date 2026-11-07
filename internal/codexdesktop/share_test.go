package codexdesktop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var (
	alice = Owner{UserID: "user-alice", AccountID: "acct-a"}
	bob   = Owner{UserID: "user-bob", AccountID: "acct-b"}
	t0    = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
)

type fixture struct {
	t       *testing.T
	sharer  Sharer
	running bool
}

func newFixture(t *testing.T) *fixture {
	root := t.TempDir()
	f := &fixture{t: t}
	clock := t0
	f.sharer = Sharer{
		CodexHome: filepath.Join(root, "codex"),
		Dir:       filepath.Join(root, "quosparr", "codex-desktop"),
		Running:   func() bool { return f.running },
		Now:       func() time.Time { clock = clock.Add(time.Second); return clock },
	}
	if err := os.MkdirAll(f.sharer.CodexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) write(doc map[string]any) {
	f.t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.sharer.statePath(), data, 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) read() map[string]any {
	f.t.Helper()
	data, err := os.ReadFile(f.sharer.statePath())
	if err != nil {
		f.t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		f.t.Fatal(err)
	}
	return doc
}

func (f *fixture) share(target Owner) Outcome {
	f.t.Helper()
	got, err := f.sharer.Share(target)
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

// partition returns the stashed globals and atoms of o.
func partition(t *testing.T, doc map[string]any, o Owner) (globals, atoms map[string]any) {
	t.Helper()
	renderer := doc[rendererKey].(map[string]any)
	entry, ok := renderer["inactive"].(map[string]any)[o.Key()].(map[string]any)
	if !ok {
		t.Fatalf("no partition for %s", o.UserID)
	}
	return entry["globals"].(map[string]any), entry["atoms"].(map[string]any)
}

// aliceActive is Alice's state at the top level, Bob's stashed.
func aliceActive(bobGlobals map[string]any) map[string]any {
	inactive := map[string]any{}
	if bobGlobals != nil {
		inactive[bob.Key()] = map[string]any{"atoms": map[string]any{"thread-sort-key": "bob"}, "globals": bobGlobals}
	}
	return map[string]any{
		rendererKey:         map[string]any{"owner": alice.Key(), "inactive": inactive},
		"local-projects":    map[string]any{"p1": map[string]any{"id": "p1", "rootPaths": []string{"/w/one"}}},
		"project-order":     []string{"p1"},
		"remote-projects":   []any{map[string]any{"hostId": "h"}},
		"queued-follow-ups": map[string]any{"t": []any{}},
		"opaqueWindows":     true,
		atomsKey: map[string]any{
			"unified-sidebar-project-order-v1": []string{"p1"},
			"sidebar-collapsed-sections-v1":    map[string]any{"chats": true},
		},
	}
}

func TestOwnerKeyMatchesDesktopApp(t *testing.T) {
	// sha256 of the JavaScript JSON.stringify(["user-alice","acct-a"]).
	if got := alice.Key(); got != "a4515b3f87fe32de6cea9d6e7591c4efd6ac44581c64240d12c1e50f410911db" {
		t.Fatalf("key = %s", got)
	}
	if string(encode([]string{"<a>&", "b"})) != `["<a>&","b"]` {
		t.Fatal("identity JSON is HTML-escaped unlike JSON.stringify")
	}
}

func TestShareCreatesTargetPartitionWithOnlyProjectKeys(t *testing.T) {
	f := newFixture(t)
	f.write(aliceActive(nil))
	if got := f.share(bob); got != Shared {
		t.Fatalf("outcome %q", got)
	}
	doc := f.read()
	globals, atoms := partition(t, doc, bob)
	if !reflect.DeepEqual(globals["local-projects"], doc["local-projects"]) || !reflect.DeepEqual(globals["project-order"], []any{"p1"}) {
		t.Fatalf("projects not carried: %v", globals)
	}
	for _, private := range []string{"remote-projects", "queued-follow-ups", "opaqueWindows"} {
		if _, ok := globals[private]; ok {
			t.Errorf("%s crossed accounts", private)
		}
	}
	if !reflect.DeepEqual(atoms, map[string]any{"unified-sidebar-project-order-v1": []any{"p1"}}) {
		t.Fatalf("atoms = %v", atoms)
	}
	if renderer := doc[rendererKey].(map[string]any); renderer["owner"] != alice.Key() {
		t.Fatal("owner changed")
	}
	if backups, _ := filepath.Glob(filepath.Join(f.sharer.Dir, "backups", "*")); len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	bak, err := os.ReadFile(f.sharer.statePath() + ".bak")
	if main, _ := os.ReadFile(f.sharer.statePath()); err != nil || string(bak) != string(main) {
		t.Fatal("the .bak copy does not mirror the state file")
	}
}

func TestFirstShareMergesThenPoolReplaces(t *testing.T) {
	f := newFixture(t)
	f.write(aliceActive(map[string]any{
		"local-projects":  map[string]any{"p2": map[string]any{"id": "p2"}},
		"project-order":   []string{"p2"},
		"remote-projects": []any{"bob-host"},
	}))
	f.share(bob)
	globals, atoms := partition(t, f.read(), bob)
	if got := globals["local-projects"].(map[string]any); len(got) != 2 {
		t.Fatalf("merge lost projects: %v", got)
	}
	if !reflect.DeepEqual(globals["project-order"], []any{"p1", "p2"}) || !reflect.DeepEqual(globals["remote-projects"], []any{"bob-host"}) {
		t.Fatalf("globals = %v", globals)
	}
	if atoms["thread-sort-key"] != "bob" {
		t.Fatal("bob's own atoms were dropped")
	}

	// Both are pooled now: Alice removed p1, Bob must follow.
	doc := aliceActive(nil)
	doc[rendererKey] = f.read()[rendererKey]
	doc["local-projects"] = map[string]any{}
	delete(doc, "project-order")
	f.write(doc)
	f.share(bob)
	globals, _ = partition(t, f.read(), bob)
	if len(globals["local-projects"].(map[string]any)) != 0 {
		t.Fatalf("removed project survived: %v", globals["local-projects"])
	}
	if _, ok := globals["project-order"]; ok {
		t.Fatal("a key the owner no longer has was kept")
	}
	if !reflect.DeepEqual(globals["remote-projects"], []any{"bob-host"}) {
		t.Fatal("isolated key was touched")
	}

	if err := f.sharer.ResetPool(); err != nil || len(f.sharer.loadPool()) != 0 {
		t.Fatalf("reset pool: %v", err)
	}
}

func TestShareLeavesStateAlone(t *testing.T) {
	signedOut := aliceActive(nil)
	signedOut[rendererKey] = map[string]any{"owner": nil, "inactive": map[string]any{}}
	legacy := aliceActive(nil)
	delete(legacy, rendererKey)
	for name, tc := range map[string]struct {
		doc     map[string]any
		target  Owner
		running bool
		want    Outcome
	}{
		"target is owner": {aliceActive(nil), alice, false, Unchanged},
		"signed out":      {signedOut, bob, false, Unchanged},
		"no isolation":    {legacy, bob, false, Unchanged},
		"app running":     {aliceActive(nil), bob, true, Running},
		"no identity":     {aliceActive(nil), Owner{AccountID: "acct-b"}, false, Unsupported},
		"unknown format":  {map[string]any{rendererKey: map[string]any{"owner": 3, "inactive": map[string]any{}}}, bob, false, Unsupported},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.write(tc.doc)
			before, _ := os.ReadFile(f.sharer.statePath())
			f.running = tc.running
			if got := f.share(tc.target); got != tc.want {
				t.Fatalf("outcome %q, want %q", got, tc.want)
			}
			after, _ := os.ReadFile(f.sharer.statePath())
			if string(before) != string(after) {
				t.Fatal("state file changed")
			}
			if backups, _ := filepath.Glob(filepath.Join(f.sharer.Dir, "backups", "*")); len(backups) != 0 {
				t.Fatal("backup made without a change")
			}
		})
	}
	t.Run("missing file", func(t *testing.T) {
		f := newFixture(t)
		if got := f.share(bob); got != Unchanged {
			t.Fatalf("outcome %q", got)
		}
		if _, err := os.Stat(f.sharer.statePath()); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("state file was created")
		}
	})
	t.Run("undetectable app", func(t *testing.T) {
		f := newFixture(t)
		f.write(aliceActive(nil))
		f.sharer.Running = nil
		if got := f.share(bob); got != Unsupported {
			t.Fatalf("outcome %q", got)
		}
	})
}

func TestSecondShareIsANoOp(t *testing.T) {
	f := newFixture(t)
	f.write(aliceActive(nil))
	f.share(bob)
	if got := f.share(bob); got != Unchanged {
		t.Fatalf("outcome %q", got)
	}
	if backups, _ := filepath.Glob(filepath.Join(f.sharer.Dir, "backups", "*")); len(backups) != 1 {
		t.Fatalf("backups = %d", len(backups))
	}
}

func TestShareRefusesWhenAppOpensBeforeWrite(t *testing.T) {
	f := newFixture(t)
	f.write(aliceActive(nil))
	before, _ := os.ReadFile(f.sharer.statePath())
	calls := 0
	f.sharer.Running = func() bool { calls++; return calls > 1 }
	if got := f.share(bob); got != Running {
		t.Fatalf("outcome %q", got)
	}
	if after, _ := os.ReadFile(f.sharer.statePath()); string(after) != string(before) {
		t.Fatal("state written while the app was open")
	}
}

func TestRestoreLatestUndoesShare(t *testing.T) {
	f := newFixture(t)
	if err := f.sharer.RestoreLatest(); !errors.Is(err, ErrNoBackup) {
		t.Fatalf("err = %v", err)
	}
	f.write(aliceActive(nil))
	original, _ := os.ReadFile(f.sharer.statePath())
	f.share(bob)
	f.running = true
	if err := f.sharer.RestoreLatest(); !errors.Is(err, ErrDesktopRunning) {
		t.Fatalf("err = %v", err)
	}
	f.running = false
	if err := f.sharer.RestoreLatest(); err != nil {
		t.Fatal(err)
	}
	if restored, _ := os.ReadFile(f.sharer.statePath()); string(restored) != string(original) {
		t.Fatal("restore did not bring back the original state")
	}
	if bak, _ := os.ReadFile(f.sharer.statePath() + ".bak"); string(bak) != string(original) {
		t.Fatal("restore did not mirror the .bak copy")
	}
	// The shared state was backed up, so restoring again undoes the restore.
	if err := f.sharer.RestoreLatest(); err != nil {
		t.Fatal(err)
	}
	partition(t, f.read(), bob)
}
