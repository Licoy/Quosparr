package app

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/platform"
)

// DirectMygoPaths points mygo's user data, cache and log directories into
// ~/.quosparr, so the framework's own files (updater.json,
// window-state.json, ...) stay in the one app-owned directory instead of
// ~/Library/Application Support, %AppData% or ~/.config. It must run before
// anything calls App.Path. It creates the directories (0700).
//
// mygo v0.6.2 offers App.SetPath for PathAppData, PathUserData, PathCache
// and PathLogs; the single-instance socket lives in the temp directory and
// cannot be moved (it is not a data file).
func DirectMygoPaths(app *mygo.Application, home string) error {
	root := platform.RunwayDir(home)
	dirs := map[mygo.PathName]string{
		mygo.PathUserData: root,
		mygo.PathCache:    filepath.Join(root, "cache"),
		mygo.PathLogs:     filepath.Join(root, "logs"),
	}
	for name, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		app.SetPath(name, dir)
	}
	return nil
}
