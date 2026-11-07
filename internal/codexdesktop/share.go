// Package codexdesktop shares the local projects of the ChatGPT desktop app
// (bundle com.openai.codex) between the accounts Quosparr switches.
//
// The app keeps its state in <CODEX_HOME>/.codex-global-state.json and an
// identical ".bak" copy it falls back to. Since it isolates accounts, the key
// "electron-account-renderer-state-v1" holds
//
//	{"owner": <key>|null, "inactive": {<key>: {"atoms": {...}, "globals": {...}}}}
//
// where a key is the hex SHA-256 of the JSON array ["<userId>","<accountId>"].
// The top level of the file holds the owner's values; when the app selects
// another account it stashes them under the owner's key and loads the
// target's stash. Share writes the owner's project keys into the target's
// stash while the app is closed, so its next launch shows the same projects.
//
// Only projectGlobals and projectAtoms are carried; remote hosts, queued
// follow-ups and per-account preferences stay isolated. The file is changed
// only after a backup in Dir/backups, and never while the app runs. Between
// two accounts that are both in the shared pool (Dir/shared-owners.json) the
// source values replace the target's, so a removed project disappears
// everywhere; an account entering the pool keeps its own entries as well.
package codexdesktop

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

const (
	// StateFileName is the desktop app's state file inside CODEX_HOME.
	StateFileName = ".codex-global-state.json"
	rendererKey   = "electron-account-renderer-state-v1"
	atomsKey      = "electron-persisted-atom-state"
	poolFileName  = "shared-owners.json"
	backupPrefix  = "global-state-"
	backupExt     = ".json"
	backupKeep    = 10
)

var (
	ErrDesktopRunning = errors.New("codexdesktop: the desktop app is running")
	ErrUnknownFormat  = errors.New("codexdesktop: unrecognized desktop state")
	ErrNoBackup       = errors.New("codexdesktop: no backup to restore")
	ErrUnavailable    = errors.New("codexdesktop: the desktop app cannot be detected")
	errStateChanged   = errors.New("codexdesktop: desktop state changed during the update")
)

// Outcome is what Share did; the UI localizes it.
type Outcome string

const (
	Shared      Outcome = "shared"
	Unchanged   Outcome = "unchanged"
	Running     Outcome = "running"
	Unsupported Outcome = "unsupported"
)

// projectGlobals are the account-scoped globals that describe local
// projects and the threads filed under them.
var projectGlobals = []string{
	"active-workspace-roots",
	"electron-saved-workspace-roots",
	"electron-workspace-root-labels",
	"local-projects",
	"app-server-project-id-by-legacy-project-id-by-host",
	"app-server-projects-migration-by-host",
	"app-server-pending-project-deletions-by-host",
	"selected-project",
	"project-writable-roots",
	"project-appearances",
	"project-files",
	"pinned-thread-ids",
	"app-server-migrated-pinned-thread-ids-by-host",
	"pinned-project-ids",
	"sidebar-project-thread-orders",
	"sidebar-thread-metadata",
	"thread-project-assignments",
	"thread-project-membership-host-ids",
	"thread-writable-roots",
	"thread-workspace-root-hints",
	"thread-projectless-output-directories",
	"projectless-thread-ids",
	"project-order",
}

// projectAtoms are the account-scoped renderer atoms that order projects.
var projectAtoms = []string{"unified-sidebar-project-order-v1"}

// Owner is the identity the desktop app partitions its state by.
type Owner struct{ UserID, AccountID string }

// Key is the app's partition key: hex SHA-256 of JSON [userId, accountId].
func (o Owner) Key() string {
	sum := sha256.Sum256(encode([]string{o.UserID, o.AccountID}))
	return hex.EncodeToString(sum[:])
}

// DefaultDir is ~/.quosparr/codex-desktop under home.
func DefaultDir(home string) string { return filepath.Join(platform.RunwayDir(home), "codex-desktop") }

// Sharer edits the desktop state of one CODEX_HOME.
type Sharer struct {
	CodexHome string
	// Dir holds the shared pool and the backups.
	Dir string
	// Running reports whether the desktop app is open; nil cannot tell, so
	// nothing is ever written.
	Running func() bool
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

func (s Sharer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Sharer) statePath() string { return filepath.Join(s.CodexHome, StateFileName) }
func (s Sharer) poolPath() string  { return filepath.Join(s.Dir, poolFileName) }

func (s Sharer) backups() platform.BackupPolicy {
	return platform.BackupPolicy{Dir: filepath.Join(s.Dir, "backups"), Prefix: backupPrefix, Ext: backupExt, Keep: backupKeep}
}

// Share carries the active owner's projects into target's partition. A
// missing file, an app without account isolation, or target already being
// the owner leave everything as it is.
func (s Sharer) Share(target Owner) (Outcome, error) {
	if target.UserID == "" || target.AccountID == "" || s.Running == nil {
		return Unsupported, nil
	}
	if s.Running() {
		return Running, nil
	}
	pool := s.loadPool()
	outcome, source := Unchanged, ""
	err := s.edit(func(old []byte) ([]byte, error) {
		if len(old) == 0 {
			return old, nil
		}
		out, owner, err := shareState(old, target.Key(), pool)
		if err != nil {
			return nil, err
		}
		source = owner
		if !bytes.Equal(out, old) {
			outcome = Shared
		}
		return out, nil
	})
	switch {
	case errors.Is(err, ErrDesktopRunning):
		return Running, nil
	case errors.Is(err, ErrUnknownFormat):
		return Unsupported, nil
	case err != nil:
		return Unchanged, err
	}
	if source != "" {
		pool[source], pool[target.Key()] = true, true
		if err := s.savePool(pool); err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}

// RestoreLatest puts back the state saved before the last change Quosparr
// made. The current state is backed up first, so a second restore undoes it.
func (s Sharer) RestoreLatest() error {
	if s.Running == nil {
		return ErrUnavailable
	}
	if s.Running() {
		return ErrDesktopRunning
	}
	path, err := s.latestBackup()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !json.Valid(data) {
		return ErrUnknownFormat
	}
	return s.edit(func([]byte) ([]byte, error) { return data, nil })
}

// ResetPool forgets which accounts already share, so the next switch merges
// again instead of replacing. It is called when sharing is turned on.
func (s Sharer) ResetPool() error {
	if err := os.Remove(s.poolPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// edit rewrites the state file after a backup, refusing at the last moment
// if the app was opened or the file changed, then mirrors the ".bak" copy.
func (s Sharer) edit(fn func(old []byte) ([]byte, error)) error {
	path := s.statePath()
	var read, written []byte
	err := platform.EditFileWithBackupChecked(path, s.backups(), s.now(), func(old []byte) ([]byte, error) {
		read = old
		out, err := fn(old)
		written = out
		return out, err
	}, func() error {
		if s.Running() {
			return ErrDesktopRunning
		}
		if now, err := os.ReadFile(path); err != nil || !bytes.Equal(now, read) {
			return errStateChanged
		}
		return nil
	})
	if err != nil || bytes.Equal(read, written) {
		return err
	}
	// The app reads the copy only when the main file is unreadable, and
	// rewrites both on its next save, so a stale copy is harmless.
	_ = platform.WriteFileAtomic(path+".bak", written, 0o644)
	return nil
}

func (s Sharer) latestBackup() (string, error) {
	p := s.backups()
	matches, err := filepath.Glob(filepath.Join(p.Dir, p.Prefix+"*"+p.Ext))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", ErrNoBackup
	}
	// Fixed-width UTC timestamps sort chronologically.
	latest := matches[0]
	for _, m := range matches[1:] {
		latest = max(latest, m)
	}
	return latest, nil
}

type poolFile struct {
	Owners []string `json:"owners"`
}

// loadPool reads the shared pool; an unreadable pool counts as empty, which
// only makes the next share merge instead of replace.
func (s Sharer) loadPool() map[string]bool {
	pool := map[string]bool{}
	data, err := os.ReadFile(s.poolPath())
	if err != nil {
		return pool
	}
	var f poolFile
	if json.Unmarshal(data, &f) == nil {
		for _, o := range f.Owners {
			pool[o] = true
		}
	}
	return pool
}

func (s Sharer) savePool(pool map[string]bool) error {
	f := poolFile{Owners: []string{}}
	for o := range pool {
		f.Owners = append(f.Owners, o)
	}
	if err := platform.EnsurePrivateDirectory(s.Dir); err != nil {
		return err
	}
	return platform.WriteFileAtomic(s.poolPath(), encode(f), 0o600)
}
