package mcp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

func deleteServers(ctx context.Context, source Source, selected []Server, now time.Time) ([]DeleteResult, error) {
	results := makeResults(selected)
	if err := ctx.Err(); err != nil {
		return failResults(results, err), err
	}
	if err := validateSource(source, true); err != nil {
		return failResults(results, err), err
	}
	if len(selected) == 0 {
		return results, nil
	}
	original, info, exists, err := readRegular(source.Path)
	if err != nil || !exists {
		if err == nil {
			err = errors.New("MCP configuration no longer exists")
		}
		return failResults(results, err), err
	}
	names, fingerprint, err := validateSelection(source, selected, info)
	if err != nil {
		return failResults(results, err), err
	}
	if fileFingerprint(original, info) != fingerprint {
		err = errors.New("MCP configuration changed since it was scanned")
		return failResults(results, err), err
	}
	err = rewrite(ctx, source, original, info, fingerprint, now, func(old []byte) ([]byte, error) {
		switch source.Provider {
		case ProviderCodex, ProviderGrok:
			return deleteTOML(old, source.Provider, names)
		case ProviderClaude:
			return deleteJSON(old, names)
		default:
			return nil, errors.New("unsupported MCP provider")
		}
	})
	if err != nil {
		return failResults(results, err), err
	}
	if err := verifyWritten(source, names); err != nil {
		return failResults(results, err), err
	}
	return results, nil
}

// rewrite applies edit to the scanned configuration after a backup,
// refusing any change made to the file since the scan.
func rewrite(ctx context.Context, source Source, original []byte, info fs.FileInfo, fingerprint string, now time.Time, edit func([]byte) ([]byte, error)) error {
	if err := validateBackupDir(source.BackupDir); err != nil {
		return err
	}
	policy := platform.BackupPolicy{Dir: source.BackupDir, Prefix: string(source.Provider) + "-mcp-config-", Ext: filepath.Ext(source.Path), Keep: 10}
	checkCurrent := func() error {
		current, currentInfo, exists, err := readRegular(source.Path)
		if err != nil || !exists || !os.SameFile(info, currentInfo) {
			return errors.New("MCP configuration file identity changed")
		}
		if fileFingerprint(current, currentInfo) != fingerprint {
			return errors.New("MCP configuration changed since it was scanned")
		}
		return nil
	}
	err := platform.EditFileWithBackupChecked(source.Path, policy, now, func(old []byte) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := checkCurrent(); err != nil {
			return nil, err
		}
		if digest(old) != digest(original) {
			return nil, errors.New("MCP configuration changed since it was scanned")
		}
		return edit(old)
	}, checkCurrent)
	return sanitizeError(err)
}

func validateBackupDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("MCP backup directory cannot be inspected")
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("MCP backup path is not a regular directory")
	}
	return nil
}

func validateSelection(source Source, selected []Server, currentInfo fs.FileInfo) (map[string]struct{}, string, error) {
	names := make(map[string]struct{}, len(selected))
	fingerprint := selected[0].Fingerprint
	if fingerprint == "" {
		return nil, "", errors.New("selected MCP server has no snapshot fingerprint")
	}
	for _, server := range selected {
		if !server.Deletable {
			return nil, "", fmt.Errorf("invalid MCP selection %q", server.Name)
		}
		if err := validateServer(source, server, currentInfo); err != nil {
			return nil, "", err
		}
		if server.Fingerprint != fingerprint {
			return nil, "", errors.New("selected MCP servers come from different snapshots")
		}
		if _, duplicate := names[server.Name]; duplicate {
			return nil, "", fmt.Errorf("duplicate MCP selection %q", server.Name)
		}
		names[server.Name] = struct{}{}
	}
	return names, fingerprint, nil
}

func validateServer(source Source, server Server, currentInfo fs.FileInfo) error {
	if server.Provider != source.Provider || server.Path != source.Path || server.ID != string(source.Provider)+"\x00"+server.Name {
		return fmt.Errorf("invalid MCP selection %q", server.Name)
	}
	if server.fileInfo == nil || !os.SameFile(server.fileInfo, currentInfo) {
		return errors.New("MCP configuration file identity changed")
	}
	return nil
}

func verifyWritten(source Source, names map[string]struct{}) error {
	snapshot, err := load(context.Background(), source)
	if err != nil {
		return errors.New("written MCP configuration could not be verified")
	}
	for _, server := range snapshot.Servers {
		if _, removed := names[server.Name]; removed {
			return fmt.Errorf("MCP server %q remains after deletion", server.Name)
		}
	}
	return nil
}

func makeResults(selected []Server) []DeleteResult {
	results := make([]DeleteResult, len(selected))
	for i, server := range selected {
		results[i].Server = server
	}
	return results
}

func failResults(results []DeleteResult, err error) []DeleteResult {
	for i := range results {
		results[i].Err = err
	}
	return results
}

func sanitizeError(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return fmt.Errorf("MCP configuration operation failed: %s", pathErr.Op)
	}
	return err
}
