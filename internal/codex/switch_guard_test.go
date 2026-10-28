package codex

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

func TestPreparedSwitchRejectsExternalFileChanges(t *testing.T) {
	for _, which := range []string{"official", "credential", "config", "target_metadata", "cancel"} {
		t.Run(which, func(t *testing.T) {
			f := newSwitchFixture(t, false, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			commit, err := f.switcher(SwitchHooks{}).PrepareSwitch(ctx, f.target.ID)
			noErr(t, err)
			switch which {
			case "official":
				f.writeOfficial(t, fixtureAuth(t, "external", false))
			case "credential":
				noErr(t, f.store.SaveCredential(f.target.ID, fixtureAuth(t, "replacement", false)))
			case "config":
				noErr(t, os.WriteFile(f.store.ConfigPath(), []byte("model = \"external\"\n"), 0600))
			case "target_metadata":
				acc := f.target
				acc.APIBaseURL = "https://changed.example/v1"
				noErr(t, f.store.UpdateMetadata(acc))
			case "cancel":
				cancel()
			}
			before := snapshotAll(f, f.target.ID)
			_, err = commit()
			if which == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("err=%v", err)
				}
			} else if !errors.Is(err, ErrSwitchFilesChanged) {
				t.Fatalf("err=%v", err)
			}
			for path, data := range before {
				after, _ := os.ReadFile(path)
				if !bytes.Equal(data, after) {
					t.Fatal("rejected switch changed files")
				}
			}
		})
	}
}

func TestPreparedSwitchPreservesConcurrentPresentationAndCacheUpdates(t *testing.T) {
	f := newSwitchFixture(t, false, false)
	commit, err := f.switcher(SwitchHooks{}).PrepareSwitch(context.Background(), f.target.ID)
	noErr(t, err)
	idx, err := f.store.LoadIndex()
	noErr(t, err)
	target, _ := idx.Account(f.target.ID)
	target.Alias = "Concurrent alias"
	target.Note = "Concurrent note"
	noErr(t, f.store.UpdateMetadata(target))
	current, _ := idx.Account(f.current.ID)
	current.LastError = "Synthetic quota error"
	noErr(t, f.store.UpdateMetadata(current))
	_, err = commit()
	noErr(t, err)
	idx, err = f.store.LoadIndex()
	noErr(t, err)
	target, _ = idx.Account(f.target.ID)
	current, _ = idx.Account(f.current.ID)
	if target.Alias != "Concurrent alias" || target.Note != "Concurrent note" || current.LastError != "Synthetic quota error" {
		t.Fatal("concurrent metadata lost")
	}
}

func TestPreparedSwitchRejectsCredentialReplacedDuringValidation(t *testing.T) {
	f := newSwitchFixture(t, false, false)
	before := snapshotAll(f, f.target.ID)
	sw := f.switcher(SwitchHooks{Validate: func(context.Context, Auth) error {
		return f.store.SaveCredential(f.target.ID, fixtureAuth(t, "replacement", false))
	}})
	_, err := sw.PrepareSwitch(context.Background(), f.target.ID)
	if !errors.Is(err, ErrSwitchFilesChanged) {
		t.Fatalf("err=%v", err)
	}
	official, _ := os.ReadFile(f.store.OfficialAuthPath())
	if !bytes.Equal(official, before[f.store.OfficialAuthPath()]) {
		t.Fatal("validation race changed official login")
	}
}
