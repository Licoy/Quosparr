package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexdesktop"
	"github.com/licoy/quosparr/internal/codexruntime"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestCodexProjectFailureSeparatesCommittedSwitchFromPreflight(t *testing.T) {
	for _, tc := range []struct {
		name, state, share string
		failure            error
		preflight, restart bool
	}{
		{"failed", "matched", "projects_failed", errors.New("synthetic-secret-path"), false, false},
		{"partial", "matched", "projects_partial", codexdesktop.ErrPartialWrite, false, false},
		{"preflight", "projects_prepare_failed", "projects_failed", codexdesktop.ErrConflict, true, false},
		{"restart_failed", "start_failed", "projects_failed", errors.New("synthetic-secret-path"), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, nil)
			if err := e.s.Load(); err != nil {
				t.Fatal(err)
			}
			e.s.SetLanguage(store.LanguageEnglish)
			u, l := newSettingsUI(e.s, SettingsOptions{}), l10n.New(l10n.English)
			source := codex.ManagedAccount{ID: "source", UserID: "source-user", AccountID: "source-workspace", Email: "source@example.invalid"}
			target := codex.ManagedAccount{ID: "target", UserID: "target-user", AccountID: "target-workspace", Email: "target@example.invalid"}
			auth := codex.Auth{AuthMode: "chatgpt", Tokens: codex.Tokens{AccessToken: "synthetic-token", AccountID: target.AccountID}}
			var mu sync.Mutex
			active, official := source.ID, source
			commit := func() (codex.SwitchResult, error) {
				mu.Lock()
				active, official = target.ID, target
				mu.Unlock()
				return codex.SwitchResult{Account: target, Auth: auth}, nil
			}
			inspect := func(context.Context) (codexruntime.Snapshot, error) {
				mu.Lock()
				defer mu.Unlock()
				return codexruntime.Snapshot{Status: codexruntime.Running, Identity: codexruntime.Identity{UserID: official.UserID, AccountID: official.AccountID, Email: official.Email}}, nil
			}
			prefs := quietLivePreferences()
			prefs.Language, prefs.ShareCodexDesktopProjects = store.LanguageEnglish, true
			svc := runway.New(runway.Options{Preferences: prefs, Sources: runway.Sources{
				LoadIndex: func() (codex.AccountIndex, error) {
					mu.Lock()
					defer mu.Unlock()
					return codex.AccountIndex{ActiveAccountID: active, Accounts: []codex.ManagedAccount{source, target}}, nil
				},
				LoadValidAuth: func(ctx context.Context, _ bool, _ *codex.Auth) (codex.Auth, error) {
					// Hold the unrelated quota refresh until cancellation so it cannot
					// replace the switch feedback before the assertion below.
					<-ctx.Done()
					return codex.Auth{}, ctx.Err()
				},
				SwitchAccount: func(context.Context, string) (codex.SwitchResult, error) { return commit() },
				PrepareSwitch: func(context.Context, string) (func() (codex.SwitchResult, error), error) { return commit, nil },
				RuntimeIdentity: func() (codexruntime.Identity, error) {
					snapshot, _ := inspect(context.Background())
					return snapshot.Identity, nil
				},
				RuntimeInspect: inspect,
				RuntimeStop: func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
					return codexruntime.Snapshot{Status: codexruntime.NotRunning}, nil
				},
				RuntimeStart: func(ctx context.Context, _ codexruntime.Snapshot) (codexruntime.Snapshot, error) {
					if tc.restart {
						return codexruntime.Snapshot{}, &codexruntime.Error{Stage: "start", Reason: "command_failed"}
					}
					return inspect(ctx)
				},
				PrepareDesktopProjects: func(codexdesktop.Owner, codexdesktop.Owner) (func() (codexdesktop.Outcome, error), error) {
					if tc.preflight {
						return nil, tc.failure
					}
					return func() (codexdesktop.Outcome, error) { return "", tc.failure }, nil
				},
			}})
			t.Cleanup(svc.Close)
			if err := svc.ReloadAccounts(); err != nil {
				t.Fatal(err)
			}
			rt := &liveApp{svc: svc}
			old := currentRuntime()
			setLive(rt)
			t.Cleanup(func() { setLive(old) })
			first, completed := rt.switchCodexResult(l, target.ID, true)
			if completed == tc.preflight {
				t.Fatalf("completed=%v preflight=%v message=%q", completed, tc.preflight, first)
			}
			view := svc.Snapshot()
			wantActive := target.ID
			if tc.preflight {
				wantActive = source.ID
			}
			if view.Accounts.ActiveID != wantActive || view.Accounts.Runtime.State != tc.state || view.Accounts.Runtime.ProjectShare != tc.share || view.Accounts.Runtime.FilesUpdated == tc.preflight {
				t.Fatalf("active=%q runtime=%+v", view.Accounts.ActiveID, view.Accounts.Runtime)
			}
			for i, message := range []string{first, view.Codex.LastError} {
				outcome := tc.share
				if tc.preflight && i == 1 {
					outcome = tc.state
				}
				if !strings.Contains(message, l.CodexRuntime(outcome)) || strings.Contains(message, l.Text(l10n.KeyAccountsSwitchFailed)) || strings.Contains(message, "synthetic-secret-path") {
					t.Fatalf("misleading switch feedback: %q", message)
				}
				if !tc.preflight && !strings.Contains(message, l.CodexRuntime("updated")) || tc.restart && !strings.Contains(message, l.CodexRuntime("start_failed")) {
					t.Fatalf("committed or restart outcome missing: %q", message)
				}
			}
			second, completed := rtSwitchCodexResult(l, target.ID, true)
			third := rt.panelActions(nil).Accounts.Switch(target.ID, true)
			if first != second || first != third || completed == tc.preflight {
				t.Fatalf("entrypoints differ: %q / %q / %q", first, second, third)
			}
			f := accountFormOf(u)
			f.mode, f.editID, f.alias = "switch", target.ID, "saved draft"
			u.accountEditAct(f, func() (string, bool) { return rtSwitchCodexResult(l, target.ID, true) })
			deadline := time.Now().Add(time.Second)
			for f.busy && time.Now().Before(deadline) {
				e.q.drain()
				time.Sleep(time.Millisecond)
			}
			if f.busy {
				t.Fatal("switch form did not finish")
			}
			if tc.preflight {
				if f.mode != "switch" || f.alias != "saved draft" || f.editorError != first {
					t.Fatalf("uncommitted switch discarded its draft: %+v", f)
				}
			} else if f.mode != "" || f.alias != "" || !u.noticeOpen || u.notice != first {
				t.Fatalf("committed switch stayed editable: mode=%q alias=%q notice=%q", f.mode, f.alias, u.notice)
			}
		})
	}
}

func TestCodexMissingBinaryFeedbackNeverClaimsPermissionDenied(t *testing.T) {
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		message := codexRuntimeMessage(l, runway.CodexRuntimeView{State: "preflight_failed", FailureStage: "read_settings", FailureReason: "no_trusted_binary"})
		if !strings.Contains(message, l.CodexRuntime("no_trusted_binary")) || strings.Contains(message, accountReasonText(l, "permission")) {
			t.Fatalf("%s missing-binary feedback=%q", language, message)
		}
	}
}

func TestCodexSwitchAPICapabilityUsesAuthMode(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	u, f := newSettingsUI(e.s, SettingsOptions{}), &accountForm{}
	for _, acc := range []codex.ManagedAccount{
		{ID: "plain-api", AuthMode: codex.AuthModeAPIKey},
		{ID: "relay-api", AuthMode: codex.AuthModeAPIKey, APIBaseURL: "https://api.example.invalid"},
		{ID: "oauth"},
	} {
		codexRowActions(u, f, nil, acc, rowPlace{}, "synthetic", false)[0].run()
		api := acc.AuthMode == codex.AuthModeAPIKey
		if f.switchAPI != api || f.switchShare != (codexDesktopShareSupported && !api) {
			t.Fatalf("account=%s api=%v share=%v", acc.ID, f.switchAPI, f.switchShare)
		}
		f.closeEditor()
		if f.switchAPI || f.switchShare {
			t.Fatal("closing switch retained the prior account capability")
		}
	}
}
