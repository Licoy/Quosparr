package claude

import (
	"bytes"
	"context"
	"errors"
	"github.com/licoy/quosparr/internal/platform"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestBrowserCommitWriteFailuresRestoreLibrary(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, failAt := range []int{1, 2} {
			t.Run(map[bool]string{false: "new", true: "existing"}[existing]+string(rune('0'+failAt)), func(t *testing.T) {
				r := newSyncRig(t)
				result := browserResult("new", "browser-new")
				if existing {
					if _, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("new", "browser-old")); err != nil {
						t.Fatal(err)
					}
				}
				idx, err := r.store.LoadIndex()
				if err != nil {
					t.Fatal(err)
				}
				beforeIndex, _, hadIndex, err := readOptional(r.store.IndexPath())
				if err != nil {
					t.Fatal(err)
				}
				path := r.store.CredentialPath(result.Identity.StableID())
				before, mode, had, err := readOptional(path)
				if err != nil {
					t.Fatal(err)
				}
				doc, _ := ParseAuthDocument(result.Credential)
				acc := browserAccount(idx, result.Identity.withCredential(doc), doc, r.now)
				idx.Upsert(acc)
				calls := 0
				err = r.store.commitBrowser(browserCommit{acc, idx, result.Credential}, func(path string, data []byte, perm os.FileMode) error {
					calls++
					if calls == failAt {
						return errors.New("injected failure")
					}
					return platform.WriteFileAtomic(path, data, perm)
				})
				if err == nil {
					t.Fatal("write failure accepted")
				}
				after, afterMode, has, e := readOptional(path)
				if e != nil || had != has || mode != afterMode || !bytes.Equal(before, after) {
					t.Fatal("credential not restored")
				}
				afterIndex, _, hasIndex, e := readOptional(r.store.IndexPath())
				if e != nil || hadIndex != hasIndex || !bytes.Equal(beforeIndex, afterIndex) {
					t.Fatal("index changed")
				}
				if !existing {
					if _, e := os.Stat(r.store.AccountDir(acc.ID)); !os.IsNotExist(e) {
						t.Fatal("new account directory survived rollback")
					}
				}
			})
		}
	}
}

func TestBrowserCommitRollbackFailureIsVisibleAndRedacted(t *testing.T) {
	r := newSyncRig(t)
	result := browserResult("new", "browser-new")
	acc, err := r.mod.ImportBrowserLogin(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := r.store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	err = r.store.commitBrowser(browserCommit{acc, idx, result.Credential}, func(path string, data []byte, perm os.FileMode) error {
		calls++
		if calls == 2 {
			if err := os.Remove(r.store.CredentialPath(acc.ID)); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(r.store.CredentialPath(acc.ID), 0700); err != nil {
				t.Fatal(err)
			}
			return errors.Join(errors.New("secret-in-error"), syscall.ENOSPC)
		}
		return platform.WriteFileAtomic(path, data, perm)
	})
	detail := requireOperation(t, err, StageRollbackCredential, KindRollbackFailed)
	if detail.Failure == nil || detail.Failure.Stage != StageWriteIndex || detail.Failure.Kind != KindDiskFull || detail.Rollback == nil || detail.Rollback.Stage != StageRollbackCredential {
		t.Fatal("original write and rollback summaries are missing")
	}
	if !errors.Is(err, syscall.ENOSPC) {
		t.Fatal("original disk-full cause lost")
	}
	if !errors.Is(err, ErrLoginRollback) || strings.Contains(err.Error(), "secret-in-error") {
		t.Fatalf("rollback error = %v", err)
	}
}
