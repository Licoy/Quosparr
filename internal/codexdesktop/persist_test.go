package codexdesktop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/licoy/quosparr/internal/platform"
)

func TestPartialWritesRemainVisibleAndUndoable(t *testing.T) {
	for _, kind := range []string{"bak", "record", "main-readback"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.write(f.active(nil))
			oldBak := []byte(`{"original":true}`)
			must(t, os.WriteFile(f.sharer.statePath()+".bak", oldBak, 0o600))
			p, err := f.sharer.Prepare(Request{alice.Key(), bob})
			must(t, err)
			f.sharer.writeAtomic = func(path string, data []byte) error {
				if kind == "bak" && path == f.sharer.statePath()+".bak" {
					return errors.New("injected mirror failure")
				}
				if kind == "record" && path == f.sharer.writtenPath() {
					return errors.New("injected record failure")
				}
				err := platform.WriteFileAtomic(path, data, 0o600)
				if err == nil && kind == "main-readback" && path == f.sharer.statePath() {
					must(t, os.Remove(path))
				}
				return err
			}
			outcome, err := f.sharer.Apply(p)
			if !errors.Is(err, ErrPartialWrite) || outcome != Shared {
				t.Fatalf("%s %v", outcome, err)
			}
			backups := f.backups()
			if len(backups) != 1 {
				t.Fatal("backup missing")
			}
			if _, err := os.Stat(filepath.Join(backups[0], "undo.json")); err != nil {
				t.Fatal("applied undo evidence missing", err)
			}
			if kind == "bak" {
				data, err := os.ReadFile(f.sharer.statePath() + ".bak")
				must(t, err)
				if !bytes.Equal(data, oldBak) {
					t.Fatal("failed bak write changed original")
				}
			}
			f.sharer.writeAtomic = nil
			if kind != "main-readback" {
				must(t, f.sharer.RestoreLatest())
				if hasProject(f.target(), "p1") {
					t.Fatal("partial change not undone")
				}
			}
		})
	}
}
func TestRejectedPreparedAttemptDoesNotHidePreviousUndo(t *testing.T) {
	for _, kind := range []string{"running", "state"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			f.write(f.active(nil))
			f.share()
			doc := f.read()
			fresh := f.globals("p1", "p3")
			for key, v := range fresh {
				doc[key] = v
			}
			f.write(doc)
			p, err := f.sharer.Prepare(Request{alice.Key(), bob})
			must(t, err)
			calls := 0
			f.sharer.Running = func() bool {
				calls++
				if calls == 3 {
					if kind == "running" {
						return true
					}
					doc := f.read()
					doc["theme"] = "changed"
					f.write(doc)
				}
				return false
			}
			_, err = f.sharer.Apply(p)
			expected := ErrStateChanged
			if kind == "running" {
				expected = ErrDesktopRunning
			}
			if !errors.Is(err, expected) {
				t.Fatal(err)
			}
			prepared, _ := filepath.Glob(filepath.Join(f.sharer.Dir, "backups", "*", "prepared.json"))
			if len(prepared) != 1 {
				t.Fatal("prepared evidence missing")
			}
			f.sharer.Running = func() bool { return false }
			must(t, f.sharer.RestoreLatest())
			if hasProject(f.target(), "p1") {
				t.Fatal("prior successful share not undone")
			}
		})
	}
}
func TestCompletedBackupRetentionPreservesPartialEvidence(t *testing.T) {
	f := newFixture(t)
	f.write(f.active(nil))
	p, err := f.sharer.Prepare(Request{alice.Key(), bob})
	must(t, err)
	f.sharer.writeAtomic = func(path string, data []byte) error {
		if path == f.sharer.statePath()+".bak" {
			return errors.New("injected failure")
		}
		return platform.WriteFileAtomic(path, data, 0o600)
	}
	_, err = f.sharer.Apply(p)
	if !errors.Is(err, ErrPartialWrite) {
		t.Fatal(err)
	}
	partial := f.backups()[0]
	f.sharer.writeAtomic = nil
	must(t, f.sharer.RestoreLatest())
	for i := 0; i < 12; i++ {
		if i%2 == 0 {
			f.share()
		} else {
			must(t, f.sharer.RestoreLatest())
		}
	}
	complete, _ := filepath.Glob(filepath.Join(f.sharer.Dir, "backups", "*", "complete"))
	if len(complete) != 10 {
		t.Fatalf("completed backups=%d", len(complete))
	}
	if _, err := os.Stat(filepath.Join(partial, "undo.json")); err != nil {
		t.Fatal("partial evidence deleted", err)
	}
}
