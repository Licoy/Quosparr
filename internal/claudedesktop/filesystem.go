package claudedesktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/licoy/quosparr/internal/platform"
)

// checkAncestors never enumerates profile contents or follows internal links.
func checkAncestors(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrUnsafePath
	}
	for p := path; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.Join(ErrUnsafePath, err)
		}
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return ErrUnsafePath
			}
			if _, e := directoryID(p); e != nil {
				return errors.Join(ErrUnsafePath, e)
			}
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}
func safeFile(path string) error {
	if err := checkAncestors(filepath.Dir(path)); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafePath
	}
	return platform.VerifyOwnerOnly(path, 0o600)
}
func (m *Manager) ensureRoot() error {
	if err := checkAncestors(m.root); err != nil {
		return err
	}
	for _, dir := range []string{filepath.Dir(m.root), m.root, filepath.Join(m.root, "slots")} {
		if err := platform.EnsurePrivateDirectory(dir); err != nil {
			return err
		}
	}
	return safeFile(filepath.Join(m.root, "operation.lock"))
}
func inside(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}
func (m *Manager) validateApplication(app Application) error {
	data := app.DataDir
	if app.Path == "" || app.Executable == "" || !filepath.IsAbs(data) || filepath.Clean(data) != data {
		return ErrUnsafePath
	}
	if inside(data, m.root) || inside(m.root, data) {
		return ErrUnsafePath
	}
	if filepath.Dir(filepath.Dir(data)) == filepath.Dir(data) {
		return ErrUnsafePath
	}
	if err := checkAncestors(data); err != nil {
		return err
	}
	parent, err := directoryID(filepath.Dir(data))
	if err != nil {
		return errors.Join(ErrUnsafePath, err)
	}
	slots, err := directoryID(filepath.Join(m.root, "slots"))
	if err != nil {
		return err
	}
	if parent.Volume != slots.Volume {
		return ErrCrossVolume
	}
	return nil
}
func optionalDirectory(path string) (directoryIdentity, error) {
	id, err := directoryID(path)
	if errors.Is(err, os.ErrNotExist) {
		return directoryIdentity{}, nil
	}
	return id, err
}
func (m *Manager) move(from, to string, want directoryIdentity) error {
	if err := checkAncestors(from); err != nil {
		return err
	}
	if err := checkAncestors(to); err != nil {
		return err
	}
	id, err := directoryID(from)
	if err != nil {
		return errors.Join(ErrMove, err)
	}
	if id != want || want.empty() {
		return ErrProfileChanged
	}
	if _, err = os.Lstat(to); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrProfileChanged, err)
	}
	parent, err := directoryID(filepath.Dir(to))
	if err != nil {
		return err
	}
	if parent.Volume != id.Volume {
		return ErrCrossVolume
	}
	if err = m.rename(from, to); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.Join(ErrProfileChanged, err)
		}
		if errors.Is(err, syscall.EXDEV) {
			return errors.Join(ErrCrossVolume, err)
		}
		return errors.Join(ErrMove, err)
	}
	moved, err := directoryID(to)
	if err != nil || moved != want {
		return errors.Join(ErrProfileChanged, err)
	}
	if err = platform.ProtectOwnerOnly(to, 0o700); err != nil {
		return errors.Join(ErrMove, err)
	}
	return nil
}
func (m *Manager) checkBound(s desktopState, id string) error {
	if !s.Profiles[id].IsBound() {
		return ErrNotBound
	}
	path := m.slot(id)
	want := s.Profiles[id].Directory
	if s.Active == id {
		path = s.Application.DataDir
		want = s.Native
	}
	got, err := optionalDirectory(path)
	if err != nil {
		return err
	}
	if got.empty() || (!want.empty() && got != want) {
		return ErrProfileChanged
	}
	return checkAncestors(path)
}
