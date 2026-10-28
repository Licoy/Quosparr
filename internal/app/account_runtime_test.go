package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexruntime"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestCodexRuntimeControlsVisibleAndWrap(t *testing.T) {
	for _, language := range l10n.Languages() {
		for _, width := range []int{380, 540} {
			for _, surface := range []string{"panel", "settings"} {
				t.Run(fmt.Sprintf("%s-%d-%s", language, width, surface), func(t *testing.T) {
					l := l10n.New(language)
					e := newEnv(t, nil)
					if err := e.s.Load(); err != nil {
						t.Fatal(err)
					}
					e.s.SetLanguage(store.Language(language))
					e.settle()
					snap := panelFixture()
					snap.Accounts.Runtime = runway.CodexRuntimeView{State: "mismatched", FilesUpdated: true}
					svc := runway.New(runway.Options{Preferences: e.s.Prefs()})
					t.Cleanup(svc.Close)
					rt := &liveApp{svc: svc}
					old := currentRuntime()
					setLive(rt)
					t.Cleanup(func() { setLive(old) })
					a := New(Options{Settings: e.s})
					u := newSettingsUI(e.s, SettingsOptions{})
					form := accountFormOf(u)
					checks, applies := 0, 0
					tt := ui.NewTester(func(c *ui.Context) {
						applyTheme(c)
						ui.Column(c).Fill().Padding(14).Gap(10).Children(func() {
							if surface == "panel" {
								a.codexAccountsPage(c, l, e.s.Prefs(), snap, PanelActions{Accounts: AccountActions{CheckRuntime: func() string { checks++; return "" }, Switch: func(string, bool) string { applies++; return "" }}})
							} else {
								// The same production control is rendered above settings account rows.
								codexRuntimeControls(c, l, snap.Accounts, &form.runtimeApply, false, func() { checks++ }, func() { applies++ })
								for _, acc := range snap.Accounts.Sidebar {
									renderCodexSettingsRow(c, u, form, rt, acc, rowPlace{}, snap.Accounts.ActiveID, false)
								}
							}
						})
					}, width, 640)
					for _, label := range []string{l.CodexRuntime("title"), l.CodexRuntime("check"), l.CodexRuntime("apply")} {
						box, ok := tt.Find(label)
						if !ok {
							t.Fatalf("missing %q", label)
						}
						if box.X < 0 || box.X+box.W > float32(width)+1 {
							t.Fatalf("control outside width: %q %+v", label, box)
						}
					}
					if err := tt.Click(l.CodexRuntime("apply")); err != nil {
						t.Fatal(err)
					}
					if !codexRestartSupported {
						if tt.HasText(l.CodexRuntime("restart_hint")) || applies != 0 {
							t.Fatal("unsupported restart opened confirmation or applied")
						}
						return
					}
					if !tt.HasText(l.CodexRuntime("restart_hint")) {
						t.Fatal("restart risk confirmation absent")
					}
					if applies != 0 {
						t.Fatal("applied without confirmation")
					}
					tt.Key(0, ui.KeyEscape)
					tt.Frame()
					if dir := os.Getenv("QUOSPARR_RUNTIME_QA_DIR"); dir != "" && language == l10n.SimplifiedChinese && width == 540 {
						if err := os.MkdirAll(dir, 0700); err != nil {
							t.Fatal(err)
						}
						exportReviewPNG(t, tt, filepath.Join(dir, "codex-runtime-"+surface+".png"))
					}
				})
			}
		}
	}
}

func TestCodexRuntimeMessageContainsSafeStageAndKeepsFileOutcome(t *testing.T) {
	l := l10n.New(l10n.English)
	msg := codexRuntimeMessage(l, runway.CodexRuntimeView{State: "start_failed", FilesUpdated: true, FailureStage: "connect", FailureReason: "timeout"})
	for _, text := range []string{l.CodexRuntime("updated"), accountReasonText(l, "connect"), accountReasonText(l, "timeout")} {
		if !strings.Contains(msg, text) {
			t.Fatalf("missing %q: %s", text, msg)
		}
	}
}

func TestCodexSwitchEntrypointsShareRuntimeOutcome(t *testing.T) {
	l := l10n.New(l10n.English)
	p := quietLivePreferences()
	p.Language = store.LanguageEnglish
	account := codex.ManagedAccount{ID: "synthetic", AuthMode: codex.AuthModeAPIKey, AccountID: "synthetic-key-fingerprint"}
	auth := codex.APIKeyAuth("synthetic-non-secret-key")
	switches, inspections := 0, 0
	svc := runway.New(runway.Options{Preferences: p, Sources: runway.Sources{
		LoadIndex: func() (codex.AccountIndex, error) {
			return codex.AccountIndex{ActiveAccountID: account.ID, Accounts: []codex.ManagedAccount{account}}, nil
		},
		LoadValidAuth: func(context.Context, bool, *codex.Auth) (codex.Auth, error) { return auth, nil },
		SwitchAccount: func(context.Context, string) (codex.SwitchResult, error) {
			switches++
			return codex.SwitchResult{Account: account, Auth: auth}, nil
		},
		RuntimeInspect: func(context.Context) (codexruntime.Snapshot, error) {
			inspections++
			return codexruntime.Snapshot{Status: codexruntime.Running, Identity: codexruntime.Identity{API: true}}, nil
		},
		RuntimeStop: func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			t.Fatal("write-only entry stopped daemon")
			return codexruntime.Snapshot{}, nil
		},
		RuntimeStart: func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			t.Fatal("write-only entry started daemon")
			return codexruntime.Snapshot{}, nil
		},
	}})
	t.Cleanup(svc.Close)
	rt := &liveApp{svc: svc}
	old := currentRuntime()
	setLive(rt)
	t.Cleanup(func() { setLive(old) })
	actions := rt.panelActions(nil)
	first, ok := rt.switchCodexResult(l, account.ID, false)
	if !ok {
		t.Fatal(first)
	}
	second, ok := rtSwitchCodexResult(l, account.ID, false)
	if !ok {
		t.Fatal(second)
	}
	third := actions.Accounts.Switch(account.ID, false)
	if first != second || first != third || switches != 3 || inspections != 3 || !strings.Contains(first, l.CodexRuntime("unverified")) || !strings.Contains(first, l.CodexRuntime("updated")) {
		t.Fatalf("outcomes differ: %q / %q / %q, switches=%d inspections=%d", first, second, third, switches, inspections)
	}
}

func TestCodexRuntimeRollbackFailureNeverClaimsUnchangedFiles(t *testing.T) {
	l := l10n.New(l10n.English)
	msg := codexRuntimeMessage(l, runway.CodexRuntimeView{State: "rollback_failed"})
	if !strings.Contains(msg, l.CodexRuntime("rollback_failed")) || strings.Contains(msg, l.CodexRuntime("not_updated")) || strings.Contains(msg, l.CodexRuntime("write_failed_recovered")) {
		t.Fatalf("misleading rollback outcome: %s", msg)
	}
}

func TestCodexRuntimeRestartCapabilityMatchesPlatform(t *testing.T) {
	if codexRestartSupported != (runtime.GOOS != "windows") {
		t.Fatalf("restart support on %s: %v", runtime.GOOS, codexRestartSupported)
	}
	if codexRestartDefault != (runtime.GOOS == "darwin") {
		t.Fatalf("restart default on %s: %v", runtime.GOOS, codexRestartDefault)
	}
}
