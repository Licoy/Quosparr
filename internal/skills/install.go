package skills

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// InstallTarget is a user skills folder an imported skill can go into.
type InstallTarget string

// InstallTarget values.
const (
	TargetCodex  InstallTarget = "codex"  // $CODEX_HOME/skills
	TargetClaude InstallTarget = "claude" // <Claude config>/skills
	TargetGrok   InstallTarget = "grok"   // $GROK_HOME/skills
	TargetShared InstallTarget = "shared" // ~/.agents/skills, which Codex and Grok both read
)

// InstallTargets lists the targets in the order to offer them.
func InstallTargets() []InstallTarget {
	return []InstallTarget{TargetCodex, TargetClaude, TargetGrok, TargetShared}
}

// InstallRoot is the folder a target writes into.
func InstallRoot(input Env, target InstallTarget) (string, error) {
	env, err := normalizeEnv(input)
	if err != nil {
		return "", err
	}
	if target == TargetShared {
		return filepath.Join(env.Home, ".agents", "skills"), nil
	}
	return transferTargetRoot(env, Provider(target))
}

// targetReaders are the CLIs that load a target when no scan says more.
func targetReaders(target InstallTarget) []Provider {
	if target == TargetShared {
		return []Provider{ProviderCodex, ProviderGrok}
	}
	return []Provider{Provider(target)}
}

// InstallStatus is what installing a skill into a target would do.
type InstallStatus string

// InstallStatus values.
const (
	InstallAdd       InstallStatus = "add"
	InstallExists    InstallStatus = "exists"    // a folder of that name is there and is never overwritten
	InstallInstalled InstallStatus = "installed" // the same skill from the same source; update it from its page
	InstallInvalid   InstallStatus = "invalid"   // the folder name is reserved here
)

// InstallClash is a loaded skill with the same name in a CLI that reads
// the target.
type InstallClash struct {
	Provider Provider
	File     string
}

// InstallItem is one skill of a plan.
type InstallItem struct {
	Folder  string // the skill's folder in the source
	Name    string
	DirName string
	Path    string // the folder it would be installed as
	Status  InstallStatus
	Clashes []InstallClash
}

// InstallPlan is what installing the picked skills into one target does.
type InstallPlan struct {
	Target  InstallTarget
	Root    string
	Readers []Provider // the CLIs that load skills from Root
	Items   []InstallItem
	home    string
	root    fs.FileInfo // nil when Root did not exist
	source  Source
	commit  string
	skills  map[string]Candidate
}

// PrepareInstall checks the picked skills against a target. sums are the
// latest scans, used to name the CLIs that read the target and the skills
// they already load under the same names.
func PrepareInstall(input Env, target InstallTarget, sums []Summary, snap *Snapshot, picked []Candidate) (InstallPlan, error) {
	env, err := normalizeEnv(input)
	if err != nil {
		return InstallPlan{}, err
	}
	root, err := InstallRoot(env, target)
	if err != nil {
		return InstallPlan{}, err
	}
	if snap == nil || len(picked) == 0 || len(picked) > MaxImportSkills {
		return InstallPlan{}, importError(ImportNoSkills, "")
	}
	plan := InstallPlan{Target: target, Root: root, home: env.Home, source: snap.Source, commit: snap.Commit, skills: map[string]Candidate{}}
	if plan.root, err = installRootInfo(root); err != nil {
		return InstallPlan{}, err
	}
	plan.Readers = installReaders(target, root, sums)
	records, _ := LoadRecords(env.Home)
	names := map[string]bool{}
	for _, c := range picked {
		if c.Problem != ProblemNone || c.data == nil || names[strings.ToLower(c.DirName)] {
			return InstallPlan{}, importError(ImportInvalidPath, c.Folder)
		}
		names[strings.ToLower(c.DirName)] = true
		item := InstallItem{Folder: c.Folder, Name: c.Name, DirName: c.DirName, Path: filepath.Join(root, c.DirName), Status: InstallAdd}
		reserved := Provider(target)
		if target == TargetShared {
			reserved = ProviderCodex
		}
		switch _, err := os.Lstat(item.Path); {
		case !validTransferName(c.DirName, reserved):
			item.Status = InstallInvalid
		case err == nil && sameSource(records[recordKey(item.Path)], snap.Source, c.Folder):
			item.Status = InstallInstalled
		case err == nil || !errors.Is(err, fs.ErrNotExist):
			item.Status = InstallExists
		}
		item.Clashes = installClashes(sums, plan.Readers, item, c.Name)
		plan.Items = append(plan.Items, item)
		plan.skills[c.Folder] = c
	}
	return plan, nil
}

// installRootInfo freezes the identity of an existing target folder; a
// link or a file in its place is refused.
func installRootInfo(root string) (fs.FileInfo, error) {
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, info) {
		return nil, importError(ImportUnsafeRoot, root)
	}
	return info, nil
}

func installReaders(target InstallTarget, root string, sums []Summary) []Provider {
	var readers []Provider
	for _, sum := range sums {
		for _, r := range sum.Roots {
			if r.Scope == ScopeUser && filepath.Clean(r.Path) == filepath.Clean(root) && !slices.Contains(readers, sum.Provider) {
				readers = append(readers, sum.Provider)
			}
		}
	}
	if len(readers) == 0 {
		return targetReaders(target)
	}
	return readers
}

func installClashes(sums []Summary, readers []Provider, item InstallItem, name string) []InstallClash {
	var clashes []InstallClash
	for _, sum := range sums {
		if !slices.Contains(readers, sum.Provider) {
			continue
		}
		for _, sk := range sum.Skills {
			if sk.State == StateNotLoaded || !strings.EqualFold(sk.InvokeName, name) || filepath.Clean(sk.Dir) == filepath.Clean(item.Path) {
				continue
			}
			clashes = append(clashes, InstallClash{Provider: sum.Provider, File: sk.File})
		}
	}
	return clashes
}

func sameSource(rec Record, src Source, folder string) bool {
	if rec.Kind != src.Kind {
		return false
	}
	if src.Kind == SourceGitHub {
		return strings.EqualFold(rec.Repository, src.Repository()) && rec.SkillPath == joinImport(src.Subpath, folder)
	}
	return rec.Source == src.Path
}

// InstallOutcome is how one skill went.
type InstallOutcome struct {
	Folder, Name, Path string
	Err                error
}

// InstallResult reports each skill a plan added. RecordErr is set when the
// skills were written but the install record could not be.
type InstallResult struct {
	Items     []InstallOutcome
	RecordErr error
}

// Install writes the plan's new skills. Each is written into a fresh
// folder with SKILL.md last, so no CLI sees it half written, and a failed
// one is removed again; existing folders are never touched.
func Install(ctx context.Context, plan InstallPlan, now time.Time) (InstallResult, error) {
	var result InstallResult
	if plan.Root == "" || plan.skills == nil {
		return result, importError(ImportNoSkills, "")
	}
	root, err := openInstallRoot(plan.Root, plan.root)
	if err != nil {
		return result, err
	}
	defer root.Close()
	records := map[string]Record{}
	for _, item := range plan.Items {
		if item.Status != InstallAdd {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		c := plan.skills[item.Folder]
		out := InstallOutcome{Folder: item.Folder, Name: item.Name, Path: item.Path}
		if _, out.Err = writeSkill(root, item.DirName, c); out.Err == nil {
			records[recordKey(item.Path)] = plan.record(c, now)
		}
		result.Items = append(result.Items, out)
	}
	if len(records) > 0 {
		result.RecordErr = editRecords(plan.home, func(all map[string]Record) {
			for dir, rec := range records {
				all[dir] = rec
			}
		})
	}
	return result, nil
}

func (p InstallPlan) record(c Candidate, now time.Time) Record {
	rec := Record{Kind: p.source.Kind, ContentHash: contentHash(c.data), InstalledAt: now.UTC(), UpdatedAt: now.UTC()}
	if p.source.Kind == SourceGitHub {
		rec.Repository, rec.Ref, rec.Commit, rec.TreeSHA = p.source.Repository(), p.source.Ref, p.commit, c.TreeSHA
		rec.SkillPath = joinImport(p.source.Subpath, c.Folder)
	} else {
		rec.Source = p.source.Path
	}
	return rec
}

// openInstallRoot creates a missing target folder and opens it, refusing
// one that was replaced since the plan was made.
func openInstallRoot(dir string, frozen fs.FileInfo) (*os.Root, error) {
	current, err := installRootInfo(dir)
	if err != nil {
		return nil, err
	}
	if frozen != nil && (current == nil || !os.SameFile(frozen, current)) {
		return nil, importError(ImportChanged, dir)
	}
	if current == nil {
		if err := ensureTransferRoot(dir); err != nil {
			return nil, importError(ImportUnsafeRoot, dir)
		}
		if current, err = installRootInfo(dir); err != nil || current == nil {
			return nil, importError(ImportUnsafeRoot, dir)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, importError(ImportWrite, "")
	}
	if opened, err := root.Stat("."); err != nil || !os.SameFile(opened, current) {
		root.Close()
		return nil, importError(ImportChanged, dir)
	}
	return root, nil
}

// writeSkill creates name in root and writes the skill into it, SKILL.md
// last. It returns what it created; on failure that is removed again.
func writeSkill(root *os.Root, name string, c Candidate) (_ *transferTarget, err error) {
	if err := root.Mkdir(name, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, importError(ImportExists, name)
		}
		return nil, importError(ImportWrite, "")
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, importError(ImportWrite, "")
	}
	created := &transferTarget{root: root, name: name, created: []transferCreated{{path: name, info: info}}}
	defer func() {
		if err != nil {
			err = errors.Join(err, created.cleanup())
		}
	}()
	dest, err := root.OpenRoot(name)
	if err != nil {
		return nil, importError(ImportWrite, "")
	}
	defer dest.Close()
	if opened, err := dest.Stat("."); err != nil || !os.SameFile(opened, info) {
		return nil, importError(ImportChanged, name)
	}
	for _, dir := range skillDirs(c.Files) {
		if err := dest.Mkdir(filepath.FromSlash(dir), 0o755); err != nil {
			return nil, importError(ImportWrite, "")
		}
		if err := recordTransferCreated(dest, created, filepath.FromSlash(dir)); err != nil {
			return nil, importError(ImportWrite, "")
		}
	}
	files := slices.Clone(c.Files)
	sort.SliceStable(files, func(i, j int) bool { return files[j].Path == "SKILL.md" && files[i].Path != "SKILL.md" })
	for _, f := range files {
		if err := writeImportFile(dest, created, f.Path, c.data[f.Path]); err != nil {
			return nil, err
		}
	}
	return created, nil
}

// skillDirs are the folders the files need, parents first.
func skillDirs(files []ImportFile) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, f := range files {
		for dir := path.Dir(f.Path); dir != "."; dir = path.Dir(dir) {
			if seen[dir] {
				break
			}
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func writeImportFile(dest *os.Root, created *transferTarget, rel string, file importFile) error {
	mode := os.FileMode(0o644)
	if file.exec {
		mode = 0o755
	}
	name := filepath.FromSlash(rel)
	out, err := dest.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return importError(ImportWrite, "")
	}
	info, statErr := out.Stat()
	if statErr == nil {
		created.created = append(created.created, transferCreated{path: filepath.Join(created.name, name), info: info})
	}
	_, writeErr := out.Write(file.data)
	if err := errors.Join(statErr, writeErr, out.Sync(), out.Close()); err != nil {
		return importError(ImportWrite, "")
	}
	return nil
}
