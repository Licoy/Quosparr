package codexdesktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/licoy/quosparr/internal/platform"
)

type undoRecord struct {
	Version int       `json:"version"`
	Target  string    `json:"target"`
	Before  partition `json:"before"`
	After   partition `json:"after"`
	Existed bool      `json:"existed"`
}
type fileSnapshot struct {
	Name   string
	Data   []byte
	Exists bool
}

// safePath rejects links at every existing component, including the leaf;
// desktop data and backup files must never redirect writes to another file.
func safePath(path string) error {
	if !filepath.IsAbs(path) {
		return ErrIO
	}
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ErrIO
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return ErrIO
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return nil
}
func readRegular(path string) ([]byte, bool, error) {
	if err := safePath(path); err != nil {
		return nil, false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, false, ErrIO
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, ErrIO
	}
	return data, true, nil
}
func writeChecked(path string, data []byte) error {
	if err := safePath(path); err != nil {
		return err
	}
	if err := platform.WriteFileAtomic(path, data, 0o600); err != nil {
		return ErrIO
	}
	actual, found, err := readRegular(path)
	if err != nil || !found || !bytes.Equal(actual, data) {
		return ErrIO
	}
	return nil
}
func (s Sharer) snapshots() ([]fileSnapshot, error) {
	files := []fileSnapshot{}
	for _, path := range []string{s.statePath(), s.statePath() + ".bak", s.writtenPath()} {
		data, exists, err := readRegular(path)
		if err != nil {
			return nil, err
		}
		files = append(files, fileSnapshot{path, data, exists})
	}
	return files, nil
}
func sameFiles(files []fileSnapshot) bool {
	for _, old := range files {
		data, exists, err := readRegular(old.Name)
		if err != nil || exists != old.Exists || !bytes.Equal(data, old.Data) {
			return false
		}
	}
	return true
}
func (s Sharer) backup(files []fileSnapshot, record undoRecord) (string, error) {
	root := filepath.Join(s.Dir, "backups")
	if err := safePath(root); err != nil {
		return "", err
	}
	if err := platform.EnsurePrivateDirectory(root); err != nil {
		return "", ErrIO
	}
	dir, err := os.MkdirTemp(root, s.now().UTC().Format("20060102T150405.000000000Z")+"-")
	if err != nil {
		return "", ErrIO
	}
	if err := platform.ProtectOwnerOnly(dir, 0o700); err != nil {
		return "", ErrIO
	}
	names := []string{"state.json", "state.bak", "shared-state.json"}
	manifest := map[string]bool{}
	for i, file := range files {
		manifest[names[i]] = file.Exists
		if file.Exists {
			if err := writeChecked(filepath.Join(dir, names[i]), file.Data); err != nil {
				return "", err
			}
		}
	}
	if err := writeChecked(filepath.Join(dir, "files.json"), encode(manifest)); err != nil {
		return "", err
	}
	if err := writeChecked(filepath.Join(dir, "prepared.json"), encode(record)); err != nil {
		return "", err
	}
	return dir, nil
}
func (s Sharer) commit(before, after []byte, record undoRecord) error {
	if err := s.stopped(); err != nil {
		return err
	}
	files, err := s.snapshots()
	if err != nil {
		return err
	}
	if !files[0].Exists || !bytes.Equal(files[0].Data, before) {
		return ErrStateChanged
	}
	record.Version = 1
	backup, err := s.backup(files, record)
	if err != nil {
		return err
	}
	if err := s.stopped(); err != nil {
		return err
	}
	if !sameFiles(files) {
		return ErrStateChanged
	}
	if err := safePath(s.statePath()); err != nil {
		return err
	}
	if err := s.write(s.statePath(), after); err != nil {
		return ErrIO
	}
	// Publication means the main file has changed, even if its readback or a
	// later mirror write fails; prepared attempts never authorize an undo.
	if err := os.Rename(filepath.Join(backup, "prepared.json"), filepath.Join(backup, "undo.json")); err != nil {
		return ErrPartialWrite
	}
	data, found, err := readRegular(s.statePath())
	if err != nil || !found || !bytes.Equal(data, after) {
		return ErrPartialWrite
	}
	if err := s.writeVerified(s.statePath()+".bak", after); err != nil {
		return ErrPartialWrite
	}
	if err := s.writeVerified(s.writtenPath(), encode(record)); err != nil {
		return ErrPartialWrite
	}
	if err := writeChecked(filepath.Join(backup, "complete"), []byte("1")); err != nil {
		return ErrPartialWrite
	}
	if err := s.pruneBackups(); err != nil {
		return ErrPartialWrite
	}
	return nil
}
func (s Sharer) write(path string, data []byte) error {
	if s.writeAtomic != nil {
		return s.writeAtomic(path, data)
	}
	return platform.WriteFileAtomic(path, data, 0o600)
}
func (s Sharer) writeVerified(path string, data []byte) error {
	if err := safePath(path); err != nil {
		return err
	}
	if err := s.write(path, data); err != nil {
		return ErrIO
	}
	actual, found, err := readRegular(path)
	if err != nil || !found || !bytes.Equal(actual, data) {
		return ErrIO
	}
	return nil
}
func (s Sharer) pruneBackups() error {
	// Only completed operations count toward retention; partial or rejected
	// attempts retain their evidence and are never promoted into undo records.
	matches, err := filepath.Glob(filepath.Join(s.Dir, "backups", "*", "complete"))
	if err != nil {
		return ErrIO
	}
	sort.Strings(matches)
	for len(matches) > 10 {
		dir := filepath.Dir(matches[0])
		if err := safePath(dir); err != nil {
			return err
		}
		if err := os.RemoveAll(dir); err != nil {
			return ErrIO
		}
		matches = matches[1:]
	}
	return nil
}

// RestoreLatest reverses only this operation's shared fields, identified by
// account key even if the application moved them between active and inactive.
// Any change to the target's shared fields rejects the undo; unrelated state
// and every other account remain intact.
func (s Sharer) RestoreLatest() error {
	editMu.Lock()
	defer editMu.Unlock()
	if err := s.stopped(); err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(s.Dir, "backups", "*", "undo.json"))
	if err != nil {
		return ErrIO
	}
	if len(matches) == 0 {
		return ErrNoBackup
	}
	sort.Strings(matches)
	data, found, err := readRegular(matches[len(matches)-1])
	if err != nil {
		return err
	}
	if !found {
		return ErrNoBackup
	}
	var record undoRecord
	if json.Unmarshal(data, &record) != nil || record.Version != 1 || record.Target == "" {
		return ErrUnknownFormat
	}
	doc, err := s.readState()
	if err != nil {
		return err
	}
	current, _, err := doc.partition(record.Target)
	if err != nil {
		return err
	}
	if !sameJSON(encode(relatedValues(current)), encode(record.After)) {
		return ErrStateChanged
	}
	next := partition{clone(current.Globals), clone(current.Atoms)}
	replaceShared(&next, record.Before)
	doc.setPartition(record.Target, next)
	if !record.Existed && record.Target != doc.owner && len(next.Globals) == 0 && len(next.Atoms) == 0 {
		entry, _ := decodeObject(doc.inactive[record.Target])
		if len(entry) == 2 {
			delete(doc.inactive, record.Target)
			doc.renderer["inactive"] = encode(doc.inactive)
			doc.root[rendererKey] = encode(doc.renderer)
		}
	}
	reverse := undoRecord{Target: record.Target, Before: record.After, After: record.Before, Existed: true}
	return s.commit(doc.original, encode(doc.root), reverse)
}
func replaceShared(dst *partition, src partition) {
	for _, key := range projectGlobals {
		delete(dst.Globals, key)
		if raw, ok := src.Globals[key]; ok {
			dst.Globals[key] = raw
		}
	}
	for _, key := range projectAtoms {
		delete(dst.Atoms, key)
		if raw, ok := src.Atoms[key]; ok {
			dst.Atoms[key] = raw
		}
	}
}
