package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/claudedesktop"
	"github.com/licoy/quosparr/internal/l10n"
)

type desktopUIHost struct {
	app                            claudedesktop.Application
	stopErr, startErr              error
	starts, stops                  int
	resolveEntered, resolveRelease chan struct{}
	startEntered, startRelease     chan struct{}
}

func (h *desktopUIHost) Resolve(ctx context.Context, _ string) (claudedesktop.Application, error) {
	if h.resolveEntered != nil {
		close(h.resolveEntered)
		<-h.resolveRelease
		h.resolveEntered = nil
	}
	return h.app, ctx.Err()
}
func (h *desktopUIHost) Stop(context.Context, claudedesktop.Application) (bool, error) {
	h.stops++
	return true, h.stopErr
}
func (h *desktopUIHost) Start(context.Context, claudedesktop.LaunchRequest) error {
	h.starts++
	if h.startEntered != nil {
		entered, release := h.startEntered, h.startRelease
		h.startEntered = nil
		close(entered)
		<-release
	}
	return h.startErr
}

func desktopUIFixture(t *testing.T) (*liveApp, *desktopUIHost, string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(home, "official", "Claude")
	if err = os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(data, "local-state"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(home, "desktop-fixture")
	if err = os.WriteFile(exe, []byte("never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	host := &desktopUIHost{app: claudedesktop.Application{Path: exe, Executable: exe, DataDir: data}}
	st := claude.NewStoreAt(filepath.Join(home, "accounts"), filepath.Join(home, "cli"), home)
	idx := claude.NewAccountIndex()
	id := claude.CredentialIdentity{AccountUUID: "fixture-account", OrganizationUUID: "fixture-org", Email: "private@example.invalid", OrganizationName: "Private Organization"}
	account := claude.ManagedAccount{ID: id.StableID(), Identity: id}
	idx.Accounts = append(idx.Accounts, account)
	b, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(st.IndexPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(st.IndexPath(), b, 0600); err != nil {
		t.Fatal(err)
	}
	rt := &liveApp{claudeMod: claude.NewModule(st)}
	rt.claudeDesktop = claudedesktop.NewManager(filepath.Join(home, "quosparr"), host, func(context.Context, string) (claude.ManagedAccount, error) { return account, nil })
	return rt, host, account.ID
}

func desktopDelivery(t *testing.T, ch <-chan func()) {
	t.Helper()
	select {
	case fn := <-ch:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("Desktop fixture did not deliver")
	}
}

func desktopUITester(t *testing.T, rt *liveApp, id string, privacy bool) (*ui.Tester, **desktopDialog, chan func()) {
	t.Helper()
	l := l10n.New(l10n.English)
	d := newDesktopDialog(rt, id, privacy)
	queue := make(chan func(), 8)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { renderDesktopDialog(c, l, &d, rt, func(fn func()) { queue <- fn }) })
	}, 760, 720)
	tt.Advance(time.Second)
	desktopDelivery(t, queue)
	tt.Frame()
	return tt, &d, queue
}

func TestDesktopBindingManualConfirmationAndCancel(t *testing.T) {
	rt, host, id := desktopUIFixture(t)
	tt, slot, queue := desktopUITester(t, rt, id, true)
	l := l10n.New(l10n.English)
	if tt.HasText("private@example.invalid") || tt.HasText("Private Organization") {
		t.Fatal("binding revealed private identity")
	}
	if !tt.HasText(tx(l, xDesktopUnbound)) {
		t.Fatalf("missing unbound state: %v", tt.Texts())
	}
	if err := tt.Click(tx(l, xDesktopStart)); err != nil {
		t.Fatal(err)
	}
	if host.starts != 0 {
		t.Fatal("binding started without explicit complete-profile consent")
	}
	if err := tt.Click(tx(l, xDesktopConsent)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(l, xDesktopStart)); err != nil {
		t.Fatal(err)
	}
	desktopDelivery(t, queue)
	tt.Frame()
	d := *slot
	if !d.profile.Pending || d.profile.IsBound() || !tt.HasText(tx(l, xDesktopPending)) {
		t.Fatal("pending binding was marked verified")
	}
	if err := tt.Click(tx(l, xDesktopFinish)); err != nil {
		t.Fatal(err)
	}
	if d.busy {
		t.Fatal("binding accepted without manual account assertion")
	}
	if err := os.WriteFile(filepath.Join(host.app.DataDir, "login-fixture"), []byte("user-confirmed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(l, xDesktopConfirmIdentity)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(l, xDesktopFinish)); err != nil {
		t.Fatal(err)
	}
	desktopDelivery(t, queue)
	tt.Frame()
	p, err := rt.claudeDesktop.Status(context.Background(), id)
	if err != nil || !p.IsBound() || p.Pending {
		t.Fatalf("manual binding not recorded: %+v %v", p, err)
	}
	if b, err := os.ReadFile(filepath.Join(host.app.DataDir, "local-state")); err != nil || string(b) != "original" {
		t.Fatal("binding did not restore original Desktop")
	}
	// Opening the bound Desktop changes no CLI account and uses no CLI credentials.
	r, err := rt.claudeDesktop.Open(context.Background(), id)
	if err != nil || r.CLISwitched || !r.DesktopStarted {
		t.Fatalf("Desktop-only open changed CLI: %+v %v", r, err)
	}
}

func TestDesktopDialogCancelRestoresAndLateResultIsIsolated(t *testing.T) {
	rt, host, id := desktopUIFixture(t)
	_, slot, queue := desktopUITester(t, rt, id, false)
	old := *slot
	host.resolveEntered, host.resolveRelease = make(chan struct{}), make(chan struct{})
	entered, release := host.resolveEntered, host.resolveRelease
	old.run("begin")
	<-entered
	old.close()
	fresh := newDesktopDialog(nil, "fresh", false)
	*slot = fresh
	close(release)
	desktopDelivery(t, queue)
	if *slot != fresh || fresh.busy || fresh.done || fresh.problem != "" {
		t.Fatal("late result changed replacement dialog")
	}
	if !old.done {
		t.Fatalf("clean cancellation was not completed: %v", old.failure)
	}
	p, err := rt.claudeDesktop.Status(context.Background(), id)
	if err != nil || p.IsBound() || p.RecoveryRequired {
		t.Fatalf("cancel left binding: %+v %v", p, err)
	}
}

func TestDesktopDialogClosePreservesRestoreFailure(t *testing.T) {
	rt, host, id := desktopUIFixture(t)
	tt, slot, queue := desktopUITester(t, rt, id, false)
	d := *slot
	d.run("begin")
	desktopDelivery(t, queue)
	tt.Frame()
	host.stopErr = errors.New("private-system-detail")
	d.close()
	desktopDelivery(t, queue)
	tt.Frame()
	if d.done || d.failure == nil || !d.profile.RecoveryRequired || !errors.Is(d.failure, claudedesktop.ErrRollback) {
		t.Fatal("close hid restoration failure")
	}
	if !tt.HasText(tx(l10n.New(l10n.English), xDesktopRecover)) || tt.HasText("private-system-detail") {
		t.Fatal("missing safe recovery action")
	}
	host.stopErr = nil
	d.run("recover")
	desktopDelivery(t, queue)
	tt.Frame()
	if d.failure != nil || d.profile.RecoveryRequired {
		t.Fatalf("explicit recovery failed: %v", d.failure)
	}
}

func TestDesktopResultLocalizationAndManualCLIRecovery(t *testing.T) {
	for _, lang := range l10n.Languages() {
		l := l10n.New(lang)
		r := claudedesktop.SwitchResult{ManualCLIRecoveryRequired: true, ManualReopenRequired: true, PreviousDesktopRestored: true}
		got := desktopResultText(l, r, errors.Join(claudedesktop.ErrRollback, errors.New("secret-detail")))
		if !strings.Contains(got, tx(l, xDesktopCLIRecovery)) || !strings.Contains(got, tx(l, xDesktopRecovery)) || strings.Contains(got, "secret-detail") {
			t.Fatalf("incomplete localized recovery for %s", lang)
		}
	}
	if harmlessDesktopCancellation(errors.Join(context.Canceled, errors.New("unexpected failure"))) || harmlessDesktopCancellation(errors.Join(context.Canceled, claudedesktop.ErrRollback)) || harmlessDesktopCancellation(errors.Join(context.Canceled, claudedesktop.ErrStop)) {
		t.Fatal("meaningful cancellation failures were hidden")
	}
}
