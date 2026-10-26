package claudedesktop

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func (h *NativeHost) Resolve(ctx context.Context, selected string) (Application, error) {
	if err := ctx.Err(); err != nil {
		return Application{}, err
	}
	paths := []string{selected}
	if selected == "" {
		var err error
		paths, err = h.candidates(ctx)
		if err != nil {
			return Application{}, err
		}
	}
	found := []Application{}
	for _, path := range paths {
		app, err := h.inspectApplication(ctx, path)
		if err != nil {
			if selected != "" || !errors.Is(err, os.ErrNotExist) {
				return Application{}, err
			}
			continue
		}
		duplicate := false
		for _, known := range found {
			duplicate = duplicate || sameExecutable(known.Executable, app.Executable, h.platform)
		}
		if !duplicate {
			found = append(found, app)
		}
	}
	if len(found) > 1 {
		return Application{}, errors.New("multiple Claude Desktop installations were found; select the intended application explicitly")
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return Application{}, errors.New("Claude Desktop is not installed; select its application path")
}

func (h *NativeHost) inspectApplication(ctx context.Context, path string) (Application, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return Application{}, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return Application{}, err
	}
	exe, archive := path, ""
	switch h.platform {
	case "darwin":
		if filepath.Ext(path) != ".app" {
			return Application{}, errors.New("select the Claude Desktop .app bundle")
		}
		data, err := h.run(ctx, "/usr/bin/plutil", "-extract", "CFBundleIdentifier", "raw", "-o", "-", filepath.Join(path, "Contents", "Info.plist"))
		if err != nil {
			return Application{}, err
		}
		if strings.TrimSpace(string(data)) != "com.anthropic.claudefordesktop" {
			return Application{}, errors.New("application is not Claude Desktop")
		}
		name, err := h.run(ctx, "/usr/bin/plutil", "-extract", "CFBundleExecutable", "raw", "-o", "-", filepath.Join(path, "Contents", "Info.plist"))
		if err != nil {
			return Application{}, err
		}
		executable := strings.TrimSpace(string(name))
		if executable == "" || filepath.Base(executable) != executable {
			return Application{}, errors.New("invalid Claude Desktop bundle executable")
		}
		exe = filepath.Join(path, "Contents", "MacOS", executable)
		archive = filepath.Join(path, "Contents", "Resources", "app.asar")
		if _, err = h.run(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", path); err != nil {
			return Application{}, err
		}
	case "windows":
		if !strings.EqualFold(filepath.Base(path), "Claude.exe") {
			return Application{}, errors.New("select Claude Desktop Claude.exe, not the Claude CLI")
		}
		archive = filepath.Join(filepath.Dir(path), "resources", "app.asar")
	case "linux":
		if err := verifyDesktopELF(path); err != nil {
			return Application{}, err
		}
		archive = filepath.Join(filepath.Dir(path), "resources", "app.asar")
	default:
		return Application{}, errors.New("Claude Desktop platform is unsupported")
	}
	info, err := os.Stat(exe)
	if err != nil {
		return Application{}, err
	}
	// FileMode permissions follow the running OS, even when inspecting another
	// platform's application layout.
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
		return Application{}, errors.New("Claude Desktop executable is invalid")
	}
	if err := verifyDesktopArchive(archive); err != nil {
		return Application{}, err
	}
	dataDir, err := h.dataDir(ctx, path)
	if err != nil {
		return Application{}, err
	}
	return Application{Path: path, Executable: exe, DataDir: dataDir}, nil
}

// ASAR is read as a bounded archive; no application code is executed and no
// default Desktop profile or credentials are inspected.
func verifyDesktopArchive(path string) error {
	data, err := readASAREntry(path, []string{"package.json"}, 1<<20)
	if err != nil {
		return fmt.Errorf("cannot verify Claude Desktop installation: %w", err)
	}
	var metadata struct {
		Name        string `json:"name"`
		ProductName string `json:"productName"`
		Main        string `json:"main"`
	}
	if json.Unmarshal(data, &metadata) != nil || metadata.Name != "@ant/desktop" || metadata.ProductName != "Claude" || metadata.Main != ".vite/build/index.pre.js" {
		return errors.New("application is not the official Claude Desktop installation")
	}
	return nil
}

type asarEntry struct {
	Files    map[string]asarEntry `json:"files"`
	Offset   string               `json:"offset"`
	Size     int64                `json:"size"`
	Unpacked bool                 `json:"unpacked"`
	Link     string               `json:"link"`
}

func readASAREntry(path string, parts []string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var prefix [16]byte
	if _, err = io.ReadFull(f, prefix[:]); err != nil {
		return nil, err
	}
	headerSize := int64(binary.LittleEndian.Uint32(prefix[4:8]))
	jsonSize := int64(binary.LittleEndian.Uint32(prefix[12:16]))
	if binary.LittleEndian.Uint32(prefix[:4]) != 4 || headerSize < 8 || jsonSize < 2 || jsonSize > 32<<20 || jsonSize > headerSize-8 {
		return nil, errors.New("invalid ASAR header")
	}
	header := make([]byte, jsonSize)
	if _, err = io.ReadFull(f, header); err != nil {
		return nil, err
	}
	var entry asarEntry
	if err = json.Unmarshal(header, &entry); err != nil {
		return nil, errors.New("invalid ASAR directory")
	}
	for _, part := range parts {
		next, ok := entry.Files[part]
		if !ok {
			return nil, errors.New("ASAR startup entry is missing")
		}
		entry = next
	}
	offset, err := strconv.ParseInt(entry.Offset, 10, 64)
	if err != nil || offset < 0 || entry.Size <= 0 || entry.Size > limit || entry.Unpacked || entry.Link != "" {
		return nil, errors.New("invalid ASAR startup entry")
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := 8 + headerSize + offset
	if start < offset || start > stat.Size() || entry.Size > stat.Size()-start {
		return nil, errors.New("ASAR startup entry exceeds archive")
	}
	result := make([]byte, entry.Size)
	_, err = f.ReadAt(result, start)
	return result, err
}
