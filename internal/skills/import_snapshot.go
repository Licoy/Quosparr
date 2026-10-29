package skills

import (
	"path"
	"sort"
	"strings"
)

// Snapshot is a source read into memory, rooted at the chosen folder.
// Paths are slash-separated and relative to that root.
type Snapshot struct {
	Source Source
	Commit string // the GitHub commit the files were read at
	files  map[string]importFile
	links  map[string]string // symlink path to its target, never followed outside
	dirs   map[string]bool
	lower  map[string]string // lowercased path to the path that took it
	// rejected are entries that could not be kept: unsafe or unusable
	// names and names that differ from another only in case. A skill
	// containing one cannot be installed.
	rejected  []string
	trees     map[string]string // GitHub tree SHA by folder, when known
	size      int64
	children  map[string][]string
	order     []string // files and links, sorted
	truncated bool
	keep      func(string) bool // what plan kept, nil for everything
}

// Truncated reports that Discover stopped at MaxImportSkills.
func (s *Snapshot) Truncated() bool { return s.truncated }

// Rejected are the source's paths left out as unsafe or as differing from
// another only in case.
func (s *Snapshot) Rejected() []string { return s.rejected }

type importFile struct {
	data []byte
	exec bool
}

func newSnapshot(src Source) *Snapshot {
	return &Snapshot{Source: src, files: map[string]importFile{}, links: map[string]string{},
		dirs: map[string]bool{"": true}, lower: map[string]string{}}
}

// importSkipped reports a path inside a folder, or a file, a source never
// keeps; "._" files are macOS AppleDouble metadata.
func importSkipped(p string, dir bool) bool {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		last := i == len(parts)-1
		if importSkippedDirs[part] && (!last || dir) || last && !dir && (importSkippedFiles[part] || strings.HasPrefix(part, "._")) {
			return true
		}
	}
	return false
}

// claim records p in the case-insensitive index, or rejects it when a
// different path already holds the same letters.
func (s *Snapshot) claim(p string) bool {
	key := strings.ToLower(p)
	if other, ok := s.lower[key]; ok && other != p {
		s.rejected = append(s.rejected, p)
		return false
	}
	s.lower[key] = p
	return true
}

func (s *Snapshot) addDir(raw string) error {
	p, ok := cleanImportPath(raw)
	if !ok {
		s.rejected = append(s.rejected, raw)
		return nil
	}
	if importSkipped(p, true) || s.keep != nil && !s.keep(p) {
		return nil
	}
	for dir := p; dir != "" && !s.dirs[dir]; dir = parentImport(dir) {
		if s.has(dir) || s.links[dir] != "" {
			s.rejected = append(s.rejected, dir)
			return nil
		}
		if !s.claim(dir) {
			return nil
		}
		s.dirs[dir] = true
	}
	return nil
}

// parentImport is the folder holding p, "" for the root.
func parentImport(p string) string {
	if dir := path.Dir(p); dir != "." {
		return dir
	}
	return ""
}

func (s *Snapshot) addFile(raw string, data []byte, exec bool) error {
	p, ok := cleanImportPath(raw)
	if !ok {
		s.rejected = append(s.rejected, raw)
		return nil
	}
	if importSkipped(p, false) || s.keep != nil && !s.keep(p) {
		return nil
	}
	if len(s.files) >= MaxImportFiles {
		return importError(ImportTooManyFiles, "")
	}
	if s.size+int64(len(data)) > MaxImportBytes {
		return importError(ImportTooLarge, "")
	}
	if _, dup := s.files[p]; dup || s.dirs[p] || !s.claim(p) {
		if dup || s.dirs[p] {
			s.rejected = append(s.rejected, p)
		}
		return nil
	}
	if dir := parentImport(p); dir != "" {
		if err := s.addDir(dir); err != nil {
			return err
		}
	}
	s.files[p] = importFile{data: data, exec: exec}
	s.size += int64(len(data))
	return nil
}

func (s *Snapshot) addLink(raw, target string) error {
	p, ok := cleanImportPath(raw)
	if !ok {
		s.rejected = append(s.rejected, raw)
		return nil
	}
	if importSkipped(p, false) || s.keep != nil && !s.keep(p) {
		return nil
	}
	if len(s.links) >= MaxImportFiles {
		return importError(ImportTooManyFiles, "")
	}
	if !s.claim(p) {
		return nil
	}
	if dir := parentImport(p); dir != "" {
		if err := s.addDir(dir); err != nil {
			return err
		}
	}
	s.links[p] = target
	return nil
}

// finish indexes the folders once everything is read.
func (s *Snapshot) finish() error {
	if len(s.files) == 0 {
		return importError(ImportNoSkills, "")
	}
	s.children = map[string][]string{}
	for dir := range s.dirs {
		if dir == "" {
			continue
		}
		parent := parentImport(dir)
		s.children[parent] = append(s.children[parent], dir)
	}
	for _, list := range s.children {
		sort.Strings(list)
	}
	s.order = make([]string, 0, len(s.files)+len(s.links))
	for p := range s.files {
		s.order = append(s.order, p)
	}
	for p := range s.links {
		s.order = append(s.order, p)
	}
	sort.Strings(s.order)
	sort.Strings(s.rejected)
	return nil
}

func (s *Snapshot) has(p string) bool {
	_, ok := s.files[p]
	return ok
}

// joinImport joins snapshot paths, where "" is the root.
func joinImport(dir, name string) string {
	if dir == "" {
		return name
	}
	if name == "" {
		return dir
	}
	return dir + "/" + name
}

// inFolder reports whether p is folder itself or inside it.
func inFolder(folder, p string) bool {
	return folder == "" || p == folder || strings.HasPrefix(p, folder+"/")
}

// relInFolder is p relative to folder.
func relInFolder(folder, p string) string {
	if folder == "" {
		return p
	}
	return strings.TrimPrefix(p, folder+"/")
}

// resolveLink follows a link inside the snapshot to the regular file it
// names, staying within folder; links to folders, outside folder or that
// go round are not followed.
func (s *Snapshot) resolveLink(folder, p string) (importFile, bool) {
	for range maxImportLinkHops {
		target, ok := s.links[p]
		if !ok {
			break
		}
		if target == "" || strings.HasPrefix(target, "/") || strings.Contains(target, "\\") {
			return importFile{}, false
		}
		next := path.Join(path.Dir(p), target)
		if next == ".." || strings.HasPrefix(next, "../") || !inFolder(folder, next) || next == folder {
			return importFile{}, false
		}
		if file, ok := s.files[next]; ok {
			return file, true
		}
		p = next
	}
	return importFile{}, false
}
