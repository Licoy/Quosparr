package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/licoy/quosparr/internal/platform"
)

const (
	// SnapshotFileName is the widget snapshot file.
	SnapshotFileName = "widget-snapshot.json"
	// DefaultAppGroupID is the App Group the Swift widget extension reads.
	DefaultAppGroupID = "group.com.quosparr.app"
)

var (
	// ErrMissing: there is no snapshot file yet.
	ErrMissing = errors.New("export: widget snapshot is missing")
	// ErrAppGroupUnavailable: the App Group container cannot be located on
	// this system.
	ErrAppGroupUnavailable = errors.New("export: app group container is unavailable")
)

// UnsupportedSchemaError is a snapshot of another schema version.
type UnsupportedSchemaError struct{ Version int }

func (e UnsupportedSchemaError) Error() string {
	return fmt.Sprintf("export: unsupported widget snapshot schema version %d", e.Version)
}

// SnapshotStore reads and writes widget-snapshot.json.
type SnapshotStore struct {
	Path string
}

// LocalSnapshotPath is ~/.quosparr/widget-snapshot.json under home, the
// location of every build that does not opt into the App Group.
func LocalSnapshotPath(home string) string {
	return filepath.Join(platform.RunwayDir(home), SnapshotFileName)
}

// AppGroupSnapshotPath is the snapshot in the App Group container,
// ~/Library/Group Containers/<group>/widget-snapshot.json. Only macOS has App
// Groups; elsewhere it is ErrAppGroupUnavailable. The container is derived
// from the home directory instead of asking Cocoa, which avoids the privacy
// prompt but does not create the container: it must already exist, as it does
// once the signed Swift host has run.
func AppGroupSnapshotPath(home, groupID string) (string, error) {
	return appGroupSnapshotPath(runtime.GOOS, home, groupID)
}

func appGroupSnapshotPath(goos, home, groupID string) (string, error) {
	if goos != "darwin" {
		return "", ErrAppGroupUnavailable
	}
	if groupID == "" {
		groupID = DefaultAppGroupID
	}
	return filepath.Join(home, "Library", "Group Containers", groupID, SnapshotFileName), nil
}

// SnapshotPath picks the file for a storage mode. groupID may be empty for the
// default group.
func SnapshotPath(mode StorageMode, groupID, home string) (string, error) {
	if mode == StorageAppGroup {
		return AppGroupSnapshotPath(home, groupID)
	}
	return LocalSnapshotPath(home), nil
}

// Save writes the snapshot compact with sorted keys, atomically. The file is
// 0600 and a directory created for it 0700.
func (s SnapshotStore) Save(snapshot Snapshot) error {
	data, err := snapshot.Encode()
	if err != nil {
		return err
	}
	return platform.WriteFileAtomic(s.Path, data, 0o600)
}

// Load reads the snapshot; ErrMissing without a file, UnsupportedSchemaError
// for another schema version. Unlike Foundation's strict ISO 8601 decoder,
// times with fractional seconds are accepted.
func (s SnapshotStore) Load() (Snapshot, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, ErrMissing
	}
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if snapshot.SchemaVersion != SchemaVersion {
		return Snapshot{}, UnsupportedSchemaError{snapshot.SchemaVersion}
	}
	return snapshot, nil
}
