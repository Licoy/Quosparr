package claudedesktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/claude"
)

func (r *managerRig) cli(t *testing.T) (*claude.Module, string) {
	t.Helper()
	store := claude.NewStoreAt(filepath.Join(r.home, "cli-accounts"), filepath.Join(r.home, "cli-config"), r.home)
	expiry := strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10)
	data := []byte(`{"claudeAiOauth":{"accessToken":"fixture-access","refreshToken":"fixture-refresh","expiresAt":` + expiry + `,"subscriptionType":"pro"}}`)
	a, err := store.Upsert(data, claude.CredentialIdentity{AccountUUID: "target", OrganizationUUID: "organization"}, claude.UpsertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r.accounts[a.ID] = a
	return claude.NewModule(store), a.ID
}

func TestManagerCoordinatedCLIRollbackOnLaunchOrMetadataFailure(t *testing.T) {
	for _, kind := range []string{"launch", "metadata"} {
		t.Run(kind, func(t *testing.T) {
			r := newManagerRig(t)
			module, id := r.cli(t)
			r.bind(t, id)
			if kind == "launch" {
				r.host.onStart = func() error { r.host.onStart = nil; return errors.New("fake partial startup") }
			} else {
				failed := false
				r.m.write = func(path string, b []byte) error {
					if filepath.Base(path) == "profiles.json" && r.host.running && !failed {
						failed = true
						return errors.New("fake metadata write denied")
					}
					return writeMetadata(path, b)
				}
			}
			result, err := r.m.Switch(context.Background(), id, func(after func() error) error { return module.SwitchWithAfter(context.Background(), id, false, after) })
			if err == nil || result.CLISwitched || result.DesktopStarted || result.ManualCLIRecoveryRequired {
				t.Fatal("incorrect partial result", err)
			}
			if _, e := module.Store.LoadOfficial(context.Background()); !errors.Is(e, claude.ErrNotLoggedIn) {
				t.Fatal("CLI credentials did not roll back", e)
			}
			idx, e := module.Store.LoadIndex()
			if e != nil || idx.CurrentAccountID != "" {
				t.Fatal("CLI current marker did not roll back", e)
			}
			r.assertPayload(t, r.host.app.DataDir, "original")
			if j, e := r.m.loadJournal(); e != nil || j != nil {
				t.Fatal("successful rollback left journal", e)
			}
		})
	}
}

func TestManagerCLIErrorRestoresPreviousDesktop(t *testing.T) {
	r := newManagerRig(t)
	r.bind(t, "a")
	failure := errors.New("fake CLI switch denied")
	result, err := r.m.Switch(context.Background(), "a", func(func() error) error { return failure })
	if !errors.Is(err, failure) || !result.PreviousDesktopRestored || result.CLISwitched {
		t.Fatal(err)
	}
	r.assertPayload(t, r.host.app.DataDir, "original")
}

func TestManagerCoordinatedSuccessAndRollbackClassification(t *testing.T) {
	r := newManagerRig(t)
	r.bind(t, "a")
	result, err := r.m.Switch(context.Background(), "a", func(after func() error) error { return after() })
	if err != nil || !result.CLISwitched || !result.DesktopStarted || !result.NeedsLoginConfirmation {
		t.Fatal(err)
	}
	r.assertPayload(t, r.host.app.DataDir, "a")
	if _, err = r.m.BeginBinding(context.Background(), "b", ""); err != nil {
		t.Fatal(err)
	}
	r.payload(t, "b")
	if _, err = r.m.ConfirmBinding(context.Background(), "b"); err != nil {
		t.Fatal(err)
	}
	r.assertPayload(t, r.host.app.DataDir, "a")
}

func TestManagerRetainsJournalWhenRollbackCannotStop(t *testing.T) {
	r := newManagerRig(t)
	r.bind(t, "a")
	r.host.onStart = func() error {
		r.host.stopErr = errors.New("fake running helper refuses stop")
		return errors.New("fake startup failure")
	}
	_, err := r.m.Open(context.Background(), "a")
	if !errors.Is(err, ErrRollback) || !errors.Is(err, ErrStop) {
		t.Fatal(err)
	}
	p, e := r.m.Status(context.Background(), "a")
	if e != nil || !p.RecoveryRequired {
		t.Fatal(e)
	}
	r.assertPayload(t, r.host.app.DataDir, "a")
	r.assertPayload(t, r.m.slot(""), "original")
	r.host.stopErr = nil
	r.host.onStart = nil
	if _, e = r.m.Recover(context.Background()); e != nil {
		t.Fatal(e)
	}
	r.assertPayload(t, r.host.app.DataDir, "original")
}

func TestManagerRecoveryReportsPossibleCLISwitch(t *testing.T) {
	r := newManagerRig(t)
	r.bind(t, "a")
	err := r.m.run(context.Background(), func(ctx context.Context) error {
		_, j, e := r.m.prepare(ctx, profileRequest{id: "a", kind: "switch"})
		if e != nil {
			return e
		}
		if e = r.m.activate(j); e != nil {
			return e
		}
		j.CLIWasApplied = true
		j.Phase = "calling-cli"
		return r.m.save("journal.json", j)
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := r.m.Recover(context.Background())
	if err != nil || !result.ManualCLIRecoveryRequired || result.CLISwitched {
		t.Fatal(err)
	}
	r.assertPayload(t, r.host.app.DataDir, "original")
}

func TestManagerRecoveryRejectsUnknownDirectoryCollision(t *testing.T) {
	r := newManagerRig(t)
	if _, err := r.m.BeginBinding(context.Background(), "a", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(r.m.slot("a"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := r.m.Recover(context.Background()); !errors.Is(err, ErrProfileChanged) {
		t.Fatal(err)
	}
	if j, e := r.m.loadJournal(); e != nil || j == nil {
		t.Fatal("journal not retained", e)
	}
	r.assertPayload(t, r.m.slot(""), "original")
}

func TestManagerOldDesktopStartsAfterCLIRollback(t *testing.T) {
	r := newManagerRig(t)
	module, id := r.cli(t)
	r.bind(t, id)
	starts := 0
	r.host.onStart = func() error {
		starts++
		_, err := module.Store.LoadOfficial(context.Background())
		if starts == 1 {
			if err != nil {
				t.Fatal("target started before CLI install", err)
			}
			return errors.New("fake launch failed")
		}
		if !errors.Is(err, claude.ErrNotLoggedIn) {
			t.Fatal("previous Desktop restarted before CLI rollback", err)
		}
		return nil
	}
	_, err := r.m.Switch(context.Background(), id, func(after func() error) error { return module.SwitchWithAfter(context.Background(), id, false, after) })
	if !errors.Is(err, ErrLaunch) || starts != 2 {
		t.Fatal(err, starts)
	}
}

func TestManagerMalformedJournalCannotMoveDirectories(t *testing.T) {
	for _, kind := range []string{"kind", "phase", "target-identity", "previous", "state", "oversize", "unknown-field"} {
		t.Run(kind, func(t *testing.T) {
			r := newManagerRig(t)
			if _, err := r.m.BeginBinding(context.Background(), "a", ""); err != nil {
				t.Fatal(err)
			}
			j, err := r.m.loadJournal()
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "kind":
				j.Kind = "unknown"
			case "phase":
				j.Phase = "unknown"
			case "target-identity":
				j.TargetIdentity = directoryIdentity{}
			case "previous":
				j.Previous = "untracked-account"
			case "state":
				j.Application.Executable = "other-executable"
			}
			if err = r.m.save("journal.json", j); err != nil {
				t.Fatal(err)
			}
			if kind == "oversize" {
				if err = os.WriteFile(filepath.Join(r.m.root, "journal.json"), make([]byte, (4<<20)+1), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "unknown-field" {
				if err = os.WriteFile(filepath.Join(r.m.root, "journal.json"), []byte(`{"Version":1,"secret":true}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			stops := r.host.stops
			if _, err = r.m.Recover(context.Background()); !errors.Is(err, ErrPersistence) {
				t.Fatal(err)
			}
			if r.host.stops != stops {
				t.Fatal("invalid journal affected Desktop")
			}
			r.assertPayload(t, r.m.slot(""), "original")
		})
	}
}

func TestManagerRechecksMovedIdentityBeforePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture symlink creation requires Windows privileges")
	}
	r := newManagerRig(t)
	outside := filepath.Join(r.home, "external-fixture")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	r.m.rename = func(from, to string) error {
		calls++
		if calls == 2 {
			if err := os.Rename(from, from+"-replaced"); err != nil {
				return err
			}
			if err := os.Symlink(outside, from); err != nil {
				return err
			}
		}
		return renameDirectory(from, to)
	}
	if _, err := r.m.BeginBinding(context.Background(), "a", ""); !errors.Is(err, ErrRollback) {
		t.Fatal(err)
	}
	info, err := os.Stat(outside)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatal("permissions followed unexpected link", err)
	}
	r.assertPayload(t, r.m.slot(""), "original")
	if r.host.starts != 0 {
		t.Fatal("started after an identity mismatch")
	}
}

func TestManagerCLIRollbackFailureKeepsJournalAndDoesNotRestartPrevious(t *testing.T) {
	r := newManagerRig(t)
	module, id := r.cli(t)
	r.bind(t, id)
	official := desktopRollbackFailStore{FileStore: claude.FileStore{Path: claude.CredentialsFilePath(module.Store.ConfigDir())}}
	module.Store.Official = official
	starts := r.host.starts
	r.host.onStart = func() error { return errors.New("fake target startup failed") }
	result, err := r.m.Switch(context.Background(), id, func(after func() error) error { return module.SwitchWithAfter(context.Background(), id, false, after) })
	if !errors.Is(err, claude.ErrSwitchRollback) || !errors.Is(err, ErrRollback) || !result.ManualCLIRecoveryRequired || r.host.starts != starts+1 || r.host.running {
		t.Fatal("unknown CLI state restarted previous Desktop", err)
	}
	if j, e := r.m.loadJournal(); e != nil || j == nil {
		t.Fatal("recovery journal missing", e)
	}
	r.assertPayload(t, r.m.slot(""), "original")
	// Simulate explicit user reconciliation before the explicit recovery action.
	if e := official.FileStore.Remove(context.Background()); e != nil {
		t.Fatal(e)
	}
	r.host.onStart = nil
	result, err = r.m.Recover(context.Background())
	if err != nil || !result.ManualCLIRecoveryRequired {
		t.Fatal(err)
	}
	r.assertPayload(t, r.host.app.DataDir, "original")
}
