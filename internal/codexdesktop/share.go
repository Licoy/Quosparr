// Package codexdesktop copies validated local project metadata between explicit
// desktop account partitions, without editing SQLite, credentials or sessions.
//
// A state that names its content owner without the account partition keeps
// local projects at the top level for every account; each operation reports
// ErrNotPartitioned and never writes, and earlier undo records stay unused.
package codexdesktop

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

const (
	StateFileName   = ".codex-global-state.json"
	rendererKey     = "electron-account-renderer-state-v1"
	contentOwnerKey = "electron-renderer-content-owner"
	atomsKey        = "electron-persisted-atom-state"
	writtenFile     = "shared-state.json"
)

var (
	ErrDesktopRunning = errors.New("codexdesktop: the desktop app is running")
	ErrUnknownFormat  = errors.New("codexdesktop: unrecognized desktop state")
	ErrNotPartitioned = errors.New("codexdesktop: local projects are not partitioned by account")
	ErrNoBackup       = errors.New("codexdesktop: no backup to restore")
	ErrUnavailable    = errors.New("codexdesktop: the desktop app cannot be detected")
	ErrNoSource       = errors.New("codexdesktop: select a source account")
	ErrConflict       = errors.New("codexdesktop: conflicting local project metadata")
	ErrStateChanged   = errors.New("codexdesktop: desktop state changed during the update")
	ErrPartialWrite   = errors.New("codexdesktop: partial write; backup retained")
	ErrValidation     = errors.New("codexdesktop: local project metadata could not be validated")
	ErrDatabase       = errors.New("codexdesktop: local project database unavailable or unsupported")
	ErrIO             = errors.New("codexdesktop: cannot read or write desktop state")
)

type Outcome string

const (
	Shared          Outcome = "shared"
	SharedWithSkips Outcome = "shared_with_skips"
	Unchanged       Outcome = "unchanged"
	Running         Outcome = "running"
	Unsupported     Outcome = "unsupported"
)

type Owner struct{ UserID, AccountID string }

func (o Owner) Key() string {
	sum := sha256.Sum256(encode([]string{o.UserID, o.AccountID}))
	return hex.EncodeToString(sum[:])
}
func DefaultDir(home string) string { return filepath.Join(platform.RunwayDir(home), "codex-desktop") }

type Sharer struct {
	CodexHome   string
	Dir         string
	Running     func() bool
	Now         func() time.Time
	writeAtomic func(string, []byte) error
}
type Source struct {
	Key      string
	Projects int
	Threads  int
	Names    []string
}
type Request struct {
	SourceKey string
	Target    Owner
}
type Summary struct {
	Projects int
	Threads  int
	Skipped  int
	Reasons  []string
}

// Plan is an immutable preview; Apply revalidates its private contents.
type Plan struct {
	Summary                                Summary
	request                                Request
	sourcePrint, targetPrint, catalogPrint string
	selectedPrint                          string
	home, dir                              string
}

var editMu sync.Mutex

func (s Sharer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func (s Sharer) statePath() string   { return filepath.Join(s.CodexHome, StateFileName) }
func (s Sharer) writtenPath() string { return filepath.Join(s.Dir, writtenFile) }

// CheckState reads the state without writing, so an undo can be refused
// before the desktop app is asked to quit.
func (s Sharer) CheckState() error {
	_, err := s.readState()
	return err
}

func (s Sharer) Sources() ([]Source, error) {
	doc, err := s.readState()
	if err != nil {
		return nil, err
	}
	cat, err := s.catalog()
	if err != nil {
		return nil, err
	}
	out := []Source{}
	var invalid error
	for _, key := range doc.keys() {
		part, _, err := doc.partition(key)
		if err != nil {
			invalid = err
			continue
		}
		_, summary, names, err := validated(part, partition{object{}, object{}}, cat, s.CodexHome)
		if err != nil {
			invalid = err
			continue
		}
		if summary.Projects > 0 {
			out = append(out, Source{key, summary.Projects, summary.Threads, names})
			continue
		}
		for _, reason := range summary.Reasons {
			if reason != "cloud_project" && reason != "remote_project" {
				invalid = ErrValidation
			}
		}
	}
	if len(out) == 0 {
		if invalid != nil {
			return nil, invalid
		}
		return nil, ErrNoSource
	}
	return out, nil
}

func (s Sharer) Prepare(req Request) (*Plan, error) {
	if req.SourceKey == "" {
		return nil, ErrNoSource
	}
	if req.Target.UserID == "" || req.Target.AccountID == "" {
		return nil, ErrUnavailable
	}
	doc, err := s.readState()
	if err != nil {
		return nil, err
	}
	src, found, err := doc.partition(req.SourceKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNoSource
	}
	dst, _, err := doc.partition(req.Target.Key())
	if err != nil {
		return nil, err
	}
	cat, err := s.catalog()
	if err != nil {
		return nil, err
	}
	selected, summary, _, err := validated(src, dst, cat, s.CodexHome)
	if err != nil {
		return nil, err
	}
	if _, err := mergePartition(dst, selected); err != nil {
		return nil, err
	}
	return &Plan{summary, req, partitionPrint(src), partitionPrint(dst), digest(encode(cat)), partitionPrint(selected), s.CodexHome, s.Dir}, nil
}

func (s Sharer) Apply(plan *Plan) (Outcome, error) {
	editMu.Lock()
	defer editMu.Unlock()
	if plan == nil || plan.home != s.CodexHome || plan.dir != s.Dir {
		return Unchanged, ErrStateChanged
	}
	if err := s.stopped(); err != nil {
		return Unchanged, err
	}
	fresh, err := s.Prepare(plan.request)
	if err != nil {
		return Unchanged, err
	}
	if fresh.sourcePrint != plan.sourcePrint || fresh.targetPrint != plan.targetPrint || fresh.catalogPrint != plan.catalogPrint || fresh.selectedPrint != plan.selectedPrint {
		return Unchanged, ErrStateChanged
	}
	doc, err := s.readState()
	if err != nil {
		return Unchanged, err
	}
	src, _, err := doc.partition(plan.request.SourceKey)
	if err != nil {
		return Unchanged, err
	}
	dst, existed, err := doc.partition(plan.request.Target.Key())
	if err != nil {
		return Unchanged, err
	}
	if partitionPrint(src) != plan.sourcePrint || partitionPrint(dst) != plan.targetPrint {
		return Unchanged, ErrStateChanged
	}
	cat, err := s.catalog()
	if err != nil {
		return Unchanged, err
	}
	if digest(encode(cat)) != plan.catalogPrint {
		return Unchanged, ErrStateChanged
	}
	selected, _, _, err := validated(src, dst, cat, s.CodexHome)
	if err != nil {
		return Unchanged, err
	}
	next, err := mergePartition(dst, selected)
	if err != nil {
		return Unchanged, err
	}
	if sameJSON(encode(dst), encode(next)) {
		return Unchanged, nil
	}
	doc.setPartition(plan.request.Target.Key(), next)
	record := undoRecord{Target: plan.request.Target.Key(), Before: relatedValues(dst), After: relatedValues(next), Existed: existed}
	if err := s.commit(doc.original, encode(doc.root), record); err != nil {
		if errors.Is(err, ErrPartialWrite) {
			return Shared, err
		}
		return Unchanged, err
	}
	if fresh.Summary.Skipped > 0 {
		return SharedWithSkips, nil
	}
	return Shared, nil
}

func (s Sharer) ShareFrom(source, target Owner) (Outcome, error) {
	plan, err := s.Prepare(Request{SourceKey: source.Key(), Target: target})
	if err != nil {
		return Unchanged, err
	}
	return s.Apply(plan)
}

// Share uses only the explicit active owner, never an inferred inactive source.
func (s Sharer) Share(target Owner) (Outcome, error) {
	if s.Running == nil || target.UserID == "" || target.AccountID == "" {
		return Unsupported, nil
	}
	if s.Running() {
		return Running, nil
	}
	doc, err := s.readState()
	if err != nil {
		return Unchanged, err
	}
	if doc.owner == "" {
		return Unchanged, ErrNoSource
	}
	plan, err := s.Prepare(Request{SourceKey: doc.owner, Target: target})
	if err != nil {
		return Unchanged, err
	}
	outcome, err := s.Apply(plan)
	if errors.Is(err, ErrDesktopRunning) {
		return Running, nil
	}
	return outcome, err
}
func (s Sharer) stopped() error {
	if s.Running == nil {
		return ErrUnavailable
	}
	if s.Running() {
		return ErrDesktopRunning
	}
	return nil
}
