package app

import (
	"context"
	"errors"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/claudedesktop"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDesktopSubscriptionEntriesAndAPIExclusion(t *testing.T) {
	rt, _, id := desktopUIFixture(t)
	rt.svc = runway.New(runway.Options{})
	defer rt.svc.Close()
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	u := newSettingsUI(e.s, SettingsOptions{})
	l := u.L()
	f := accountFormOf(u)
	f.kind = accountKindClaude
	for _, api := range []bool{false, true} {
		account := codex.ManagedAccount{ID: id, DisplayName: "Fixture"}
		if api {
			account.AuthMode = codex.AuthModeAPIKey
			account.APIBaseURL = "https://relay.invalid"
		}
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Children(func() { renderClaudeAccountRow(c, u, f, rt, account, rowPlace{}, id, false) })
		}, 760, 160)
		if _, found := tt.Find(tx(l, xDesktopBind)); found == api {
			t.Fatalf("Desktop settings eligibility: api=%v texts=%v", api, tt.Texts())
		}
		if _, found := tt.Find(tx(l, xAPIEdit)); found != api {
			t.Fatalf("API edit eligibility: api=%v texts=%v", api, tt.Texts())
		}
		app := &App{}
		panel := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Children(func() { app.claudeManagement(c, l, account, true, false, runway.AccountsView{}, AccountActions{}) })
		}, 760, 160)
		if panel.HasText(tx(l, xDesktopBind)) == api {
			t.Fatalf("Desktop panel eligibility: api=%v texts=%v", api, panel.Texts())
		}
	}
}

func TestDesktopSwitchDefaultsOffAndUnboundCannotEnable(t *testing.T) {
	rt, _, id := desktopUIFixture(t)
	l := l10n.New(l10n.English)
	on := false
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { desktopSwitchOption(c, l, rt, id, &on, false, func() {}) })
	}, 620, 320)
	if err := tt.Click(tx(l, xDesktopSwitch)); err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatal("unbound Desktop linked switching enabled")
	}
	if _, err := rt.claudeDesktop.BeginBinding(context.Background(), id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.claudeDesktop.ConfirmBinding(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if on {
		t.Fatal("binding changed opt-in default")
	}
	if err := tt.Click(tx(l, xDesktopSwitch)); err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("bound Desktop linked switching cannot be enabled")
	}
}

func TestDesktopDialogReviewArtifact(t *testing.T) {
	dir := os.Getenv("QUOSPARR_DESKTOP_REVIEW_DIR")
	if dir == "" {
		t.Skip("optional Tester screenshots")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	rt, _, id := desktopUIFixture(t)
	tt, slot, queue := desktopUITester(t, rt, id, false)
	exportReviewPNG(t, tt, filepath.Join(dir, "desktop-binding.png"))
	(*slot).run("begin")
	desktopDelivery(t, queue)
	tt.Frame()
	exportReviewPNG(t, tt, filepath.Join(dir, "desktop-pending.png"))
	(*slot).close()
	desktopDelivery(t, queue)
}

func TestDesktopIdentityReadIsAsyncAndLateReadCannotUnlockStart(t *testing.T) {
	rt, _, id := desktopUIFixture(t)
	d := newDesktopDialog(rt, id, false)
	queue := make(chan func(), 4)
	entered, release := make(chan struct{}), make(chan struct{})
	d.loadAccount = func(context.Context) (claude.AccountState, error) {
		close(entered)
		<-release
		return claude.AccountState{}, errors.New("private-read-error")
	}
	l := l10n.New(l10n.English)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { renderDesktopDialog(c, l, &d, rt, func(fn func()) { queue <- fn }) })
	}, 760, 720)
	<-entered
	if !d.identityLoading || d.identityReady {
		t.Fatal("identity loading did not disable start")
	}
	d.consent = true
	tt.Frame()
	if err := tt.Click(tx(l, xDesktopStart)); err != nil {
		t.Fatal(err)
	}
	if d.busy {
		t.Fatal("Desktop operation started before identity loaded")
	}
	old := d
	old.close()
	d = newDesktopDialog(nil, "replacement", false)
	close(release)
	desktopDelivery(t, queue)
	if d.identityReady || d.identityLoading || d.problem != "" {
		t.Fatal("obsolete identity read changed replacement dialog")
	}
}

func TestDesktopCloseRetainsOperationFailureAfterCleanup(t *testing.T) {
	rt, host, id := desktopUIFixture(t)
	_, slot, queue := desktopUITester(t, rt, id, false)
	d := *slot
	host.startEntered, host.startRelease = make(chan struct{}), make(chan struct{})
	entered, release := host.startEntered, host.startRelease
	host.startErr = errors.New("private-launch-error")
	d.run("begin")
	<-entered
	d.close()
	close(release)
	desktopDelivery(t, queue)
	if d.done || !errors.Is(d.failure, claudedesktop.ErrLaunch) {
		t.Fatalf("late launch failure lost after cleanup: %v", d.failure)
	}
	if d.outcome == nil {
		t.Fatal("failure lost operation result")
	}
	if harmlessDesktopCancellation(errors.Join(context.Canceled, claudedesktop.ErrLaunch)) {
		t.Fatal("launch failure treated as clean cancellation")
	}
	host.startErr = nil
	d.run("recover")
	desktopDelivery(t, queue)
}

func TestDesktopRecoveryAvailableWithoutAccountRow(t *testing.T) {
	rt, _, id := desktopUIFixture(t)
	if _, err := rt.claudeDesktop.BeginBinding(context.Background(), id, ""); err != nil {
		t.Fatal(err)
	}
	l := l10n.New(l10n.English)
	var d *desktopDialog
	queue := make(chan func(), 4)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			desktopRecoveryButton(c, l, &d, rt)
			renderDesktopDialog(c, l, &d, rt, func(fn func()) { queue <- fn })
		})
	}, 760, 720)
	if !tt.HasText(tx(l, xDesktopRecover)) {
		t.Fatal("missing account-independent recovery")
	}
	if err := tt.Click(tx(l, xDesktopRecover)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if d == nil || !d.profile.RecoveryRequired || d.identityLoading {
		t.Fatal("recovery depends on account identity")
	}
	if err := tt.Click(tx(l, xDesktopConsent)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(l, xDesktopRecover)); err != nil {
		t.Fatal(err)
	}
	desktopDelivery(t, queue)
	if d.failure != nil || d.profile.RecoveryRequired {
		t.Fatalf("recovery failed: %v", d.failure)
	}
}

func TestDesktopBindingDialogSevenLanguageLayout(t *testing.T) {
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		ctx, cancel := context.WithCancel(context.Background())
		d := &desktopDialog{name: "fixture@example.invalid / Fixture Organization", ctx: ctx, cancel: cancel, identityReady: true, activeBinding: true, profile: claudedesktop.Profile{Pending: true}}
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Children(func() { renderDesktopDialog(c, l, &d, nil, func(fn func()) { fn() }) })
		}, 760, 720)
		tt.Advance(100 * time.Millisecond)
		if !tt.HasText(tx(l, xDesktopWarning)) || !tt.HasText(tx(l, xDesktopConfirmIdentity)) {
			t.Fatalf("missing binding language %s", language)
		}
		bounds, ok := tt.Find(l.Text(l10n.KeyCancel))
		if !ok || bounds.Y < 0 || bounds.Y+bounds.H > 720 {
			t.Fatalf("dialog exceeds viewport for %s: %+v", language, bounds)
		}
		if dir := os.Getenv("QUOSPARR_DESKTOP_REVIEW_DIR"); dir != "" {
			exportReviewPNG(t, tt, filepath.Join(dir, "desktop-pending-"+string(language)+".png"))
		}
		cancel()
	}
}
