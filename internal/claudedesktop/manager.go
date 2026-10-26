package claudedesktop

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/platform"
	"path/filepath"
)

var (
	ErrBusy             = errors.New("claude desktop: another operation is running")
	ErrRecoveryRequired = errors.New("claude desktop: unfinished transaction requires explicit recovery")
	ErrNotBound         = errors.New("claude desktop: profile has not been manually confirmed")
	ErrAPIAccount       = claude.ErrAPIAccount
	ErrUnsafePath       = errors.New("claude desktop: unsafe directory path")
	ErrProfileChanged   = errors.New("claude desktop: directory identity changed or destination exists")
	ErrCrossVolume      = errors.New("claude desktop: profile and official directory must share a volume")
	ErrPersistence      = errors.New("claude desktop: metadata persistence failed")
	ErrStop             = errors.New("claude desktop: could not confirm Desktop stopped")
	ErrLaunch           = errors.New("claude desktop: Desktop did not start")
	ErrMove             = errors.New("claude desktop: directory move failed")
	ErrRollback         = errors.New("claude desktop: profile restoration failed; recovery required")
)

// Profile records a user's explicit assertion, never an automatically verified login.
// Identity is obtained from the account library and is never serialized here.
type Profile struct {
	AccountID        string                    `json:"accountId"`
	Identity         claude.CredentialIdentity `json:"-"`
	ApplicationPath  string                    `json:"applicationPath"`
	ConfirmedAt      *time.Time                `json:"confirmedAt,omitempty"`
	Confirmation     string                    `json:"confirmation,omitempty"`
	Directory        directoryIdentity         `json:"directory"`
	Pending          bool                      `json:"-"`
	RecoveryRequired bool                      `json:"-"`
}

func (p Profile) IsBound() bool { return p.Confirmation == "manual" && p.ConfirmedAt != nil }

type SwitchResult struct {
	CLISwitched, DesktopStarted, NeedsLoginConfirmation                      bool
	PreviousDesktopRestored, ManualReopenRequired, ManualCLIRecoveryRequired bool
}

type Manager struct {
	root    string
	host    Host
	account func(context.Context, string) (claude.ManagedAccount, error)
	mu      sync.Mutex
	rename  func(string, string) error
	write   func(string, []byte) error
}

// NewManager is pure construction; all Desktop operations require explicit calls.
func NewManager(runwayHome string, host Host, account func(context.Context, string) (claude.ManagedAccount, error)) *Manager {
	return &Manager{root: filepath.Join(runwayHome, "claude-desktop"), host: host, account: account, rename: renameDirectory, write: writeMetadata}
}

func (m *Manager) run(ctx context.Context, fn func(context.Context) error) (err error) {
	if !m.mu.TryLock() {
		return ErrBusy
	}
	defer m.mu.Unlock()
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = m.ensureRoot(); err != nil {
		return err
	}
	lock, e := platform.AcquireLock(filepath.Join(m.root, "operation.lock"))
	if e != nil {
		return errors.Join(ErrBusy, e)
	}
	defer func() { err = errors.Join(err, lock.Release()) }()
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return fn(bounded)
}

func (m *Manager) lookup(ctx context.Context, id string) (claude.ManagedAccount, error) {
	if m.account == nil {
		return claude.ManagedAccount{}, errors.New("claude desktop: account lookup unavailable")
	}
	a, err := m.account(ctx, id)
	if err != nil {
		return a, err
	}
	if a.ID != id || id == "" {
		return a, errors.New("claude desktop: account not found")
	}
	if a.IsAPI() {
		return a, ErrAPIAccount
	}
	if a.Identity.AccountUUID == "" {
		return a, claude.ErrIdentityRequired
	}
	return a, nil
}

func (m *Manager) Status(ctx context.Context, id string) (p Profile, err error) {
	if !m.mu.TryLock() {
		return p, ErrBusy
	}
	defer m.mu.Unlock()
	if err = ctx.Err(); err != nil {
		return p, err
	}
	s, e := m.loadState()
	if e != nil {
		return p, e
	}
	p = s.Profiles[id]
	p.AccountID = id
	j, e := m.loadJournal()
	if e != nil {
		return p, e
	}
	p.Pending = j != nil && j.Kind == "binding" && j.Target == id
	p.RecoveryRequired = j != nil
	return p, nil
}
