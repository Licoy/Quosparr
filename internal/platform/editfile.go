package platform

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupPolicy says where EditFileWithBackup keeps the copies it makes before
// changing a user-owned file. Files are named <Prefix><UTC timestamp><Ext> and
// only the newest Keep of them are retained.
type BackupPolicy struct {
	Dir    string
	Prefix string
	Ext    string
	Keep   int
}

const (
	editedFileDefaultPerm = 0o600
	backupTimeLayout      = "20060102T150405.000000000Z"
)

// EditFileWithBackup reads path (a missing file reads as nil), lets edit
// produce the new content, and when the content changed, copies the old file
// into the backup directory and replaces path atomically with the existing
// mode (0600 for a new file). Nothing is written when edit returns the same
// bytes or an error. The backup is made before the write; if it fails the
// original file is left untouched. A symlinked path is edited through to its
// target so the link survives.
func EditFileWithBackup(path string, backup BackupPolicy, now time.Time, edit func(old []byte) ([]byte, error)) error {
	return editFileWithBackup(path, backup, now, edit, nil, false)
}

// EditFileWithBackupChecked is EditFileWithBackup with a final caller check
// after the backup and temporary replacement are ready, immediately before
// the atomic replacement. A failed check leaves the destination untouched.
func EditFileWithBackupChecked(path string, backup BackupPolicy, now time.Time, edit func(old []byte) ([]byte, error), beforeReplace func() error) error {
	return editFileWithBackup(path, backup, now, edit, beforeReplace, false)
}

// EditPrivateFileWithBackupChecked writes a secret-bearing configuration with
// private permissions, including the temporary replacement and Windows ACL.
func EditPrivateFileWithBackupChecked(path string, backup BackupPolicy, now time.Time, edit func(old []byte) ([]byte, error), beforeReplace func() error) error {
	return editFileWithBackup(path, backup, now, edit, beforeReplace, true)
}

func editFileWithBackup(path string, backup BackupPolicy, now time.Time, edit func(old []byte) ([]byte, error), beforeReplace func() error, private bool) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	old, mode, exists, err := readExisting(target)
	if err != nil {
		return err
	}
	out, err := edit(old)
	if err != nil {
		return err
	}
	if bytes.Equal(old, out) && (exists || len(out) == 0) {
		return nil // unchanged, and never create an empty file
	}
	if exists {
		if err := writeBackup(backup, old, now); err != nil {
			return err
		}
	}
	if private {
		mode = editedFileDefaultPerm
	}
	return writeFileAtomic(target, out, mode, beforeReplace)
}

func readExisting(path string) (data []byte, mode fs.FileMode, exists bool, err error) {
	mode = editedFileDefaultPerm
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, mode, false, nil
	}
	if err != nil {
		return nil, mode, false, err
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, mode, false, err
	}
	return data, info.Mode().Perm(), true, nil
}

func writeBackup(p BackupPolicy, data []byte, now time.Time) error {
	if err := EnsurePrivateDirectory(p.Dir); err != nil {
		return err
	}
	stamp := now.UTC()
	var name string
	for {
		name = filepath.Join(p.Dir, p.Prefix+stamp.Format(backupTimeLayout)+p.Ext)
		if _, err := os.Lstat(name); errors.Is(err, os.ErrNotExist) {
			break
		}
		// Same instant as an earlier backup: never overwrite the older copy.
		stamp = stamp.Add(time.Nanosecond)
	}
	if err := WriteFileAtomic(name, data, editedFileDefaultPerm); err != nil {
		return err
	}
	pruneBackups(p)
	return nil
}

// pruneBackups is best effort: a leftover old backup is harmless, whereas
// failing here would abort an edit whose backup already succeeded.
func pruneBackups(p BackupPolicy) {
	if p.Keep <= 0 {
		return
	}
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasPrefix(name, p.Prefix) && strings.HasSuffix(name, p.Ext) {
			names = append(names, name)
		}
	}
	if len(names) <= p.Keep {
		return
	}
	// Fixed-width UTC timestamps sort chronologically.
	sort.Strings(names)
	for _, name := range names[:len(names)-p.Keep] {
		_ = os.Remove(filepath.Join(p.Dir, name))
	}
}
