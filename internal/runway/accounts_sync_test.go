package runway

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/codex"
)

func TestSyncOfficialPreservesConcurrentAccountChanges(t *testing.T) {
	for _, change := range []string{"save", "delete", "update"} {
		t.Run(change, func(t *testing.T) {
			f := newFake(t)
			st := tempStore(t)
			old, err := st.Upsert(f.auth, codex.UpsertOptions{Now: t0})
			if err != nil {
				t.Fatal(err)
			}
			writeOfficialAuth(t, st, f.auth)
			captured, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			s, _ := newService(t, f, func(o *Options) {
				o.Sources.LoadIndex = st.LoadIndex
				o.Sources.SyncOfficial = func(now time.Time) (codex.AccountIndex, error) {
					idx, _, err := st.SyncFromOfficialAuth(now)
					close(captured)
					<-release
					return idx, err
				}
			})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			go func() { defer close(done); s.syncOfficial() }()
			wait(t, captured)
			changeStoredAccount(t, st, old, change)
			if err := s.ReloadAccounts(); err != nil {
				t.Fatal(err)
			}
			before := s.Snapshot().Accounts.Sidebar
			close(release)
			wait(t, done)
			if after := s.Snapshot().Accounts.Sidebar; !reflect.DeepEqual(after, before) {
				t.Fatalf("late official sync replaced %s result: before=%d accounts, after=%d accounts", change, len(before), len(after))
			}
		})
	}
}

func TestReloadAccountsReturnsErrorAndPreservesList(t *testing.T) {
	f := newFake(t)
	af := newAccountFixture(t, f)
	loadErr := &os.PathError{Op: "open", Path: "/private/synthetic/account-index", Err: os.ErrPermission}
	broken := false
	s, _ := newService(t, f, func(o *Options) {
		o.Sources.LoadIndex = func() (codex.AccountIndex, error) {
			if broken {
				return codex.AccountIndex{}, loadErr
			}
			return af.st.LoadIndex()
		}
		o.Sources.SyncOfficial = func(time.Time) (codex.AccountIndex, error) {
			return codex.NewAccountIndex(), nil
		}
	})
	if err := s.ReloadAccounts(); err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot().Accounts.Sidebar
	broken = true
	if err := s.ReloadAccounts(); !errors.Is(err, loadErr) {
		t.Fatal("ReloadAccounts did not return the source failure")
	}
	s.syncOfficial()
	after := s.Snapshot()
	if !reflect.DeepEqual(after.Accounts.Sidebar, before) {
		t.Fatal("index read failure replaced the existing account list")
	}
	if after.Codex.LastError == "" || strings.Contains(after.Codex.LastError, loadErr.Path) {
		t.Fatal("index read failure was hidden or exposed a private path")
	}
}

func TestSyncOfficialIndexResultContract(t *testing.T) {
	for _, withLoader := range []bool{false, true} {
		for _, result := range []string{"success", "partial", "failure"} {
			name := result
			if withLoader {
				name += "_with_loader"
			}
			t.Run(name, func(t *testing.T) {
				f := newFake(t)
				idx := codex.NewAccountIndex()
				idx.Upsert(codex.NewManagedAccount("synced", f.auth, 0, "", "", "", t0))
				var syncErr error
				if result != "success" {
					syncErr = errors.New("synthetic official sync failure")
				}
				if result == "failure" {
					idx = codex.AccountIndex{}
				}
				loads := 0
				s, _ := newService(t, f, func(o *Options) {
					o.Sources.SyncOfficial = func(time.Time) (codex.AccountIndex, error) { return idx, syncErr }
					if withLoader {
						o.Sources.LoadIndex = func() (codex.AccountIndex, error) { loads++; return idx, nil }
					}
				})
				loads = 0
				s.syncOfficial()
				want := 1
				if result == "failure" {
					want = 0
				}
				if got := s.Snapshot().Accounts; len(got.Sidebar) != want || (syncErr != nil && got.Message == "") {
					t.Fatal("official sync lost the partial-index or error-message contract")
				}
				if withLoader && loads != want {
					t.Fatalf("index load count=%d, want %d", loads, want)
				}
			})
		}
	}
}

func changeStoredAccount(t *testing.T, st *codex.Store, old codex.ManagedAccount, change string) {
	t.Helper()
	var err error
	switch change {
	case "save":
		_, err = st.Upsert(testAuth(t, "new-account", "new@example.com", "synthetic-new"), codex.UpsertOptions{Now: t0})
	case "delete":
		err = st.DeleteAccount(old.ID)
	case "update":
		old.Alias = "updated alias"
		err = st.UpdateMetadata(old)
	}
	if err != nil {
		t.Fatal(err)
	}
}
