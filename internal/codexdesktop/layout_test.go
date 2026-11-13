package codexdesktop

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

// ownerScoped is the layout without account partitions: local projects stay
// at the top level and only owner-specific values carry the owner key.
func (f *fixture) ownerScoped() map[string]any {
	d := f.globals("p1", "p2")
	d[contentOwnerKey] = alice.Key()
	d["electron-owner-atoms:"+alice.Key()] = map[string]any{"unified-sidebar-project-order-v1": []string{"codex:project:p1"}}
	d["electron-owner-value:electron-saved-workspace-roots:"+alice.Key()] = []string{f.roots["p1"]}
	d[atomsKey] = map[string]any{"thread-sort-key": "updated_at"}
	return d
}

func TestUnpartitionedStateIsReportedAndNeverWritten(t *testing.T) {
	f := newFixture(t)
	f.write(f.active(f.globals("p2")))
	f.share()
	f.write(f.ownerScoped())
	before, err := os.ReadFile(f.sharer.statePath())
	must(t, err)
	backups := len(f.backups())
	_, err = f.sharer.Sources()
	if !errors.Is(err, ErrNotPartitioned) {
		t.Fatalf("sources: %v", err)
	}
	if _, err := f.sharer.Prepare(Request{alice.Key(), bob}); !errors.Is(err, ErrNotPartitioned) {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := f.sharer.Share(bob); !errors.Is(err, ErrNotPartitioned) {
		t.Fatalf("share: %v", err)
	}
	if err := f.sharer.CheckState(); !errors.Is(err, ErrNotPartitioned) {
		t.Fatalf("check: %v", err)
	}
	if err := f.sharer.RestoreLatest(); !errors.Is(err, ErrNotPartitioned) {
		t.Fatalf("restore: %v", err)
	}
	after, err := os.ReadFile(f.sharer.statePath())
	must(t, err)
	if !bytes.Equal(before, after) || len(f.backups()) != backups {
		t.Fatal("unpartitioned state was written")
	}
}

func TestPartitionKeyStillWinsAndUnknownLayoutFails(t *testing.T) {
	f := newFixture(t)
	doc := f.active(nil)
	doc[contentOwnerKey] = alice.Key()
	f.write(doc)
	if f.share() != Shared || !hasProject(f.target(), "p1") {
		t.Fatal("partitioned state with an owner key was not shared")
	}
	doc = f.ownerScoped()
	delete(doc, contentOwnerKey)
	f.write(doc)
	if err := f.sharer.CheckState(); !errors.Is(err, ErrUnknownFormat) {
		t.Fatal(err)
	}
}
