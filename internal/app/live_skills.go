package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

var errSkillsUnavailable = errors.New("skills service unavailable")

// installSkillActions wires the skills pages to the scanner, the provider
// config writers, the Quosparr skills trash and the system shell. Deleting
// moves to that trash (the system Trash from another volume); purging
// moves on to the system Trash, so nothing is erased. The diagnostics
// fixes go through the same trash and back SKILL.md up before editing it.
func installSkillActions(rt *liveApp, a *SkillActions) {
	home := rt.skillsEnv().Home
	trash := skills.NewTrash(platform.RunwayDir(home), home)
	system := mygo.Shell.TrashItem
	a.Query = rt.querySkills
	a.ReadBody = skills.ReadBody
	a.SetEnabled = rt.setSkillEnabled
	a.Delete = func(s skills.Skill, roots []skills.RootInfo) (bool, error) {
		_, inSystem, err := trash.MoveSkill(s, roots, time.Now(), system)
		return inSystem, err
	}
	a.DeleteLink = func(sum skills.Summary, path string) (bool, error) {
		_, inSystem, err := trash.MoveBrokenLink(sum, path, time.Now(), system)
		return inSystem, err
	}
	a.Transfer = func(req skills.TransferRequest) (skills.TransferResult, error) {
		return skills.TransferSkill(rt.skillsEnv(), req, system)
	}
	a.LiftNested = func(sum skills.Summary, s skills.Skill) (skills.LiftResult, error) {
		return trash.LiftNested(sum, s, time.Now(), system)
	}
	a.SuggestDescription = skills.SuggestDescription
	backup := platform.BackupPolicy{Dir: filepath.Join(platform.RunwayDir(home), "backups"), Prefix: "skill-md-", Ext: ".md", Keep: 10}
	a.AddDescription = func(s skills.Skill, description string) error {
		return skills.AddDescription(s, description, backup, time.Now())
	}
	a.OpenFolder = mygo.Shell.OpenPath
	a.Reveal = mygo.Shell.ShowItemInFolder
	a.Copy = mygo.Clipboard.WriteText
	a.Inspect = skills.Inspect
	a.TrashDir = trash.Dir
	a.ListTrash = trash.List
	a.Restore = trash.Restore
	a.Purge = func(id string) error { return trash.Purge(id, system) }
	a.EmptyTrash = func() (int, error) { return trash.Empty(system) }
	installSkillImport(rt, a, system)
}

// installSkillImport wires adding skills: GitHub is reached through the
// app's proxy only, a folder or zip is read where it is; the picked skills
// are written into user skills folders, and an update moves the old copy
// to the skills trash.
func installSkillImport(rt *liveApp, a *SkillActions, system func(string) error) {
	a.LoadImport = func(ctx context.Context, src skills.Source, progress func(done, total int64)) (*skills.Snapshot, []skills.Candidate, error) {
		var snap *skills.Snapshot
		var err error
		switch src.Kind {
		case skills.SourceGitHub:
			var g skills.GitHub
			if g, err = skillsGitHub(ctx); err == nil {
				g.Progress = progress
				snap, err = g.Load(ctx, src)
			}
		case skills.SourceZip:
			snap, err = skills.LoadZip(ctx, src.Path)
		default:
			snap, err = skills.LoadFolder(ctx, src.Path)
		}
		if err != nil {
			return nil, nil, err
		}
		found, err := skills.Discover(snap)
		return snap, found, err
	}
	a.PickPath = func(title string, folder bool) (string, error) {
		opts := mygo.OpenDialogOptions{Title: title, Directory: folder}
		if !folder {
			opts.Filters = []mygo.FileFilter{{Name: "ZIP", Extensions: []string{"zip", "skill"}}}
		}
		paths, err := mygo.Dialog.Open(opts)
		if err != nil || len(paths) == 0 {
			return "", err
		}
		return paths[0], nil
	}
	a.PrepareInstall = func(target skills.InstallTarget, sums []skills.Summary, snap *skills.Snapshot, picked []skills.Candidate) (skills.InstallPlan, error) {
		return skills.PrepareInstall(rt.skillsEnv(), target, sums, snap, picked)
	}
	a.Install = func(ctx context.Context, plan skills.InstallPlan) (skills.InstallResult, error) {
		return skills.Install(ctx, plan, time.Now())
	}
	a.CheckUpdate = func(ctx context.Context, sk skills.Skill) skills.UpdateCheck {
		g, err := skillsGitHub(ctx)
		if err != nil {
			return skills.UpdateCheck{Dir: sk.Dir, Err: err}
		}
		return g.CheckUpdates(ctx, rt.skillsEnv().Home, []string{sk.Dir})[0]
	}
	a.Update = func(ctx context.Context, req skills.UpdateRequest) (skills.UpdateResult, error) {
		g, err := skillsGitHub(ctx)
		if err != nil {
			return skills.UpdateResult{}, err
		}
		return skills.UpdateSkill(ctx, rt.skillsEnv(), g, req, system)
	}
}

// skillsGitHub uses the app's proxied client without cookies; a
// repository archive may take longer than an API call.
func skillsGitHub(ctx context.Context) (skills.GitHub, error) {
	client, err := netx.ClientFor(ctx, netx.WithoutCookies)
	if err != nil {
		return skills.GitHub{}, err
	}
	// A large repository's tarball is streamed through to reach its
	// skills; the add page shows the progress and can cancel it.
	c := *client
	c.Timeout = 30 * time.Minute
	return skills.GitHub{Client: &c}, nil
}

func (rt *liveApp) skillsEnv() skills.Env {
	env := skills.Env{
		CodexHome: rt.codexFolder(),
		ClaudeDir: rt.claudeFolder(),
		GrokHome:  rt.grokFolder(),
		GOOS:      runtime.GOOS,
		Getenv:    os.Getenv,
		Now:       time.Now(),
	}
	if rt.claudeMod != nil && rt.claudeMod.Store != nil {
		env.Home = rt.claudeMod.Store.HomeDir()
	}
	if env.Home == "" {
		env.Home, _ = os.UserHomeDir()
	}
	return env
}

// querySkills scans all three CLIs so copies in a shared directory
// (~/.agents/skills, ~/.claude/skills) know who else reads them.
func (rt *liveApp) querySkills(ctx context.Context) (map[store.Provider]skills.Summary, error) {
	env := rt.skillsEnv()
	cx, err := skills.ScanCodex(ctx, env)
	if err != nil {
		return nil, err
	}
	cl, err := skills.ScanClaude(ctx, env)
	if err != nil {
		return nil, err
	}
	gk, err := skills.ScanGrok(ctx, env)
	if err != nil {
		return nil, err
	}
	skills.MarkShared(&cx, &cl, &gk)
	return map[store.Provider]skills.Summary{
		store.ProviderCodex:  cx,
		store.ProviderClaude: cl,
		store.ProviderGrok:   gk,
	}, nil
}

// setSkillEnabled writes the provider's own switch: Codex [[skills.config]],
// Claude skillOverrides, Grok [skills].disabled; each backs up first.
func (rt *liveApp) setSkillEnabled(s skills.Skill, on bool) error {
	now := time.Now()
	switch s.Provider {
	case skills.ProviderCodex:
		if rt.codex == nil {
			return errSkillsUnavailable
		}
		return rt.codex.SetSkillEnabledFile(s.File, s.InvokeName, on, now)
	case skills.ProviderClaude:
		if rt.claudeMod == nil || rt.claudeMod.Store == nil {
			return errSkillsUnavailable
		}
		return rt.claudeMod.Store.SetSkillEnabledFile(s.Name, on, now)
	case skills.ProviderGrok:
		if rt.grokMod == nil || rt.grokMod.Store == nil {
			return errSkillsUnavailable
		}
		return rt.grokMod.Store.SetSkillEnabledFile(s.Name, on, now)
	}
	return errSkillsUnavailable
}
