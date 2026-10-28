package runway

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexruntime"
)

func TestCodexRuntimeSwitchLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name            string
		restart         bool
		before          codexruntime.Status
		failure         string
		match           codexruntime.Match
		want            string
		updated, failed bool
		calls           []string
	}{
		{"write_only_matched", false, codexruntime.Running, "", codexruntime.Matched, "matched", true, false, []string{"write", "inspect"}},
		{"write_only_mismatched", false, codexruntime.Running, "", codexruntime.Mismatched, "mismatched", true, false, []string{"write", "inspect"}},
		{"write_only_check_failed", false, codexruntime.Running, "inspect", "", "inspect_failed", true, false, []string{"write", "inspect"}},
		{"restart_matched", true, codexruntime.Running, "", codexruntime.Matched, "matched", true, false, []string{"prepare", "inspect", "stop", "write", "start"}},
		{"restart_still_mismatched", true, codexruntime.Running, "", codexruntime.Mismatched, "mismatched", true, false, []string{"prepare", "inspect", "stop", "write", "start"}},
		{"stop_failed", true, codexruntime.Running, "stop", "", "stop_failed", false, true, []string{"prepare", "inspect", "stop"}},
		{"preflight_failed", true, codexruntime.Unknown, "inspect", "", "preflight_failed", false, true, []string{"prepare", "inspect"}},
		{"credential_invalid", true, codexruntime.Running, "prepare", "", "credential_failed", false, true, []string{"prepare"}},
		{"write_failed_restored", true, codexruntime.Running, "write", "", "write_failed_recovered", false, true, []string{"prepare", "inspect", "stop", "write", "start"}},
		{"rollback_failed", true, codexruntime.Running, "rollback", "", "rollback_failed", false, true, []string{"prepare", "inspect", "stop", "write", "start"}},
		{"write_and_recovery_failed", true, codexruntime.Running, "recovery", "", "recovery_failed", false, true, []string{"prepare", "inspect", "stop", "write", "start"}},
		{"start_failed_after_commit", true, codexruntime.Running, "start", "", "start_failed", true, false, []string{"prepare", "inspect", "stop", "write", "start"}},
		{"not_running_not_started", true, codexruntime.NotRunning, "", "", "not_running", true, false, []string{"prepare", "inspect", "write", "inspect"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t)
			af := newAccountFixture(t, f)
			auth, _ := af.st.LoadCredential(af.b.ID)
			calls := []string{}
			var service *Service
			record := func(stage string) {
				calls = append(calls, stage)
				if service == nil || !service.Snapshot().Accounts.Switching {
					t.Errorf("switch mutex released during %s", stage)
				}
			}
			current := codexruntime.Snapshot{Status: tc.before, Identity: runtimeTarget(af.a)}
			if tc.match == codexruntime.Matched {
				current.Identity = runtimeTarget(af.b)
			}
			failure := &codexruntime.Error{Stage: "connection", Reason: "timeout_or_cancelled"}
			commit := func() (codex.SwitchResult, error) {
				record("write")
				if tc.failure == "rollback" {
					return codex.SwitchResult{}, errors.Join(errors.New("synthetic file failure"), codex.ErrRollbackFailed)
				}
				if tc.failure == "write" || tc.failure == "recovery" {
					return codex.SwitchResult{}, errors.New("synthetic file failure")
				}
				if err := af.st.SetActiveAccount(af.b.ID, t0); err != nil {
					return codex.SwitchResult{}, err
				}
				f.setAuth(auth, nil)
				return codex.SwitchResult{Auth: auth, Account: af.b}, nil
			}
			s, _ := newService(t, f, func(o *Options) {
				af.apply(f)(o)
				o.Sources.SwitchAccount = func(context.Context, string) (codex.SwitchResult, error) { return commit() }
				o.Sources.PrepareSwitch = func(context.Context, string) (func() (codex.SwitchResult, error), error) {
					record("prepare")
					if tc.failure == "prepare" {
						return nil, codex.ErrInvalidCredential
					}
					return commit, nil
				}
				o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
					record("inspect")
					if tc.failure == "inspect" {
						return current, failure
					}
					return current, nil
				}
				o.Sources.RuntimeStop = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
					record("stop")
					if tc.failure == "stop" {
						return current, failure
					}
					current.Status = codexruntime.NotRunning
					return current, nil
				}
				o.Sources.RuntimeStart = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
					record("start")
					if tc.failure == "start" || tc.failure == "recovery" {
						return current, failure
					}
					current.Status = codexruntime.Running
					return current, nil
				}
			})
			service = s
			err := s.SwitchAccountWithRestart(context.Background(), af.b.ID, tc.restart)
			if (err != nil) != tc.failed {
				t.Fatalf("error=%v want failed=%v", err, tc.failed)
			}
			if !reflect.DeepEqual(calls, tc.calls) {
				t.Fatalf("calls %v want %v", calls, tc.calls)
			}
			settle(t, s)
			status := s.Snapshot().Accounts.Runtime
			if status.State != tc.want || status.FilesUpdated != tc.updated {
				t.Fatalf("status=%+v", status)
			}
			if tc.failure == "inspect" && status.FailureReason != "timeout" {
				t.Fatalf("safe detail lost: %+v", status)
			}
			if s.Snapshot().Accounts.Switching {
				t.Fatal("switch never released")
			}
		})
	}
}

func TestCodexRuntimeCheckRejectsChangedOfficialAndLateResult(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed_before_check", true: "changed_during_check"}[during], func(t *testing.T) {
			f := newFake(t)
			af := newAccountFixture(t, f)
			official := runtimeTarget(af.a)
			s, _ := newService(t, f, func(o *Options) {
				af.apply(f)(o)
				o.Sources.RuntimeIdentity = func() (codexruntime.Identity, error) { return official, nil }
				o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
					if during {
						official = runtimeTarget(af.b)
					}
					return codexruntime.Snapshot{Status: codexruntime.Running, Identity: runtimeTarget(af.a)}, nil
				}
			})
			s.ReloadAccounts()
			if !during {
				official = runtimeTarget(af.b)
			}
			if err := s.CheckCodexRuntime(context.Background()); err != nil {
				t.Fatal(err)
			}
			if status := s.Snapshot().Accounts.Runtime; status.State != "unverified" {
				t.Fatalf("stale identity verified: %+v", status)
			}
		})
	}
}

func TestCodexRuntimeVerificationInvalidatesOnExternalAccountChange(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	s, _ := newService(t, f, func(o *Options) {
		af.apply(f)(o)
		o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
			return codexruntime.Snapshot{Status: codexruntime.Running, Identity: runtimeTarget(af.a)}, nil
		}
	})
	s.ReloadAccounts()
	if err := s.CheckCodexRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Accounts.Runtime.State != "matched" {
		t.Fatal("initial check")
	}
	if err := af.st.SetActiveAccount(af.b.ID, t0); err != nil {
		t.Fatal(err)
	}
	s.ReloadAccounts()
	if s.Snapshot().Accounts.Runtime.State != "unverified" {
		t.Fatal("old verification survived account change")
	}
}

func TestCodexRuntimeFileBackedSwitchThenApplyCurrentAccount(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	writeOfficialAuth(t, af.st, f.auth)
	sw := &codex.Switcher{Store: af.st, Now: f.clk.Now}
	cached := runtimeTarget(af.a)
	status := codexruntime.Running
	stops, starts := 0, 0
	identity := func() (codexruntime.Identity, error) {
		auth, err := af.st.LoadOfficialAuth()
		if err != nil {
			return codexruntime.Identity{}, err
		}
		return runtimeTarget(codex.NewManagedAccount("", auth, 0, "", "", "", t0)), nil
	}
	s, _ := newService(t, f, func(o *Options) {
		af.apply(f)(o)
		o.Sources.LoadValidAuth = func(context.Context, bool, *codex.Auth) (codex.Auth, error) { return af.st.LoadOfficialAuth() }
		o.Sources.SwitchAccount = sw.SwitchTo
		o.Sources.PrepareSwitch = sw.PrepareSwitch
		o.Sources.RuntimeIdentity = identity
		o.Sources.RuntimeInspect = func(context.Context) (codexruntime.Snapshot, error) {
			return codexruntime.Snapshot{Status: status, Identity: cached}, nil
		}
		o.Sources.RuntimeStop = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			stops++
			status = codexruntime.NotRunning
			return codexruntime.Snapshot{Status: status}, nil
		}
		o.Sources.RuntimeStart = func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error) {
			starts++
			var err error
			cached, err = identity()
			status = codexruntime.Running
			return codexruntime.Snapshot{Status: status, Identity: cached}, err
		}
	})
	if err := s.SwitchAccount(context.Background(), af.b.ID); err != nil {
		t.Fatal(err)
	}
	settle(t, s)
	actual, err := identity()
	if err != nil {
		t.Fatal(err)
	}
	if !sameRuntimeIdentity(actual, runtimeTarget(af.b)) || !sameRuntimeIdentity(cached, runtimeTarget(af.a)) || stops != 0 || starts != 0 {
		t.Fatal("write-only switch touched runtime or failed to update file")
	}
	if got := s.Snapshot().Accounts.Runtime; got.State != "mismatched" || !got.FilesUpdated {
		t.Fatalf("after write: %+v", got)
	}
	if err := s.SwitchAccountWithRestart(context.Background(), af.b.ID, true); err != nil {
		t.Fatal(err)
	}
	settle(t, s)
	if !sameRuntimeIdentity(cached, runtimeTarget(af.b)) || stops != 1 || starts != 1 {
		t.Fatal("restart did not reload official target")
	}
	if got := s.Snapshot().Accounts.Runtime; got.State != "matched" || !got.FilesUpdated {
		t.Fatalf("after apply: %+v", got)
	}
}
