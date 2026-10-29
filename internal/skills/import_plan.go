package skills

import "path"

// importEntry is a file or link a source lists before anything is read.
type importEntry struct {
	path string // cleaned, relative to the source root
	size int64
	link bool
}

// pluginManifests are what Discover reads besides the skills' folders.
var pluginManifests = map[string]bool{".claude-plugin/marketplace.json": true, ".claude-plugin/plugin.json": true}

// plan narrows a listed source to the folders holding a SKILL.md, the
// folders above them and the plugin manifests, so that a large repository
// is read only for its skills; Discover finds the same skills either way.
// A SKILL.md at the root keeps everything. The limits are checked against
// the listing, before anything is read.
func (s *Snapshot) plan(entries []importEntry) error {
	folders := map[string]bool{}
	for _, e := range entries {
		if !e.link && path.Base(e.path) == "SKILL.md" && !importSkipped(e.path, false) {
			folders[parentImport(e.path)] = true
		}
	}
	if len(folders) == 0 {
		return importError(ImportNoSkills, "")
	}
	keep := func(string) bool { return true }
	if !folders[""] {
		above := map[string]bool{".claude-plugin": true}
		for dir := range folders {
			for d := parentImport(dir); d != "" && !above[d]; d = parentImport(d) {
				above[d] = true
			}
		}
		keep = func(p string) bool {
			if above[p] || pluginManifests[p] {
				return true
			}
			for d := p; d != ""; d = parentImport(d) {
				if folders[d] {
					return true
				}
			}
			return false
		}
		s.keep = keep
	}
	var size int64
	files, links := 0, 0
	for _, e := range entries {
		switch {
		case importSkipped(e.path, false) || !keep(e.path):
		case e.link:
			links++
		case e.size > MaxImportFileBytes:
			return importError(ImportTooLarge, e.path)
		default:
			files++
			size += e.size
		}
	}
	switch {
	case files > MaxImportFiles || links > MaxImportFiles:
		return importError(ImportTooManyFiles, "")
	case size > MaxImportBytes:
		return importError(ImportTooLarge, "")
	}
	return nil
}

// wants reports whether an entry is worth reading: everything unless plan
// narrowed the source. A path that does not clean is read to be rejected.
func (s *Snapshot) wants(raw string) bool {
	if s.keep == nil {
		return true
	}
	p, ok := cleanImportPath(raw)
	return !ok || s.keep(p)
}
