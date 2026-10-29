package skills

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"time"
)

// Updatable reports a user skill Quosparr installed from GitHub that is
// still a plain folder where it was installed.
func Updatable(sk Skill) bool {
	return sk.Origin.From == OriginQuosparr && sk.Origin.Installer == string(SourceGitHub) &&
		CanMoveSkill(sk) && sk.LinkPath == ""
}

// UpdateState is what a check found for a skill installed from GitHub.
type UpdateState string

// UpdateState values.
const (
	UpdateCurrent   UpdateState = "current"
	UpdateAvailable UpdateState = "available"
)

// UpdateCheck is the result for one installed folder.
type UpdateCheck struct {
	Dir    string
	State  UpdateState
	Commit string // the latest commit of the recorded ref
	Err    error
}

// CheckUpdates asks GitHub once per repository and ref whether the
// recorded skills changed: a new commit whose tree for the skill's folder
// differs from the recorded one, or is unknown.
func (g GitHub) CheckUpdates(ctx context.Context, home string, dirs []string) []UpdateCheck {
	records, err := LoadRecords(home)
	out := make([]UpdateCheck, 0, len(dirs))
	type repoKey struct{ owner, repo, ref string }
	type latest struct {
		commit string
		trees  map[string]string
		err    error
	}
	cache := map[repoKey]*latest{}
	for _, dir := range dirs {
		check := UpdateCheck{Dir: dir}
		rec, ok := records[recordKey(dir)]
		owner, repo, valid := recordRepo(rec)
		switch {
		case err != nil:
			check.Err = err
		case !ok || rec.Kind != SourceGitHub || !valid:
			check.Err = importError(ImportNotUpdatable, "")
		}
		if check.Err != nil {
			out = append(out, check)
			continue
		}
		key := repoKey{owner, repo, rec.Ref}
		got := cache[key]
		if got == nil {
			got = &latest{}
			got.commit, got.err = g.Latest(ctx, owner, repo, rec.Ref)
			cache[key] = got
		}
		check.Commit, check.Err = got.commit, got.err
		if check.Err == nil {
			check.State, check.Err = g.compareTree(ctx, key.owner, key.repo, rec, got.commit, &got.trees)
		}
		out = append(out, check)
	}
	return out
}

func (g GitHub) compareTree(ctx context.Context, owner, repo string, rec Record, commit string, trees *map[string]string) (UpdateState, error) {
	if commit == rec.Commit {
		return UpdateCurrent, nil
	}
	if rec.TreeSHA == "" {
		return UpdateAvailable, nil
	}
	if *trees == nil {
		got, err := g.trees(ctx, owner, repo, commit)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return UpdateAvailable, nil
		}
		*trees = got.folders
	}
	switch sha := (*trees)[rec.SkillPath]; {
	case sha == "":
		return "", importError(ImportMissing, rec.SkillPath)
	case sha == rec.TreeSHA:
		return UpdateCurrent, nil
	}
	return UpdateAvailable, nil
}

// UpdateRequest names an installed skill and the scan it came from.
type UpdateRequest struct {
	Skill Skill
	Roots []RootInfo
	// Force replaces a copy edited since it was installed; the edited copy
	// goes to the skills trash like the unedited one would.
	Force bool
}

// UpdateResult says how an update went.
type UpdateResult struct {
	Path             string
	Unchanged        bool // the source had nothing new; only the record moved on
	OldInSystemTrash bool // the previous version went to the system Trash
}

// UpdateSkill replaces an installed skill with its source's latest
// version. The new files are written next to it in a hidden folder, the
// old folder moves to the skills trash, and the new one takes its name;
// if that last step fails the old folder is put back.
func UpdateSkill(ctx context.Context, input Env, g GitHub, req UpdateRequest, systemTrash func(string) error) (result UpdateResult, err error) {
	env, err := normalizeEnv(input)
	if err != nil {
		return result, err
	}
	sk := req.Skill
	result.Path = sk.Dir
	records, err := LoadRecords(env.Home)
	if err != nil {
		return result, err
	}
	rec, ok := records[recordKey(sk.Dir)]
	owner, repo, valid := recordRepo(rec)
	if !Updatable(sk) || !ok || rec.Kind != SourceGitHub || !valid {
		return result, importError(ImportNotUpdatable, "")
	}
	if !req.Force {
		if hash, err := installedHash(sk.Dir); err != nil || hash != rec.ContentHash {
			return result, importError(ImportModified, "")
		}
	}
	snap, err := g.Load(ctx, Source{Kind: SourceGitHub, Owner: owner, Repo: repo, Ref: rec.Ref, Subpath: rec.SkillPath})
	if err != nil {
		return result, err
	}
	found, err := Discover(snap)
	if err != nil || found[0].Folder != "" || found[0].Problem != ProblemNone {
		return result, importError(ImportMissing, rec.SkillPath)
	}
	next := found[0]
	rec.Commit, rec.TreeSHA = snap.Commit, next.TreeSHA
	if hash := contentHash(next.data); hash == rec.ContentHash {
		result.Unchanged = true
		return result, saveRecord(env.Home, sk.Dir, rec, env.Now)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.OldInSystemTrash, err = replaceSkill(env, sk, req.Roots, next, systemTrash)
	if err != nil {
		return result, err
	}
	rec.ContentHash = contentHash(next.data)
	return result, saveRecord(env.Home, sk.Dir, rec, env.Now)
}

func saveRecord(home, dir string, rec Record, now time.Time) error {
	rec.UpdatedAt = now.UTC()
	return editRecords(home, func(all map[string]Record) { all[recordKey(dir)] = rec })
}

func replaceSkill(env Env, sk Skill, roots []RootInfo, next Candidate, systemTrash func(string) error) (bool, error) {
	parent, name := filepath.Dir(sk.Dir), filepath.Base(sk.Dir)
	frozen, err := installRootInfo(parent)
	if err != nil || frozen == nil {
		return false, importError(ImportUnsafeRoot, parent)
	}
	root, err := openInstallRoot(parent, frozen)
	if err != nil {
		return false, err
	}
	defer root.Close()
	staging, err := stagingName()
	if err != nil {
		return false, importError(ImportWrite, "")
	}
	created, err := writeSkill(root, staging, next)
	if err != nil {
		return false, err
	}
	trash := NewTrash(filepath.Join(env.Home, ".quosparr"), env.Home)
	entry, inSystem, err := trash.MoveSkill(sk, roots, env.Now, systemTrash)
	if err != nil {
		return false, errors.Join(err, created.cleanup())
	}
	if err := root.Rename(staging, name); err != nil {
		err = importError(ImportWrite, "")
		if !inSystem {
			_, restoreErr := trash.Restore(entry.ID)
			err = errors.Join(err, restoreErr)
		}
		return inSystem, errors.Join(err, created.cleanup())
	}
	return inSystem, nil
}

func stagingName() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return ".quosparr-update-" + hex.EncodeToString(b), nil
}
