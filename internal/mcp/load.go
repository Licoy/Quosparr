package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const maxConfigSize = 8 << 20

func load(ctx context.Context, source Source) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := validateSource(source, false); err != nil {
		return Snapshot{}, err
	}
	data, info, exists, err := readRegular(source.Path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read %s MCP configuration: %w", source.Provider, err)
	}
	if !exists {
		return Snapshot{Path: source.Path}, nil
	}
	fingerprint := fileFingerprint(data, info)
	var servers []Server
	switch source.Provider {
	case ProviderCodex, ProviderGrok:
		servers, err = loadTOML(source, data, fingerprint)
	case ProviderClaude:
		servers, err = loadJSON(source, data, fingerprint)
	case ProviderGemini:
		if !blankJSON(data) {
			servers, err = loadJSON(source, data, fingerprint)
		}
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse %s MCP configuration: %w", source.Provider, err)
	}
	for i := range servers {
		servers[i].fileInfo = info
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
	return Snapshot{Servers: servers, Path: source.Path, Fingerprint: fingerprint, fileInfo: info}, nil
}

func validateSource(source Source, deleting bool) error {
	switch source.Provider {
	case ProviderCodex, ProviderClaude, ProviderGrok, ProviderGemini:
	default:
		return fmt.Errorf("unsupported MCP provider %q", source.Provider)
	}
	if source.Path == "" || !filepath.IsAbs(source.Path) {
		return errors.New("MCP configuration path must be absolute")
	}
	if deleting && (source.BackupDir == "" || !filepath.IsAbs(source.BackupDir)) {
		return errors.New("MCP backup directory must be absolute")
	}
	return nil
}

func readRegular(path string) ([]byte, fs.FileInfo, bool, error) {
	initial, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, err
	}
	if !initial.Mode().IsRegular() {
		return nil, nil, false, errors.New("configuration is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, false, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !os.SameFile(initial, info) || !os.SameFile(info, pathInfo) || !info.Mode().IsRegular() || !pathInfo.Mode().IsRegular() {
		return nil, nil, false, errors.New("configuration is not a regular file")
	}
	if info.Size() > maxConfigSize {
		return nil, nil, false, errors.New("configuration is too large")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxConfigSize+1))
	if err != nil {
		return nil, nil, false, err
	}
	if len(data) > maxConfigSize {
		return nil, nil, false, errors.New("configuration is too large")
	}
	after, statErr := file.Stat()
	pathAfter, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !os.SameFile(info, after) || !os.SameFile(info, pathAfter) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return nil, nil, false, errors.New("configuration changed while reading")
	}
	return data, after, true, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fileFingerprint(data []byte, info fs.FileInfo) string {
	identity := fmt.Sprintf("%d:%d:%s", info.Size(), info.ModTime().UnixNano(), digest(data))
	return digest([]byte(identity))
}

func baseServer(source Source, name, fingerprint string, config map[string]any) (Server, error) {
	transport, command, rawURL, problem := summarizeConnection(source.Provider, config)
	details, err := redactedDetails(config)
	if err != nil {
		return Server{}, err
	}
	return Server{
		ID:          string(source.Provider) + "\x00" + name,
		Name:        name,
		Provider:    source.Provider,
		Path:        source.Path,
		Transport:   transport,
		Command:     safeCommand(command),
		URL:         safeURL(rawURL),
		Problem:     problem,
		Details:     details,
		Enabled:     boolValue(config["enabled"], true),
		Deletable:   true,
		Fingerprint: fingerprint,
	}, nil
}
