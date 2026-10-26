package claudedesktop

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/licoy/quosparr/internal/platform"
)

type directoryIdentity struct {
	Volume uint64 `json:"volume"`
	File   uint64 `json:"file"`
}

func (d directoryIdentity) empty() bool { return d == directoryIdentity{} }

type desktopState struct {
	Version     int                `json:"version"`
	Application Application        `json:"application"`
	Active      string             `json:"active"`
	Native      directoryIdentity  `json:"native"`
	Profiles    map[string]Profile `json:"profiles"`
}
type desktopJournal struct {
	Version                          int         `json:"version"`
	Kind                             string      `json:"kind"`
	Application                      Application `json:"application"`
	Previous, Target                 string
	PreviousIdentity, TargetIdentity directoryIdentity
	WasRunning                       bool
	Phase                            string
	Confirm                          bool
	CLIWasApplied                    bool
	Same                             bool
}

func (m *Manager) slot(id string) string {
	key := "original"
	if id != "" {
		sum := sha256.Sum256([]byte(id))
		key = hex.EncodeToString(sum[:])
	}
	return filepath.Join(m.root, "slots", key)
}
func writeMetadata(path string, b []byte) error {
	if err := platform.WriteFileAtomic(path, b, 0o600); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
func (m *Manager) save(name string, value any) error {
	path := filepath.Join(m.root, name)
	if err := safeFile(path); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errors.Join(ErrPersistence, err)
	}
	if err = m.write(path, b); err != nil {
		return errors.Join(ErrPersistence, err)
	}
	return nil
}
func (m *Manager) read(name string, value any) (bool, error) {
	path := filepath.Join(m.root, name)
	if err := safeFile(path); err != nil {
		return false, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.Join(ErrPersistence, err)
	}
	const metadataLimit = 4 << 20
	b, readErr := io.ReadAll(io.LimitReader(f, metadataLimit+1))
	closeErr := f.Close()
	if err = errors.Join(readErr, closeErr); err != nil {
		return false, errors.Join(ErrPersistence, err)
	}
	if len(b) > metadataLimit {
		return false, ErrPersistence
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		return false, ErrPersistence
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return false, ErrPersistence
	}
	return true, nil
}
func (m *Manager) loadState() (desktopState, error) {
	var s desktopState
	had, err := m.read("profiles.json", &s)
	if err != nil {
		return s, err
	}
	if !had {
		return desktopState{Version: 1, Profiles: map[string]Profile{}}, nil
	}
	if s.Version != 1 || s.Profiles == nil {
		return s, ErrPersistence
	}
	if s.Application.Path == "" || s.Application.Executable == "" || s.Application.DataDir == "" {
		return s, ErrPersistence
	}
	for id, p := range s.Profiles {
		if id == "" || p.AccountID != id || p.Directory.File == 0 || p.ApplicationPath == "" {
			return s, ErrPersistence
		}
		if p.Confirmation != "" && p.Confirmation != "manual" {
			return s, ErrPersistence
		}
		if (p.Confirmation == "manual") != (p.ConfirmedAt != nil) {
			return s, ErrPersistence
		}
	}
	if s.Active != "" {
		p, ok := s.Profiles[s.Active]
		if !ok || !p.IsBound() || s.Native.File == 0 || p.Directory != s.Native {
			return s, ErrPersistence
		}
	}
	return s, nil
}

func (m *Manager) loadJournal() (*desktopJournal, error) {
	var j desktopJournal
	had, err := m.read("journal.json", &j)
	if err != nil || !had {
		return nil, err
	}
	if err := m.validateJournal(j); err != nil {
		return nil, err
	}
	return &j, nil
}
func (m *Manager) clearJournal() error {
	path := filepath.Join(m.root, "journal.json")
	if err := safeFile(path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrPersistence, err)
	}
	return syncDirectory(m.root)
}

func (m *Manager) validateJournal(j desktopJournal) error {
	if j.Version != 1 || j.Target == "" || j.TargetIdentity.File == 0 {
		return ErrPersistence
	}
	if (!j.PreviousIdentity.empty() && j.PreviousIdentity.File == 0) || (!j.Same && j.PreviousIdentity == j.TargetIdentity) {
		return ErrPersistence
	}
	if j.Previous == j.Target && !j.Same {
		return ErrPersistence
	}
	if j.Same && (j.Previous != j.Target || j.PreviousIdentity != j.TargetIdentity) {
		return ErrPersistence
	}
	switch j.Kind {
	case "binding", "switch", "open":
	default:
		return ErrPersistence
	}
	switch j.Phase {
	case "prepared", "activated", "awaiting-confirmation", "calling-cli", "starting", "restored":
	default:
		return ErrPersistence
	}
	if j.Phase == "awaiting-confirmation" && j.Kind != "binding" {
		return ErrPersistence
	}
	if j.Confirm && j.Kind != "binding" {
		return ErrPersistence
	}
	if j.CLIWasApplied && j.Kind != "switch" {
		return ErrPersistence
	}
	if !j.PreviousIdentity.empty() && j.PreviousIdentity.Volume != j.TargetIdentity.Volume {
		return ErrPersistence
	}
	s, err := m.loadState()
	if err != nil {
		return err
	}
	if s.Application != j.Application {
		return ErrPersistence
	}
	target, ok := s.Profiles[j.Target]
	if !ok || target.AccountID != j.Target || target.Directory != j.TargetIdentity || target.ApplicationPath != j.Application.Path {
		return ErrPersistence
	}
	if j.Previous != "" {
		if p, ok := s.Profiles[j.Previous]; !ok || p.AccountID != j.Previous {
			return ErrPersistence
		}
	}
	if s.Active != j.Previous && s.Active != j.Target {
		return ErrPersistence
	}
	if s.Native != j.PreviousIdentity && s.Native != j.TargetIdentity {
		return ErrPersistence
	}
	return nil
}
