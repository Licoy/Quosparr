package skills

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxImportWalk caps the entries a folder import visits, kept or skipped.
const maxImportWalk = 5 * MaxImportFiles

// folderEntry is what a folder walk found, before anything is read.
type folderEntry struct {
	path string
	kind fs.FileMode // fs.ModeDir, fs.ModeSymlink or 0 for a file
}

// LoadFolder reads a local folder into memory: the folder is listed first
// and only the skills' folders are read. Links are kept as links and only
// followed later inside the skill that holds them.
func LoadFolder(ctx context.Context, dir string) (*Snapshot, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, importError(ImportInvalidSource, "")
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return nil, importError(ImportInvalidSource, "")
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, importError(ImportInvalidSource, "")
	}
	defer root.Close()
	snap := newSnapshot(Source{Kind: SourceFolder, Path: filepath.Clean(abs)})
	var found []folderEntry
	var listing []importEntry
	err = fs.WalkDir(root.FS(), ".", func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if p == "." {
				return importError(ImportInvalidSource, "")
			}
			snap.rejected = append(snap.rejected, p)
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if len(found) >= maxImportWalk {
			return importError(ImportTooManyFiles, "")
		}
		kind := entry.Type() & (fs.ModeDir | fs.ModeSymlink)
		switch {
		case entry.IsDir() && importSkippedDirs[entry.Name()]:
			return fs.SkipDir
		case kind == 0 && !entry.Type().IsRegular():
			return nil // devices, sockets and pipes are never read
		}
		found = append(found, folderEntry{path: p, kind: kind})
		if clean, ok := cleanImportPath(p); ok && kind != fs.ModeDir {
			e := importEntry{path: clean, link: kind == fs.ModeSymlink}
			if info, err := entry.Info(); err == nil {
				e.size = info.Size()
			}
			listing = append(listing, e)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := snap.plan(listing); err != nil {
		return nil, err
	}
	for _, e := range found {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !snap.wants(e.path) {
			continue
		}
		if err := addFolderEntry(snap, root, e); err != nil {
			return nil, err
		}
	}
	return snap, snap.finish()
}

func addFolderEntry(snap *Snapshot, root *os.Root, e folderEntry) error {
	switch e.kind {
	case fs.ModeDir:
		return snap.addDir(e.path)
	case fs.ModeSymlink:
		target, err := root.Readlink(e.path)
		if err != nil {
			snap.rejected = append(snap.rejected, e.path)
			return nil
		}
		return snap.addLink(e.path, filepath.ToSlash(target))
	}
	if importSkipped(e.path, false) {
		return nil
	}
	data, exec, err := readImportFile(root, e.path)
	if err != nil {
		return err
	}
	return snap.addFile(e.path, data, exec)
}

func readImportFile(root *os.Root, p string) ([]byte, bool, error) {
	info, err := root.Lstat(p)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false, importError(ImportInvalidSource, p)
	}
	if info.Size() > MaxImportFileBytes {
		return nil, false, importError(ImportTooLarge, p)
	}
	file, err := root.Open(p)
	if err != nil {
		return nil, false, importError(ImportInvalidSource, p)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, false, importError(ImportInvalidSource, p)
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxImportFileBytes+1))
	if err != nil {
		return nil, false, importError(ImportInvalidSource, p)
	}
	if len(data) > MaxImportFileBytes {
		return nil, false, importError(ImportTooLarge, p)
	}
	return data, info.Mode().Perm()&0o111 != 0, nil
}

// LoadZip reads a .zip (or .skill) archive into memory.
func LoadZip(ctx context.Context, file string) (*Snapshot, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, importError(ImportInvalidSource, "")
	}
	info, err := os.Stat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return nil, importError(ImportInvalidSource, "")
	}
	if info.Size() > MaxImportDownload {
		return nil, importError(ImportTooLarge, "")
	}
	reader, err := zip.OpenReader(abs)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, importError(ImportInvalidArchive, "")
	}
	defer reader.Close()
	snap := newSnapshot(Source{Kind: SourceZip, Path: filepath.Clean(abs)})
	var listing []importEntry
	for _, f := range reader.File {
		mode := f.Mode()
		if strings.HasSuffix(f.Name, "/") || !mode.IsRegular() && mode&fs.ModeSymlink == 0 {
			continue
		}
		if p, ok := cleanImportPath(f.Name); ok {
			listing = append(listing, importEntry{path: p, size: int64(min(f.UncompressedSize64, 1<<62)), link: mode&fs.ModeSymlink != 0})
		}
	}
	if err := snap.plan(listing); err != nil {
		return nil, err
	}
	var expanded int64
	for _, f := range reader.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !snap.wants(strings.TrimSuffix(f.Name, "/")) {
			continue
		}
		if err := addZipEntry(snap, f, &expanded); err != nil {
			return nil, err
		}
	}
	return snap, snap.finish()
}

func addZipEntry(snap *Snapshot, f *zip.File, expanded *int64) error {
	name, mode := f.Name, f.Mode()
	if f.Flags&0x1 != 0 {
		return importError(ImportInvalidArchive, "") // encrypted
	}
	if strings.HasSuffix(name, "/") || mode.IsDir() {
		return snap.addDir(name)
	}
	if importSkipped(strings.TrimSuffix(name, "/"), false) {
		return nil
	}
	limit := int64(MaxImportFileBytes)
	if mode&fs.ModeSymlink != 0 {
		limit = 4096
	} else if !mode.IsRegular() {
		return nil
	}
	if f.UncompressedSize64 > uint64(limit) {
		return importError(ImportTooLarge, name)
	}
	in, err := f.Open()
	if err != nil {
		return importError(ImportInvalidArchive, "")
	}
	data, err := io.ReadAll(io.LimitReader(in, limit+1))
	in.Close()
	if err != nil {
		return importError(ImportInvalidArchive, "")
	}
	if *expanded += int64(len(data)); int64(len(data)) > limit || *expanded > MaxImportExpanded {
		return importError(ImportTooLarge, name)
	}
	if mode&fs.ModeSymlink != 0 {
		return snap.addLink(name, string(data))
	}
	return snap.addFile(name, data, mode.Perm()&0o111 != 0)
}
