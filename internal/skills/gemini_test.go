package skills

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func geminiTestEnv(t *testing.T) Env {
	t.Helper()
	home := t.TempDir()
	return Env{Home: home, GeminiHome: filepath.Join(home, "gemini-home"), GOOS: runtime.GOOS, Now: time.Unix(1_800_000_000, 0)}
}

func geminiSkills(summary Summary, name string) []Skill {
	var found []Skill
	for _, skill := range summary.Skills {
		if skill.Name == name {
			found = append(found, skill)
		}
	}
	return found
}

func geminiLink(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestGeminiDefaultsToDotGeminiAndListsOnlyExistingRoots(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home, Now: time.Unix(1_800_000_000, 0)}
	claudeWriteSkill(t, filepath.Join(home, ".gemini", "config", "skills", "alpha"), "alpha", "a")
	summary, err := ScanGemini(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Provider != ProviderGemini || len(summary.Roots) != 1 || summary.Roots[0].Label != filepath.Join("~", ".gemini", "config", "skills") {
		t.Fatalf("roots = %#v", summary.Roots)
	}
	alpha := claudeFindSkill(t, summary, "alpha", "")
	if alpha.State != StateOn || !alpha.Deletable || alpha.Toggleable || alpha.ToggleBlock != ReasonNoToggle || alpha.Scope != ScopeUser {
		t.Fatalf("alpha = %#v", alpha)
	}
	root, err := InstallRoot(env, TargetGemini)
	if err != nil || root != filepath.Join(home, ".gemini", "config", "skills") {
		t.Fatalf("InstallRoot = %q, %v", root, err)
	}
	if _, err := ScanGemini(context.Background(), Env{}); err == nil {
		t.Fatal("missing Home did not fail")
	}
}

func TestGeminiRootsBundledNestedAndFrontmatter(t *testing.T) {
	env := geminiTestEnv(t)
	g := env.GeminiHome
	claudeWriteSkill(t, filepath.Join(g, "config", "skills", "user"), "user", "u")
	claudeWriteSkill(t, filepath.Join(g, "skills", "cli-extra"), "cli-extra", "c")
	claudeWriteSkill(t, filepath.Join(g, "antigravity-cli", "skills", "cli-dir"), "cli-dir", "c")
	claudeWriteSkill(t, filepath.Join(g, "antigravity", "skills", "app-extra"), "app-extra", "a")
	claudeWriteSkill(t, filepath.Join(g, "antigravity", "global_skills", "app-global"), "app-global", "a")
	claudeWriteSkill(t, filepath.Join(g, "antigravity-cli", "builtin", "skills", "cli-builtin"), "cli-builtin", "b")
	claudeWriteSkill(t, filepath.Join(g, "antigravity", "builtin", "skills", "app-builtin"), "app-builtin", "b")
	claudeWriteSkill(t, filepath.Join(g, "config", "skills", "group", "inner"), "inner", "nested")
	claudeWriteFile(t, filepath.Join(g, "config", "skills", "nofront", "SKILL.md"), "no frontmatter\n")
	claudeWriteFile(t, filepath.Join(g, "config", "skills", "nodesc", "SKILL.md"), "---\nname: nodesc\n---\nbody\n")
	claudeWriteFile(t, filepath.Join(g, "config", "skills", "folder", "SKILL.md"), "---\nname: renamed\ndescription: d\n---\n")

	summary, err := ScanGemini(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Roots) != 7 {
		t.Fatalf("roots = %#v", summary.Roots)
	}
	for _, name := range []string{"user", "cli-extra", "cli-dir", "app-extra", "app-global"} {
		if skill := claudeFindSkill(t, summary, name, ""); skill.Scope != ScopeUser || !skill.Deletable || skill.State != StateOn {
			t.Fatalf("%s = %#v", name, skill)
		}
	}
	for _, name := range []string{"cli-builtin", "app-builtin"} {
		if skill := claudeFindSkill(t, summary, name, ""); skill.Scope != ScopeBundled || skill.Deletable || skill.Toggleable || skill.ToggleBlock != ReasonReadOnlyScope {
			t.Fatalf("%s = %#v", name, skill)
		}
	}
	claudeAssertNoSkill(t, summary, "inner")
	if skill := claudeFindSkill(t, summary, "nofront", ""); skill.State != StateNotLoaded || skill.Reason != ReasonInvalidFront {
		t.Fatalf("nofront = %#v", skill)
	}
	nodesc := claudeFindSkill(t, summary, "nodesc", "")
	if nodesc.State != StateOn || !claudeHasIssue(summary, IssueMissingDesc, nodesc.File) {
		t.Fatalf("nodesc = %#v", nodesc)
	}
	if renamed := claudeFindSkill(t, summary, "renamed", ""); renamed.InvokeName != "renamed" {
		t.Fatalf("renamed = %#v", renamed)
	}
}

func TestGeminiSymlinkedRootAndSkillAreListedOnce(t *testing.T) {
	env := geminiTestEnv(t)
	g := env.GeminiHome
	claudeWriteSkill(t, filepath.Join(g, "config", "skills", "shared"), "shared", "s")
	geminiLink(t, filepath.Join(g, "config", "skills"), filepath.Join(g, "antigravity", "skills"))
	external := filepath.Join(env.Home, ".agents", "skills", "linked")
	claudeWriteSkill(t, external, "linked", "l")
	geminiLink(t, external, filepath.Join(g, "skills", "linked"))
	geminiLink(t, external, filepath.Join(g, "antigravity", "global_skills", "linked"))

	summary, err := ScanGemini(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range summary.Roots {
		if root.Path == filepath.Join(g, "antigravity", "skills") {
			t.Fatalf("symlinked root scanned twice: %#v", summary.Roots)
		}
	}
	if got := geminiSkills(summary, "shared"); len(got) != 1 {
		t.Fatalf("shared = %#v", got)
	}
	linked := geminiSkills(summary, "linked")
	if len(linked) != 1 || linked[0].LinkPath != filepath.Join(g, "skills", "linked") || !linked[0].Removal.IsLink {
		t.Fatalf("linked = %#v", linked)
	}
	other := Summary{Provider: ProviderCodex, Skills: []Skill{{Name: "linked", RealFile: linked[0].RealFile}}}
	MarkShared(&summary, &other)
	if got := geminiSkills(summary, "linked")[0].SharedWith; len(got) != 1 || got[0] != ProviderCodex {
		t.Fatalf("SharedWith = %v", got)
	}
}

func TestGeminiShadowingFollowsClients(t *testing.T) {
	env := geminiTestEnv(t)
	g := env.GeminiHome
	user := claudeWriteSkill(t, filepath.Join(g, "config", "skills", "both"), "both", "u")
	claudeWriteSkill(t, filepath.Join(g, "skills", "both"), "both", "cli copy")
	claudeWriteSkill(t, filepath.Join(g, "antigravity", "global_skills", "both"), "both", "app copy")
	cli := claudeWriteSkill(t, filepath.Join(g, "skills", "split"), "split", "cli")
	app := claudeWriteSkill(t, filepath.Join(g, "antigravity", "global_skills", "split"), "split", "app")
	claudeWriteSkill(t, filepath.Join(g, "antigravity-cli", "builtin", "skills", "split"), "split", "cli builtin")
	claudeWriteSkill(t, filepath.Join(g, "antigravity", "builtin", "skills", "own"), "own", "app builtin")

	summary, err := ScanGemini(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range geminiSkills(summary, "both") {
		if skill.File == user {
			if skill.State != StateOn {
				t.Fatalf("user copy = %#v", skill)
			}
		} else if skill.State != StateOverridden || skill.OverriddenBy != user {
			t.Fatalf("copy = %#v", skill)
		}
	}
	for _, skill := range geminiSkills(summary, "split") {
		switch skill.File {
		case cli, app:
			if skill.State != StateOn {
				t.Fatalf("client winner = %#v", skill)
			}
		default:
			if skill.State != StateOverridden || skill.OverriddenBy != cli {
				t.Fatalf("builtin = %#v", skill)
			}
		}
	}
	if own := claudeFindSkill(t, summary, "own", ""); own.State != StateOn {
		t.Fatalf("own = %#v", own)
	}
}

func TestGeminiPluginsFollowConfigState(t *testing.T) {
	env := geminiTestEnv(t)
	g := env.GeminiHome
	claudeWriteFile(t, filepath.Join(g, "config", "plugins", "on", "plugin.json"), `{"name":"on","version":"1.2.3"}`)
	claudeWriteSkill(t, filepath.Join(g, "config", "plugins", "on", "skills", "dup"), "dup", "plugin")
	claudeWriteSkill(t, filepath.Join(g, "config", "plugins", "off", "skills", "quiet"), "quiet", "plugin")
	claudeWriteSkill(t, filepath.Join(g, "config", "skills", "dup"), "dup", "user")
	claudeWriteFile(t, filepath.Join(g, "config", "config.json"), `{"plugins":{"off":{"enabled":false},"on":{"enabled":true}},"other":1}`)

	summary, err := ScanGemini(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	on := claudeFindSkill(t, summary, "dup", "on")
	if on.Scope != ScopePlugin || on.InvokeName != "on:dup" || on.State != StateOn || on.Deletable || on.Toggleable || on.ToggleBlock != ReasonPluginCollection {
		t.Fatalf("plugin skill = %#v", on)
	}
	if on.Origin.Version != "1.2.3" {
		t.Fatalf("plugin origin = %#v", on.Origin)
	}
	if user := claudeFindSkill(t, summary, "dup", ""); user.State != StateOn {
		t.Fatalf("user skill shadowed by plugin: %#v", user)
	}
	if quiet := claudeFindSkill(t, summary, "quiet", "off"); quiet.State != StateOff || quiet.Reason != ReasonPluginOff {
		t.Fatalf("disabled plugin skill = %#v", quiet)
	}

	claudeWriteFile(t, filepath.Join(g, "config", "config.json"), "{")
	if _, err := ScanGemini(context.Background(), env); err == nil {
		t.Fatal("invalid config.json accepted")
	}
}

func TestGeminiTransferTargetIsConfigSkills(t *testing.T) {
	env := geminiTestEnv(t)
	env.ClaudeDir = filepath.Join(env.Home, "claude")
	claudeWriteSkill(t, filepath.Join(env.ClaudeDir, "skills", "move-me"), "move-me", "m")
	claude, err := ScanClaude(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	result, err := TransferSkill(env, TransferRequest{Skill: claudeFindSkill(t, claude, "move-me", ""), Target: ProviderGemini}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(env.GeminiHome, "config", "skills", "move-me"); result.Path != want {
		t.Fatalf("Path = %q, want %q", result.Path, want)
	}
	summary, err := ScanGemini(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	claudeFindSkill(t, summary, "move-me", "")
}
