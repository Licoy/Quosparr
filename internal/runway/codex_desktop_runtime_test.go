package runway

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexdesktop"
	"github.com/licoy/quosparr/internal/codexruntime"
)

func TestCodexSwitchStopsDesktopBeforeInspectingDaemon(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	af.b.UserID = "target-user"
	calls := []string{}
	desktop := true
	official := codexruntime.Identity{UserID: "source-user", AccountID: af.a.AccountID}
	s, _ := newService(t, f, func(o *Options) {
		af.apply(f)(o)
		o.Preferences.ShareCodexDesktopProjects = true
		load := o.Sources.LoadIndex
		o.Sources.LoadIndex = func() (codex.AccountIndex, error) {
			idx, err := load()
			for i := range idx.Accounts {
				idx.Accounts[i].UserID = "target-user"
			}
			return idx, err
		}
		commit := func() (codex.SwitchResult, error) {
			calls = append(calls, "write")
			auth, _ := af.st.LoadCredential(af.b.ID)
			official = runtimeTarget(af.b)
			return codex.SwitchResult{Auth: auth, Account: af.b}, af.st.SetActiveAccount(af.b.ID, t0)
		}
		o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) { return commit() }
		o.Sources.PrepareSwitch = func(context.Context, string) (func() (codex.SwitchResult, error), error) { return commit, nil }
		o.Sources.RuntimeIdentity = func() (codexruntime.Identity, error) { return official, nil }
		o.Sources.LegacyStop = func() (func() error, error) {
			calls = append(calls, "quit-desktop")
			desktop = false
			return func() error { calls = append(calls, "open-desktop"); desktop = true; return nil }, nil
		}
		o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
			calls = append(calls, "inspect")
			return codexruntime.Snapshot{Status: codexruntime.Running, Version: "after-quit", Identity: runtimeTarget(af.b)}, nil
		}
		o.Sources.RuntimeStop = func(_ context.Context, snapshot codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			calls = append(calls, "stop-daemon")
			if desktop || snapshot.Version != "after-quit" {
				t.Fatal("stale daemon snapshot or desktop still open")
			}
			return codexruntime.Snapshot{Status: codexruntime.NotRunning}, nil
		}
		o.Sources.RuntimeStart = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			calls = append(calls, "start-daemon")
			return codexruntime.Snapshot{Status: codexruntime.Running, Identity: runtimeTarget(af.b)}, nil
		}
		o.Sources.PrepareDesktopProjects = func(source, target codexdesktop.Owner) (func() (codexdesktop.Outcome, error), error) {
			calls = append(calls, "prepare-projects")
			if desktop || source.UserID != "source-user" || target.UserID != "target-user" {
				t.Fatal("incorrect project preflight")
			}
			return func() (codexdesktop.Outcome, error) { calls = append(calls, "share"); return codexdesktop.Shared, nil }, nil
		}
	})
	if err := s.SwitchAccountWithRestart(context.Background(), af.b.ID, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"quit-desktop", "inspect", "stop-daemon", "prepare-projects", "write", "share", "start-daemon", "open-desktop", "inspect"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v", calls)
	}
	if !desktop {
		t.Fatal("desktop not reopened")
	}
}

func TestRestoreDesktopProjectsChecksIdentityAndExcludesSwitch(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "restores", true: "changed_during_quit"}[changed], func(t *testing.T) {
			f := newFake(t)
			af := newAccountFixture(t, f)
			target := codexdesktop.Owner{UserID: "user", AccountID: "workspace"}
			owner := target
			applied := false
			var service *Service
			s, _ := newService(t, f, func(o *Options) {
				af.apply(f)(o)
				o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) {
					t.Fatal("restore changed credentials")
					return codex.SwitchResult{}, nil
				}
				o.Sources.RuntimeIdentity = func() (codexruntime.Identity, error) {
					return codexruntime.Identity{UserID: owner.UserID, AccountID: owner.AccountID}, nil
				}
				o.Sources.LegacyStop = func() (func() error, error) {
					if changed {
						owner.UserID = "other-user"
					}
					return func() error { return nil }, nil
				}
				o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
					return codexruntime.Snapshot{Status: codexruntime.NotRunning}, nil
				}
			})
			service = s
			_, err := s.RestoreDesktopProjects(context.Background(), target, func() (codexdesktop.Outcome, error) {
				applied = true
				if err := service.SwitchAccount(context.Background(), af.b.ID); !errors.Is(err, ErrSwitchBusy) {
					t.Fatalf("concurrent switch=%v", err)
				}
				if _, err := service.DesktopProjectOwner(); !errors.Is(err, ErrSwitchBusy) {
					t.Fatalf("concurrent preview=%v", err)
				}
				return codexdesktop.Shared, nil
			})
			if changed {
				if applied || !errors.Is(err, ErrDesktopProjectTargetChanged) {
					t.Fatalf("applied=%v err=%v", applied, err)
				}
			} else if !applied || err != nil {
				t.Fatalf("applied=%v err=%v", applied, err)
			}
		})
	}
}

func TestCodexProjectPreflightFailureDoesNotWriteCredentials(t *testing.T) {
	for _, recoverFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovers", true: "recovery_fails"}[recoverFails], func(t *testing.T) {
			f := newFake(t)
			af := newAccountFixture(t, f)
			writes, opens := 0, 0
			s, _ := newService(t, f, func(o *Options) {
				af.apply(f)(o)
				o.Preferences.ShareCodexDesktopProjects = true
				load := o.Sources.LoadIndex
				o.Sources.LoadIndex = func() (codex.AccountIndex, error) {
					idx, err := load()
					for i := range idx.Accounts {
						idx.Accounts[i].UserID = "target-user"
					}
					return idx, err
				}
				o.Sources.RuntimeIdentity = func() (codexruntime.Identity, error) {
					return codexruntime.Identity{UserID: "source", AccountID: "workspace"}, nil
				}
				commit := func() (codex.SwitchResult, error) { writes++; return codex.SwitchResult{}, nil }
				o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) { return commit() }
				o.Sources.PrepareSwitch = func(context.Context, string) (func() (codex.SwitchResult, error), error) { return commit, nil }
				o.Sources.LegacyStop = func() (func() error, error) {
					return func() error {
						opens++
						if recoverFails {
							return errors.New("synthetic recovery failure")
						}
						return nil
					}, nil
				}
				o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
					return codexruntime.Snapshot{Status: codexruntime.NotRunning}, nil
				}
				o.Sources.PrepareDesktopProjects = func(codexdesktop.Owner, codexdesktop.Owner) (func() (codexdesktop.Outcome, error), error) {
					return nil, codexdesktop.ErrConflict
				}
			})
			if err := s.SwitchAccountWithRestart(context.Background(), af.b.ID, true); !errors.Is(err, ErrDesktopProjects) {
				t.Fatal(err)
			}
			status := s.Snapshot().Accounts.Runtime
			want := "projects_prepare_failed"
			if recoverFails {
				want = "recovery_failed"
			}
			if writes != 0 || opens != 1 || status.FilesUpdated || status.State != want {
				t.Fatalf("writes=%d opens=%d status=%+v", writes, opens, status)
			}
		})
	}
}

func TestRestoreDesktopProjectsRejectsUnverifiedRestart(t *testing.T) {
	f := newFake(t)
	target := codexdesktop.Owner{UserID: "user", AccountID: "workspace"}
	cached := codexruntime.Identity{UserID: target.UserID, AccountID: target.AccountID, Email: "synthetic@example.com"}
	s, _ := newService(t, f, func(o *Options) {
		o.Sources.RuntimeIdentity = func() (codexruntime.Identity, error) {
			return codexruntime.Identity{UserID: target.UserID, AccountID: target.AccountID}, nil
		}
		o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
			return codexruntime.Snapshot{Status: codexruntime.Running, Identity: cached}, nil
		}
		o.Sources.RuntimeStop = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			return codexruntime.Snapshot{Status: codexruntime.NotRunning}, nil
		}
		o.Sources.RuntimeStart = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			cached.UserID = "wrong-user"
			return codexruntime.Snapshot{Status: codexruntime.Running, Identity: cached}, nil
		}
	})
	outcome, err := s.RestoreDesktopProjects(context.Background(), target, func() (codexdesktop.Outcome, error) { return codexdesktop.Shared, nil })
	if outcome != codexdesktop.Shared || !errors.Is(err, ErrDesktopProjectRestart) {
		t.Fatalf("outcome=%s err=%v", outcome, err)
	}
}

func TestLiveDesktopSharingUnsupportedDoesNotReadState(t *testing.T) {
	// Missing paths would fail Prepare if unsupported platforms touched desktop data.
	src := NewLiveSources(LiveConfig{Store: tempStore(t), DesktopSharer: codexdesktop.Sharer{CodexHome: t.TempDir()}})
	apply, err := src.PrepareDesktopProjects(codexdesktop.Owner{UserID: "source", AccountID: "workspace"}, codexdesktop.Owner{UserID: "target", AccountID: "workspace"})
	if err != nil || apply == nil {
		t.Fatalf("prepare error=%v", err)
	}
	outcome, err := apply()
	if outcome != codexdesktop.Unsupported || err != nil {
		t.Fatalf("outcome=%s err=%v", outcome, err)
	}
}

func TestCodexWriteOnlySwitchSkipsRunningDesktopProjects(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	af.b.UserID = "target-user"
	writes := 0
	official := codexruntime.Identity{UserID: "source-user", AccountID: af.a.AccountID}
	s, _ := newService(t, f, func(o *Options) {
		af.apply(f)(o)
		o.Preferences.ShareCodexDesktopProjects = true
		load := o.Sources.LoadIndex
		o.Sources.LoadIndex = func() (codex.AccountIndex, error) {
			idx, err := load()
			for i := range idx.Accounts {
				idx.Accounts[i].UserID = "target-user"
			}
			return idx, err
		}
		o.Sources.RuntimeIdentity = func() (codexruntime.Identity, error) { return official, nil }
		o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) {
			writes++
			auth, _ := af.st.LoadCredential(af.b.ID)
			official = runtimeTarget(af.b)
			return codex.SwitchResult{Auth: auth, Account: af.b}, af.st.SetActiveAccount(af.b.ID, t0)
		}
		o.Sources.LegacyStop = func() (func() error, error) { t.Fatal("write-only switch quit desktop"); return nil, nil }
		o.Sources.RuntimeStop = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			t.Fatal("write-only switch stopped daemon")
			return codexruntime.Snapshot{}, nil
		}
		o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
			return codexruntime.Snapshot{Status: codexruntime.NotRunning}, nil
		}
		o.Sources.PrepareDesktopProjects = func(codexdesktop.Owner, codexdesktop.Owner) (func() (codexdesktop.Outcome, error), error) {
			return nil, codexdesktop.ErrDesktopRunning
		}
	})
	if err := s.SwitchAccountWithRestart(context.Background(), af.b.ID, false); err != nil {
		t.Fatal(err)
	}
	if status := s.Snapshot().Accounts.Runtime; writes != 1 || !status.FilesUpdated || status.ProjectShare != "projects_running" {
		t.Fatalf("writes=%d status=%+v", writes, status)
	}
}

func TestCodexQuitFailureRestoresAlreadyStoppedDesktop(t *testing.T) {
	for _, recoveryFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "restored", true: "recovery_failed"}[recoveryFails], func(t *testing.T) {
			f := newFake(t)
			af := newAccountFixture(t, f)
			opens := 0
			s, _ := newService(t, f, func(o *Options) {
				af.apply(f)(o)
				commit := func() (codex.SwitchResult, error) {
					t.Fatal("credentials written after quit failure")
					return codex.SwitchResult{}, nil
				}
				o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) { return commit() }
				o.Sources.PrepareSwitch = func(context.Context, string) (func() (codex.SwitchResult, error), error) { return commit, nil }
				o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
					t.Fatal("inspected daemon after quit failure")
					return codexruntime.Snapshot{}, nil
				}
				o.Sources.LegacyStop = func() (func() error, error) {
					return func() error {
						opens++
						if recoveryFails {
							return errors.New("synthetic reopen error")
						}
						return nil
					}, ErrCodexDesktopQuit
				}
			})
			if err := s.SwitchAccountWithRestart(context.Background(), af.b.ID, true); !errors.Is(err, ErrCodexRuntime) {
				t.Fatal(err)
			}
			want := "stop_failed"
			if recoveryFails {
				want = "recovery_failed"
			}
			if status := s.Snapshot().Accounts.Runtime; opens != 1 || status.FilesUpdated || status.State != want || status.FailureReason != "codex_desktop_quit" {
				t.Fatalf("opens=%d status=%+v", opens, status)
			}
		})
	}
}

func TestCodexSwitchSkipsUnsupportedDesktopProjects(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		share string
	}{
		{"not_partitioned", codexdesktop.ErrNotPartitioned, "projects_not_partitioned"},
		{"unknown_format", codexdesktop.ErrUnknownFormat, "projects_unsupported"},
		{"database", codexdesktop.ErrDatabase, "projects_unsupported"},
	} {
		for _, restart := range []bool{false, true} {
			t.Run(tc.name+"/"+map[bool]string{false: "write_only", true: "restart"}[restart], func(t *testing.T) {
				f := newFake(t)
				af := newAccountFixture(t, f)
				af.b.UserID = "target-user"
				auth, _ := af.st.LoadCredential(af.b.ID)
				calls := []string{}
				official := codexruntime.Identity{UserID: "source-user", AccountID: af.a.AccountID}
				current := codexruntime.Snapshot{Status: codexruntime.Running, Identity: runtimeTarget(af.a)}
				commit := func() (codex.SwitchResult, error) {
					calls = append(calls, "write")
					official, current.Identity = runtimeTarget(af.b), runtimeTarget(af.b)
					if err := af.st.SetActiveAccount(af.b.ID, t0); err != nil {
						return codex.SwitchResult{}, err
					}
					f.setAuth(auth, nil)
					return codex.SwitchResult{Auth: auth, Account: af.b}, nil
				}
				s, _ := newService(t, f, func(o *Options) {
					af.apply(f)(o)
					o.Preferences.ShareCodexDesktopProjects = true
					o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) { return commit() }
					o.Sources.PrepareSwitch = func(context.Context, string) (func() (codex.SwitchResult, error), error) { return commit, nil }
					o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) { return current, nil }
					o.Sources.RuntimeStop = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
						calls = append(calls, "stop")
						current.Status = codexruntime.NotRunning
						return current, nil
					}
					o.Sources.RuntimeStart = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
						calls = append(calls, "start")
						current.Status = codexruntime.Running
						return current, nil
					}
					o.Sources.RuntimeIdentity = func() (codexruntime.Identity, error) { return official, nil }
					load := o.Sources.LoadIndex
					o.Sources.LoadIndex = func() (codex.AccountIndex, error) {
						idx, err := load()
						for i := range idx.Accounts {
							idx.Accounts[i].UserID = "target-user"
						}
						return idx, err
					}
					o.Sources.PrepareDesktopProjects = func(codexdesktop.Owner, codexdesktop.Owner) (func() (codexdesktop.Outcome, error), error) {
						calls = append(calls, "prepare-share")
						return nil, tc.err
					}
				})
				if err := s.SwitchAccountWithRestart(context.Background(), af.b.ID, restart); err != nil {
					t.Fatal(err)
				}
				want := []string{"prepare-share", "write"}
				if restart {
					want = []string{"stop", "prepare-share", "write", "start"}
				}
				if !reflect.DeepEqual(calls, want) {
					t.Fatalf("calls %v want %v", calls, want)
				}
				settle(t, s)
				if status := s.Snapshot().Accounts.Runtime; !status.FilesUpdated || status.ProjectShare != tc.share {
					t.Fatalf("status=%+v", status)
				}
			})
		}
	}
}
