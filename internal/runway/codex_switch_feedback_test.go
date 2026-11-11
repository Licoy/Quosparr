package runway

import (
	"context"
	"errors"
	"testing"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexdesktop"
	"github.com/licoy/quosparr/internal/codexruntime"
)

func TestCodexRuntimeMissingBinaryIsNotPermissionFailure(t *testing.T) {
	for _, tc := range []struct{ stage, reason, wantStage, wantReason string }{
		{"discovery", "no_trusted_binary", "read_settings", "no_trusted_binary"},
		{"start", "no_trusted_binary", "command", "no_trusted_binary"},
		{"discovery", "invalid_home", "read_settings", "permission"},
		{"discovery", "untrusted_home", "read_settings", "permission"},
		{"connection", "untrusted_socket", "connect", "permission"},
	} {
		t.Run(tc.stage+"_"+tc.reason, func(t *testing.T) {
			stage, reason := runtimeFailureCodes(&codexruntime.Error{Stage: tc.stage, Reason: tc.reason})
			if stage != tc.wantStage || reason != tc.wantReason {
				t.Fatalf("codes=%q/%q want=%q/%q", stage, reason, tc.wantStage, tc.wantReason)
			}
		})
	}
}

func TestCodexDesktopOnlySwitchWithoutCLICommitsAndReopens(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	t.Setenv("PATH", t.TempDir())
	client := codexruntime.Client{CodexHome: t.TempDir()}
	quits, opens, writes := 0, 0, 0
	s, _ := newService(t, f, func(o *Options) {
		af.apply(f)(o)
		o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) {
			t.Fatal("restart bypassed the prepared credential transaction")
			return codex.SwitchResult{}, nil
		}
		o.Sources.PrepareSwitch = func(context.Context, string) (func() (codex.SwitchResult, error), error) {
			return func() (codex.SwitchResult, error) {
				writes++
				auth, err := af.st.LoadCredential(af.b.ID)
				if err != nil {
					return codex.SwitchResult{}, err
				}
				f.setAuth(auth, nil)
				return codex.SwitchResult{Account: af.b, Auth: auth}, af.st.SetActiveAccount(af.b.ID, t0)
			}, nil
		}
		o.Sources.LegacyStop = func() (func() error, error) {
			quits++
			return func() error { opens++; return nil }, nil
		}
		o.Sources.RuntimeInspect = client.Inspect
		o.Sources.RuntimeStop = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			t.Fatal("desktop-only switch stopped a daemon")
			return codexruntime.Snapshot{}, nil
		}
		o.Sources.RuntimeStart = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			t.Fatal("desktop-only switch started a daemon")
			return codexruntime.Snapshot{}, nil
		}
	})
	if err := s.SwitchAccountWithRestart(context.Background(), af.b.ID, true); err != nil {
		t.Fatal(err)
	}
	settle(t, s)
	view := s.Snapshot().Accounts
	if quits != 1 || opens != 1 || writes != 1 || !view.Runtime.FilesUpdated || view.Runtime.State != "legacy_unverified" || view.ActiveID != af.b.ID {
		t.Fatalf("quits=%d opens=%d writes=%d account=%q runtime=%+v", quits, opens, writes, view.ActiveID, view.Runtime)
	}
}

func TestCodexProjectSharingExcludesAPISourceAndTarget(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		sourceAPI, targetAPI bool
	}{
		{"oauth_to_api", false, true},
		{"api_to_oauth", true, false},
		{"api_to_api", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t)
			af := newAccountFixture(t, f)
			af.a.UserID, af.b.UserID = "source-user", "target-user"
			if tc.sourceAPI {
				af.a.AuthMode = codex.AuthModeAPIKey
			}
			if tc.targetAPI {
				af.b.AuthMode = codex.AuthModeAPIKey
			}
			official := runtimeTarget(af.a)
			writes := 0
			s, _ := newService(t, f, func(o *Options) {
				af.apply(f)(o)
				o.Preferences.ShareCodexDesktopProjects = true
				o.Sources.LoadIndex = func() (codex.AccountIndex, error) {
					idx, err := af.st.LoadIndex()
					for i := range idx.Accounts {
						if idx.Accounts[i].ID == af.a.ID {
							idx.Accounts[i] = af.a
						} else {
							idx.Accounts[i] = af.b
						}
					}
					return idx, err
				}
				o.Sources.RuntimeIdentity = func() (codexruntime.Identity, error) { return official, nil }
				o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
					return codexruntime.Snapshot{Status: codexruntime.NotRunning}, nil
				}
				o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) {
					writes++
					auth, err := af.st.LoadCredential(af.b.ID)
					if err != nil {
						return codex.SwitchResult{}, err
					}
					if tc.targetAPI {
						auth = codex.APIKeyAuth("synthetic-api-key")
					}
					official = runtimeTarget(af.b)
					f.setAuth(auth, nil)
					return codex.SwitchResult{Account: af.b, Auth: auth}, af.st.SetActiveAccount(af.b.ID, t0)
				}
				o.Sources.PrepareDesktopProjects = func(codexdesktop.Owner, codexdesktop.Owner) (func() (codexdesktop.Outcome, error), error) {
					t.Fatal("API switch reached the OAuth project writer")
					return nil, errors.New("unexpected project writer")
				}
			})
			if err := s.SwitchAccountWithRestart(context.Background(), af.b.ID, false); err != nil {
				t.Fatal(err)
			}
			settle(t, s)
			view := s.Snapshot().Accounts
			if writes != 1 || view.ActiveID != af.b.ID || !view.Runtime.FilesUpdated || view.Runtime.ProjectShare != "projects_unsupported" {
				t.Fatalf("writes=%d active=%q runtime=%+v", writes, view.ActiveID, view.Runtime)
			}
		})
	}
}
