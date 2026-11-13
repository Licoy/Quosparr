package skills

import (
	"context"
	"path/filepath"
)

// geminiClient says which Antigravity client loads a root: the desktop App,
// the agy CLI, or both.
type geminiClient uint8

const (
	geminiApp geminiClient = 1 << iota
	geminiCLI
	geminiBoth = geminiApp | geminiCLI
)

type geminiRoot struct {
	info    RootInfo
	clients geminiClient
}

// ScanGemini reports the global skills the Antigravity desktop App and agy
// CLI load from GeminiHome (~/.gemini). Both clients read config/skills; the
// CLI also reads skills and antigravity-cli/skills, the App antigravity/skills
// and antigravity/global_skills. Each client ships read-only builtin skills,
// and installed plugins live under config/plugins/<name>/skills. Only
// <root>/<name>/SKILL.md is loaded. A root reached twice through symlinks is
// scanned once, and a skill file reached twice is listed once. Neither client
// has a per-skill switch, so no Gemini skill is toggleable here; a plugin is
// switched as a whole in config/config.json. Precedence between roots is not
// documented: earlier roots in the order above win same-name skills of the
// same client.
func ScanGemini(ctx context.Context, env Env) (Summary, error) {
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}
	env, err := normalizeEnv(env)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Provider: ProviderGemini, ScannedAt: env.Now}
	clients := map[string]geminiClient{}
	seen := map[string]bool{}
	for _, root := range geminiRoots(env) {
		if !root.info.Exists {
			continue
		}
		real, err := filepath.EvalSymlinks(root.info.Path)
		if err != nil {
			continue
		}
		if real = filepath.Clean(real); seen[real] {
			continue
		}
		seen[real] = true
		summary.Roots = append(summary.Roots, root.info)
		found, issues, err := scanGeminiRoot(ctx, env, root.info)
		if err != nil {
			return Summary{}, err
		}
		for _, skill := range found {
			clients[skill.File] = root.clients
		}
		summary.Skills = append(summary.Skills, found...)
		summary.Issues = append(summary.Issues, issues...)
	}
	if err := scanGeminiPlugins(ctx, env, &summary); err != nil {
		return Summary{}, err
	}
	summary.Skills = dedupeSkillsByRealFile(summary.Skills)
	shadowGeminiSkills(summary.Skills, clients)
	newOriginScanner(env).fill(summary.Skills)
	finishSummary(&summary)
	return summary, nil
}

func geminiRoots(env Env) []geminiRoot {
	home := env.GeminiHome
	root := func(clients geminiClient, scope Scope, parts ...string) geminiRoot {
		return geminiRoot{info: newRoot(env, filepath.Join(append([]string{home}, parts...)...), scope), clients: clients}
	}
	return []geminiRoot{
		root(geminiBoth, ScopeUser, "config", "skills"),
		root(geminiCLI, ScopeUser, "skills"),
		root(geminiCLI, ScopeUser, "antigravity-cli", "skills"),
		root(geminiApp, ScopeUser, "antigravity", "skills"),
		root(geminiApp, ScopeUser, "antigravity", "global_skills"),
		root(geminiCLI, ScopeBundled, "antigravity-cli", "builtin", "skills"),
		root(geminiApp, ScopeBundled, "antigravity", "builtin", "skills"),
	}
}

func scanGeminiRoot(ctx context.Context, env Env, root RootInfo) ([]Skill, []Issue, error) {
	options := walkOptions{MaxDirs: 2000, MaxEntries: 20000, SkipHidden: true, FollowLinks: true, DirectChildren: true}
	found, issues, err := scanRoot(ctx, env, ProviderGemini, root, options)
	if err != nil {
		return nil, nil, err
	}
	for i := range found {
		found[i].Toggleable = false
		found[i].ToggleBlock = ReasonNoToggle
		if root.Scope == ScopeBundled {
			found[i].ToggleBlock = ReasonReadOnlyScope
		}
	}
	sortGrokSkills(found)
	return found, issues, nil
}

// shadowGeminiSkills marks a skill overridden once every client that loads
// it already has a same-name winner from an earlier root.
func shadowGeminiSkills(skills []Skill, clients map[string]geminiClient) {
	winners := map[geminiClient]map[string]string{geminiApp: {}, geminiCLI: {}}
	for i := range skills {
		skill := &skills[i]
		if skill.Scope == ScopePlugin || skill.State != StateOn {
			continue
		}
		loadedBy := clients[skill.File]
		winner, open := "", false
		for _, client := range []geminiClient{geminiApp, geminiCLI} {
			if loadedBy&client == 0 {
				continue
			}
			if file, ok := winners[client][skill.Name]; ok {
				if winner == "" {
					winner = file
				}
				continue
			}
			open = true
			winners[client][skill.Name] = skill.File
		}
		if !open && winner != "" {
			skill.State, skill.Reason, skill.OverriddenBy = StateOverridden, ReasonShadowed, winner
		}
	}
}
