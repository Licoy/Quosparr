package skills

import (
	"encoding/json"
	"path"
	"strings"
)

// CandidateProblem is why a found skill cannot be installed.
type CandidateProblem string

// CandidateProblem values.
const (
	ProblemNone         CandidateProblem = ""
	ProblemInvalidFront CandidateProblem = "invalidFront" // frontmatter does not parse
	ProblemMissingDesc  CandidateProblem = "missingDesc"  // no description; Codex would refuse it
	ProblemInvalidName  CandidateProblem = "invalidName"  // no usable folder name
	ProblemInvalidPath  CandidateProblem = "invalidPath"  // it holds an entry that could not be read safely
)

// ImportFile is one file an installed skill gets.
type ImportFile struct {
	Path string // slash-separated, relative to the skill's folder
	Size int64
	Exec bool
}

// Candidate is one skill found in a source.
type Candidate struct {
	Folder        string // slash-separated folder in the source, "" for its root
	Name          string // frontmatter name, or the folder's name
	Description   string
	DirName       string // the folder it installs as
	Plugin        string // the plugin whose manifest lists it
	Fields        []Field
	Body          string // SKILL.md after the frontmatter, at most MaxBodyBytes
	BodyTruncated bool
	Files         []ImportFile // what gets installed, by path; SKILL.md included
	Size          int64
	Executables   int
	Skipped       []string // links that are not followed, relative to Folder
	Problem       CandidateProblem
	TreeSHA       string // GitHub tree SHA of Folder, when known
	data          map[string]importFile
}

// discoverSkipped are folders a search never enters.
var discoverSkipped = map[string]bool{"node_modules": true, ".git": true, "dist": true, "build": true, "__pycache__": true}

// discoverContainers are where repositories keep skills; each is searched
// three levels deep, so skills/<group>/<skill> is found too.
var discoverContainers = []string{
	"skills", "skills/.curated", "skills/.experimental", "skills/.system",
	".agents/skills", ".claude/skills", ".codex/skills", ".cursor/skills", ".gemini/skills",
	".github/skills", ".grok/skills", ".opencode/skills", ".windsurf/skills",
}

const (
	discoverContainerDepth = 3
	discoverFallbackDepth  = 5
)

type discoverer struct {
	snap    *Snapshot
	seen    map[string]bool
	names   map[string]bool
	plugins map[string]string
	found   []Candidate
}

// Discover finds the skills in a source. A SKILL.md at the root makes the
// source one skill. Otherwise the root's direct folders, the usual skills
// containers and the folders plugin manifests list are searched, never
// below a skill already found; only when that finds nothing is the whole
// tree searched, five levels deep. Skills marked metadata.internal are
// left out, and a later skill with a name already found is dropped.
func Discover(snap *Snapshot) ([]Candidate, error) {
	d := discoverer{snap: snap, seen: map[string]bool{}, names: map[string]bool{}}
	dirs, plugins := pluginSkillDirs(snap)
	d.plugins = plugins
	if d.try("") && len(d.found) == 1 && d.found[0].Problem == ProblemNone {
		return d.found, nil
	}
	d.walk("", 1, 1)
	for _, dir := range discoverContainers {
		d.walk(dir, discoverContainerDepth, 1)
	}
	for _, dir := range dirs {
		d.walk(dir, 1, 1)
	}
	if len(d.found) == 0 {
		d.deep("", 0)
	}
	if len(d.found) == 0 {
		return nil, importError(ImportNoSkills, "")
	}
	return d.found, nil
}

func (d *discoverer) walk(dir string, maxDepth, depth int) {
	for _, child := range d.snap.children[dir] {
		if d.try(child) || depth >= maxDepth || discoverSkipped[path.Base(child)] {
			continue
		}
		d.walk(child, maxDepth, depth+1)
	}
}

func (d *discoverer) deep(dir string, depth int) {
	if depth > discoverFallbackDepth || dir != "" && d.try(dir) {
		return
	}
	for _, child := range d.snap.children[dir] {
		if !discoverSkipped[path.Base(child)] {
			d.deep(child, depth+1)
		}
	}
}

// try reports whether folder holds a SKILL.md, adding it the first time.
func (d *discoverer) try(folder string) bool {
	if !d.snap.has(joinImport(folder, "SKILL.md")) {
		return false
	}
	if d.seen[folder] {
		return true
	}
	d.seen[folder] = true
	c, keep := d.candidate(folder)
	key := strings.ToLower(c.Name)
	if !keep || d.names[key] {
		return true
	}
	if len(d.found) >= MaxImportSkills {
		d.snap.truncated = true
		return true
	}
	d.names[key] = true
	d.found = append(d.found, c)
	return true
}

func (d *discoverer) candidate(folder string) (Candidate, bool) {
	snap := d.snap
	data := snap.files[joinImport(folder, "SKILL.md")].data
	c := Candidate{Folder: folder, Name: importFolderName(snap, folder), Plugin: d.plugins[folder], TreeSHA: snap.trees[folder]}
	body := data
	front, bodyStart, err := parseFrontmatter(data)
	if err != nil {
		c.Problem = ProblemInvalidFront
	} else {
		if strings.EqualFold(front.metadata["internal"], "true") {
			return c, false
		}
		if name := front.field("name"); name != "" {
			c.Name = name
		}
		c.Description = front.field("description")
		c.Fields = front.displayFields()
		body = data[bodyStart:]
		if c.Description == "" {
			c.Problem = ProblemMissingDesc
		}
	}
	if len(body) > MaxBodyBytes {
		body, c.BodyTruncated = body[:MaxBodyBytes], true
	}
	c.Body = strings.ToValidUTF8(string(body), "")
	c.DirName = sanitizeImportName(c.Name)
	if c.DirName == "" {
		c.DirName = sanitizeImportName(importFolderName(snap, folder))
	}
	if c.DirName == "" && c.Problem == ProblemNone {
		c.Problem = ProblemInvalidName
	}
	d.collect(&c)
	return c, true
}

// collect lists the files of a skill's folder: repository plumbing is
// left out, and a link is copied as the file it names inside the folder
// or skipped.
func (d *discoverer) collect(c *Candidate) {
	snap := d.snap
	c.data = map[string]importFile{}
	for _, p := range snap.order {
		if !inFolder(c.Folder, p) {
			continue
		}
		rel := relInFolder(c.Folder, p)
		if skillPlumbing(rel) {
			continue
		}
		file, ok := snap.files[p]
		if !ok {
			if file, ok = snap.resolveLink(c.Folder, p); !ok {
				c.Skipped = append(c.Skipped, rel)
				continue
			}
		}
		c.data[rel] = file
		c.Files = append(c.Files, ImportFile{Path: rel, Size: int64(len(file.data)), Exec: file.exec})
		c.Size += int64(len(file.data))
		if file.exec {
			c.Executables++
		}
	}
	for _, p := range snap.rejected {
		if inFolder(c.Folder, p) && c.Problem == ProblemNone {
			c.Problem = ProblemInvalidPath
		}
	}
}

func skillPlumbing(rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		if skillSkippedNames[part] {
			return true
		}
	}
	return false
}

// importFolderName names a folder: its own name, or for the source root
// the repository, folder or archive name.
func importFolderName(snap *Snapshot, folder string) string {
	if folder != "" {
		return path.Base(folder)
	}
	src := snap.Source
	switch {
	case src.Kind == SourceGitHub && src.Subpath != "":
		return path.Base(src.Subpath)
	case src.Kind == SourceGitHub:
		return src.Repo
	}
	base := path.Base(strings.ReplaceAll(src.Path, "\\", "/"))
	if src.Kind == SourceZip {
		base = strings.TrimSuffix(base, path.Ext(base))
	}
	return base
}

type importManifest struct {
	Name     string          `json:"name"`
	Skills   []string        `json:"skills"`
	Source   json.RawMessage `json:"source"`
	Metadata struct {
		PluginRoot string `json:"pluginRoot"`
	} `json:"metadata"`
	Plugins []importManifest `json:"plugins"`
}

// pluginSkillDirs reads .claude-plugin/marketplace.json and plugin.json:
// the folders to search one level deep, and the plugin of each listed
// skill. Only "./" paths inside the source count.
func pluginSkillDirs(snap *Snapshot) ([]string, map[string]string) {
	var dirs []string
	groups := map[string]string{}
	add := func(base, name string, skills []string) {
		for _, raw := range skills {
			if dir, ok := manifestPath(base, raw); ok {
				dirs = append(dirs, parentImport(dir))
				if name != "" {
					groups[dir] = name
				}
			}
		}
		dirs = append(dirs, joinImport(base, "skills"))
	}
	var market importManifest
	if file, ok := snap.files[".claude-plugin/marketplace.json"]; ok && json.Unmarshal(file.data, &market) == nil {
		if root, ok := manifestPath("", valueOrDot(market.Metadata.PluginRoot)); ok {
			for _, plugin := range market.Plugins {
				source := "./"
				if len(plugin.Source) > 0 && json.Unmarshal(plugin.Source, &source) != nil {
					continue
				}
				if base, ok := manifestPath(root, source); ok {
					add(base, plugin.Name, plugin.Skills)
				}
			}
		}
	}
	var plugin importManifest
	if file, ok := snap.files[".claude-plugin/plugin.json"]; ok && json.Unmarshal(file.data, &plugin) == nil {
		add("", plugin.Name, plugin.Skills)
	}
	return dirs, groups
}

func valueOrDot(v string) string {
	if v == "" {
		return "./"
	}
	return v
}

// manifestPath joins a "./" manifest path to base, staying in the source.
func manifestPath(base, raw string) (string, bool) {
	if !strings.HasPrefix(raw, "./") {
		return "", false
	}
	p := path.Clean(joinImport(base, raw))
	if p == "." {
		return "", true
	}
	if p == ".." || strings.HasPrefix(p, "../") || strings.HasPrefix(p, "/") {
		return "", false
	}
	return p, true
}
