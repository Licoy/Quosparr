package claudedesktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Multiple surviving Windows installs can leave independent official profiles.
// Choosing whichever candidate appears first would silently bind the wrong user.
func chooseDesktopDataDir(paths []string, platform string) (string, error) {
	existing := []string{}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect Claude Desktop data directory: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("Claude Desktop data path is not a regular directory")
		}
		duplicate := false
		for _, known := range existing {
			if sameExecutable(path, known, platform) {
				duplicate = true
			}
		}
		if !duplicate {
			existing = append(existing, filepath.Clean(path))
		}
	}
	if len(existing) > 1 {
		return "", errors.New("Claude Desktop data directory is ambiguous between multiple official installations")
	}
	if len(existing) == 1 {
		if !sameExecutable(existing[0], paths[0], platform) {
			return "", errors.New("Claude Desktop data directory does not match the selected installation; migration is ambiguous")
		}
		return existing[0], nil
	}
	if len(paths) == 0 {
		return "", errors.New("Claude Desktop official data directory is unavailable")
	}
	// A fresh profile has not yet been created; the install-specific native path
	// is deterministic and Desktop itself will create it at ordinary launch.
	return paths[0], nil
}
