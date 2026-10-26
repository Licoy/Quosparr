package claudedesktop

import (
	"context"
	"errors"
	"github.com/licoy/quosparr/internal/claude"
	"os"
	"path/filepath"
	"testing"
)

type managerHost struct {
	app               Application
	running           bool
	stopErr, startErr error
	starts, stops     int
	onStart           func() error
}

func (h *managerHost) Resolve(context.Context, string) (Application, error) { return h.app, nil }
func (h *managerHost) Stop(context.Context, Application) (bool, error) {
	h.stops++
	was := h.running
	if h.stopErr != nil {
		return was, h.stopErr
	}
	h.running = false
	return was, nil
}
func (h *managerHost) Start(context.Context, LaunchRequest) error {
	h.starts++
	h.running = true
	if h.onStart != nil {
		if err := h.onStart(); err != nil {
			return err
		}
	}
	return h.startErr
}

type managerRig struct {
	m        *Manager
	host     *managerHost
	accounts map[string]claude.ManagedAccount
	home     string
}

func newManagerRig(t *testing.T) *managerRig {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(home, "native", "Claude")
	if err = os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	h := &managerHost{app: Application{Path: filepath.Join(home, "Claude.app"), Executable: filepath.Join(home, "Claude.app", "Claude"), DataDir: data}, running: true}
	r := &managerRig{host: h, home: home, accounts: map[string]claude.ManagedAccount{}}
	for _, id := range []string{"a", "b"} {
		r.accounts[id] = claude.ManagedAccount{ID: id, Identity: claude.CredentialIdentity{AccountUUID: id, OrganizationUUID: "org-" + id}}
	}
	r.m = NewManager(filepath.Join(home, "owned"), h, func(_ context.Context, id string) (claude.ManagedAccount, error) {
		a, ok := r.accounts[id]
		if !ok {
			return a, errors.New("fake account missing")
		}
		return a, nil
	})
	r.payload(t, "original")
	return r
}
func (r *managerRig) payload(t *testing.T, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.host.app.DataDir, "opaque-state"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
func (r *managerRig) assertPayload(t *testing.T, dir, want string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "opaque-state"))
	if err != nil || string(b) != want {
		t.Fatalf("opaque fixture mismatch, want %s: %v", want, err)
	}
}
func (r *managerRig) bind(t *testing.T, id string) {
	t.Helper()
	p, err := r.m.BeginBinding(context.Background(), id, "")
	if err != nil || !p.Pending || p.IsBound() {
		t.Fatal("begin", err)
	}
	r.payload(t, id)
	p, err = r.m.ConfirmBinding(context.Background(), id)
	if err != nil || !p.IsBound() || p.Pending || p.Confirmation != "manual" || p.Identity.AccountUUID != r.accounts[id].Identity.AccountUUID {
		t.Fatal("confirm", err)
	}
	r.assertPayload(t, r.host.app.DataDir, "original")
}

type desktopRollbackFailStore struct{ claude.FileStore }

func (s desktopRollbackFailStore) Remove(context.Context) error {
	return errors.New("fake CLI rollback removal failed")
}
