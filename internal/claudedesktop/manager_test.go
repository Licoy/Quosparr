package claudedesktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/platform"
)

func TestManagerBindingAndTwoAccountRoundtrip(t *testing.T) {
	r := newManagerRig(t)
	r.bind(t, "a")
	r.bind(t, "b")
	for _, id := range []string{"a", "b", "b", "a"} {
		result, err := r.m.Open(context.Background(), id)
		if err != nil || !result.DesktopStarted || result.CLISwitched {
			t.Fatal("open", err)
		}
		r.assertPayload(t, r.host.app.DataDir, id)
	}
	r.assertPayload(t, r.m.slot(""), "original")
	r.assertPayload(t, r.m.slot("b"), "b")
	if runtime.GOOS != "windows" {
		for _, path := range []string{r.m.root, filepath.Join(r.m.root, "slots"), r.m.slot(""), r.m.slot("b"), r.host.app.DataDir} {
			if err := platform.VerifyOwnerOnly(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := platform.VerifyOwnerOnly(filepath.Join(r.m.root, "profiles.json"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestManagerCancelRetainsUnconfirmedProfileAndDoesNotNeedAccount(t *testing.T) {
	r := newManagerRig(t)
	if _, err := r.m.BeginBinding(context.Background(), "a", ""); err != nil {
		t.Fatal(err)
	}
	r.payload(t, "unfinished")
	delete(r.accounts, "a")
	p, err := r.m.Status(context.Background(), "a")
	if err != nil || !p.Pending || !p.RecoveryRequired {
		t.Fatal(err)
	}
	if err = r.m.CancelBinding(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	r.assertPayload(t, r.host.app.DataDir, "original")
	r.assertPayload(t, r.m.slot("a"), "unfinished")
	s, err := r.m.loadState()
	if err != nil || s.Profiles["a"].IsBound() {
		t.Fatal("cancel created bound record", err)
	}
}

func TestManagerRequiresConfirmationAndRejectsAPI(t *testing.T) {
	r := newManagerRig(t)
	applied := false
	if _, err := r.m.Switch(context.Background(), "a", func(f func() error) error { applied = true; return f() }); !errors.Is(err, ErrNotBound) || applied {
		t.Fatal(err)
	}
	if _, err := r.m.BeginBinding(context.Background(), "a", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.m.Open(context.Background(), "a"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal(err)
	}
	if err := r.m.CancelBinding(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	r.accounts["api"] = claude.ManagedAccount{ID: "api", Kind: claude.KindAPI, API: &claude.APIProfile{}}
	stops := r.host.stops
	if _, err := r.m.BeginBinding(context.Background(), "api", ""); !errors.Is(err, ErrAPIAccount) {
		t.Fatal(err)
	}
	if r.host.stops != stops {
		t.Fatal("API account stopped Desktop")
	}
}

func TestManagerStatusIsReadOnly(t *testing.T) {
	r := newManagerRig(t)
	if _, err := r.m.Status(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(r.m.root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("status created metadata directory", err)
	}
	if r.host.stops != 0 || r.host.starts != 0 {
		t.Fatal("status touched Desktop")
	}
}

func TestManagerStopAndLaunchFailures(t *testing.T) {
	r := newManagerRig(t)
	r.bind(t, "a")
	cliCalled := false
	r.host.stopErr = errors.New("fake stop denied")
	if _, err := r.m.Switch(context.Background(), "a", func(f func() error) error { cliCalled = true; return f() }); !errors.Is(err, ErrStop) || cliCalled {
		t.Fatal(err)
	}
	r.assertPayload(t, r.host.app.DataDir, "original")
	r.host.stopErr = nil
	r.host.onStart = func() error { r.host.onStart = nil; return errors.New("fake launch failed after spawning") }
	if _, err := r.m.Open(context.Background(), "a"); !errors.Is(err, ErrLaunch) {
		t.Fatal(err)
	}
	r.assertPayload(t, r.host.app.DataDir, "original")
	if !r.host.running {
		t.Fatal("previous Desktop not restored")
	}
}

func TestManagerRenameFailureRecoveryAndNoCopyFallback(t *testing.T) {
	for _, fail := range []error{errors.New("fake move denied"), syscall.EXDEV} {
		t.Run(fail.Error(), func(t *testing.T) {
			r := newManagerRig(t)
			calls := 0
			r.m.rename = func(a, b string) error {
				calls++
				if calls >= 2 {
					return fail
				}
				return renameDirectory(a, b)
			}
			if _, err := r.m.BeginBinding(context.Background(), "a", ""); err == nil || !errors.Is(err, ErrRollback) {
				t.Fatal(err)
			}
			j, err := r.m.loadJournal()
			if err != nil || j == nil {
				t.Fatal("journal missing", err)
			}
			r.m.rename = renameDirectory
			delete(r.accounts, "a")
			result, err := r.m.Recover(context.Background())
			if err != nil || !result.PreviousDesktopRestored {
				t.Fatal(err)
			}
			r.assertPayload(t, r.host.app.DataDir, "original")
		})
	}
}

func TestManagerRejectsSymlinkAndExternalReplacement(t *testing.T) {
	r := newManagerRig(t)
	r.bind(t, "a")
	old := r.m.slot("a")
	if err := os.Rename(old, old+"-external"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := r.m.Open(context.Background(), "a"); !errors.Is(err, ErrProfileChanged) {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		other := newManagerRig(t)
		if err := os.Rename(other.host.app.DataDir, other.host.app.DataDir+"-saved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(other.host.app.DataDir+"-saved", other.host.app.DataDir); err != nil {
			t.Fatal(err)
		}
		if _, err := other.m.BeginBinding(context.Background(), "a", ""); !errors.Is(err, ErrUnsafePath) {
			t.Fatal(err)
		}
	}
	attack := "../../outside"
	r.accounts[attack] = claude.ManagedAccount{ID: attack, Identity: claude.CredentialIdentity{AccountUUID: "attack"}}
	if strings.Contains(filepath.Base(r.m.slot(attack)), ".") {
		t.Fatal("raw account ID used in path")
	}
}

func TestManagerInterruptedActivationExplicitRecovery(t *testing.T) {
	r := newManagerRig(t)
	err := r.m.run(context.Background(), func(ctx context.Context) error {
		_, j, e := r.m.prepare(ctx, profileRequest{id: "a", kind: "binding"})
		if e != nil {
			return e
		}
		return r.m.activate(j)
	})
	if err != nil {
		t.Fatal(err)
	}
	recreated := NewManager(filepath.Join(r.home, "owned"), r.host, func(context.Context, string) (claude.ManagedAccount, error) {
		return claude.ManagedAccount{}, errors.New("deleted account")
	})
	p, err := recreated.Status(context.Background(), "a")
	if err != nil || !p.RecoveryRequired {
		t.Fatal(err)
	}
	r.assertPayload(t, r.m.slot(""), "original")
	if _, err = recreated.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.assertPayload(t, r.host.app.DataDir, "original")
}

func TestManagerBusyWhileBindingOperationRuns(t *testing.T) {
	r := newManagerRig(t)
	entered, release := make(chan struct{}), make(chan struct{})
	r.host.onStart = func() error { close(entered); <-release; return nil }
	done := make(chan error, 1)
	go func() { _, err := r.m.BeginBinding(context.Background(), "a", ""); done <- err }()
	<-entered
	if _, err := r.m.Status(context.Background(), "a"); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r.host.onStart = nil
	if err := r.m.CancelBinding(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
}
