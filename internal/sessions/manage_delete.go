package sessions

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// DeleteResult is the outcome for one selected session. Err can describe a
// partial move when the system Trash accepted only some files in the group.
type DeleteResult struct {
	Session ManagedSession
	Removed int
	Total   int
	Err     error
}

// DeleteManaged moves only the files captured by ListManaged to the system
// Trash. trustedRoots must come from the running provider configuration.
func DeleteManaged(ctx context.Context, selected []ManagedSession, trustedRoots map[string][]string, trash func(string) error) ([]DeleteResult, error) {
	results := make([]DeleteResult, 0, len(selected))
	var failures []error
	overlaps := overlappingManagedFiles(selected)
	for i, session := range selected {
		var err error
		removed := 0
		if overlap := overlaps[i]; overlap != nil {
			err = overlap
		} else if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		} else {
			removed, err = deleteManagedSession(ctx, session, trustedRoots[session.Provider], trash)
		}
		results = append(results, DeleteResult{Session: session, Removed: removed, Total: len(session.Files), Err: err})
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", session.ID, err))
		}
	}
	return results, errors.Join(failures...)
}

func deleteManagedSession(ctx context.Context, session ManagedSession, trustedRoots []string, trash func(string) error) (int, error) {
	if trash == nil {
		return 0, errors.New("system Trash is unavailable")
	}
	if err := validateManagedSession(session, trustedRoots); err != nil {
		return 0, err
	}
	files := session.Files
	if session.Provider == ProviderGrok {
		files = append([]ManagedFile(nil), files...)
		sort.SliceStable(files, func(i, j int) bool {
			return filepath.Base(files[i].Path) != "summary.json" && filepath.Base(files[j].Path) == "summary.json"
		})
	}
	moved := 0
	var failures []error
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		if err := validateManagedFile(session.Root, file); err != nil {
			failures = append(failures, err)
			break
		}
		if err := trash(file.Path); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", filepath.Base(file.Path), err))
			break
		}
		moved++
	}
	if len(failures) == 0 {
		return moved, nil
	}
	return moved, fmt.Errorf("moved %d of %d session files to Trash: %w", moved, len(session.Files), errors.Join(failures...))
}

func overlappingManagedFiles(selected []ManagedSession) map[int]error {
	owners := make(map[string]int)
	overlaps := make(map[int]error)
	for i, session := range selected {
		for _, file := range session.Files {
			path, err := cleanAbsolute(file.Path)
			if err != nil {
				continue
			}
			if previous, found := owners[path]; found && previous != i {
				overlaps[previous] = errors.New("selected sessions contain an overlapping file")
				overlaps[i] = errors.New("selected sessions contain an overlapping file")
			} else {
				owners[path] = i
			}
		}
	}
	return overlaps
}

func validateManagedSession(session ManagedSession, trustedRoots []string) error {
	switch session.Provider {
	case ProviderCodex, ProviderClaude, ProviderGrok, ProviderGemini:
	default:
		return errors.New("unknown session provider")
	}
	if len(session.Files) == 0 {
		return errors.New("session has no scanned files")
	}
	root, err := cleanAbsolute(session.Root)
	if err != nil {
		return err
	}
	if !matchesTrustedRoot(root, trustedRoots) {
		return errors.New("session root is not trusted")
	}
	path, err := cleanAbsolute(session.Path)
	if err != nil {
		return err
	}
	inside, err := pathWithinRoot(root, path)
	if err != nil || !inside || path == root {
		return errors.New("session path is outside its trusted root")
	}
	if err := managedParentChain(root, path); err != nil {
		return err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("check session path: %w", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("session path is a symbolic link")
	}
	if session.Provider == ProviderGrok && !pathInfo.IsDir() {
		return errors.New("Grok session path is not a directory")
	}
	if session.Provider != ProviderGrok && !pathInfo.Mode().IsRegular() {
		return errors.New("session path is not a regular file")
	}
	seen := make(map[string]struct{}, len(session.Files))
	for _, file := range session.Files {
		path, err := cleanAbsolute(file.Path)
		if err != nil {
			return err
		}
		if _, duplicate := seen[path]; duplicate {
			return errors.New("session contains a duplicate file")
		}
		seen[path] = struct{}{}
		if session.Provider == ProviderGrok {
			inside, err := pathWithinRoot(session.Path, path)
			if err != nil || !inside || path == filepath.Clean(session.Path) {
				return errors.New("Grok session file is outside its session directory")
			}
		}
		if err := validateManagedFile(root, file); err != nil {
			return err
		}
	}
	return nil
}

func validateManagedFile(root string, file ManagedFile) error {
	path, err := cleanAbsolute(file.Path)
	if err != nil {
		return err
	}
	inside, err := pathWithinRoot(root, path)
	if err != nil || !inside || path == filepath.Clean(root) {
		return errors.New("session file is outside its trusted root")
	}
	if err := managedParentChain(root, path); err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("check %s: %w", filepath.Base(path), err)
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() {
		return fmt.Errorf("session file is not regular: %s", filepath.Base(path))
	}
	if file.Info == nil || !os.SameFile(file.Info, current) {
		return fmt.Errorf("session file identity changed: %s", filepath.Base(path))
	}
	if current.Size() != file.Size || !current.ModTime().Equal(file.ModTime) {
		return fmt.Errorf("session file changed after scanning: %s", filepath.Base(path))
	}
	return nil
}

func matchesTrustedRoot(root string, trusted []string) bool {
	for _, candidate := range trusted {
		clean, err := cleanAbsolute(candidate)
		if err == nil && clean == root {
			return true
		}
	}
	return false
}

func cleanAbsolute(path string) (string, error) {
	if path == "" {
		return "", fs.ErrInvalid
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
