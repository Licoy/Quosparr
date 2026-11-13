package skills

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

var errHomeRequired = errors.New("skills: home directory is required")

func normalizeEnv(env Env) (Env, error) {
	if strings.TrimSpace(env.Home) == "" {
		return Env{}, errHomeRequired
	}
	home, err := filepath.Abs(env.Home)
	if err != nil {
		return Env{}, err
	}
	env.Home = filepath.Clean(home)
	if env.CodexHome == "" {
		env.CodexHome = filepath.Join(env.Home, ".codex")
	}
	if env.ClaudeDir == "" {
		env.ClaudeDir = filepath.Join(env.Home, ".claude")
	}
	if env.GrokHome == "" {
		env.GrokHome = filepath.Join(env.Home, ".grok")
	}
	if env.GeminiHome == "" {
		env.GeminiHome = filepath.Join(env.Home, ".gemini")
	}
	for _, path := range []*string{&env.CodexHome, &env.ClaudeDir, &env.GrokHome, &env.GeminiHome} {
		abs, err := filepath.Abs(*path)
		if err != nil {
			return Env{}, err
		}
		*path = filepath.Clean(abs)
	}
	if env.GOOS == "" {
		env.GOOS = runtime.GOOS
	}
	if env.Now.IsZero() {
		env.Now = time.Now()
	}
	if env.Getenv == nil {
		env.Getenv = func(string) string { return "" }
	}
	return env, nil
}

func newRoot(env Env, path string, scope Scope) RootInfo {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	path = filepath.Clean(path)
	_, err = os.Stat(path)
	return RootInfo{Path: path, Label: rootLabel(env.Home, path), Scope: scope, Exists: err == nil}
}

func rootLabel(home, path string) string {
	rel, err := filepath.Rel(home, path)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		if rel == "." {
			return "~"
		}
		return filepath.Join("~", rel)
	}
	return path
}

// MarkShared records providers that resolve to the same skill file.
func MarkShared(summaries ...*Summary) {
	type ref struct {
		provider Provider
		skill    *Skill
	}
	byFile := make(map[string][]ref)
	for _, summary := range summaries {
		if summary == nil {
			continue
		}
		for i := range summary.Skills {
			s := &summary.Skills[i]
			s.SharedWith = nil
			if s.RealFile != "" {
				byFile[filepath.Clean(s.RealFile)] = append(byFile[filepath.Clean(s.RealFile)], ref{summary.Provider, s})
			}
		}
	}
	for _, refs := range byFile {
		for _, current := range refs {
			seen := map[Provider]bool{current.provider: true}
			for _, other := range refs {
				if !seen[other.provider] {
					current.skill.SharedWith = append(current.skill.SharedWith, other.provider)
					seen[other.provider] = true
				}
			}
			sort.Slice(current.skill.SharedWith, func(i, j int) bool { return current.skill.SharedWith[i] < current.skill.SharedWith[j] })
		}
	}
}

func finishSummary(summary *Summary) {
	if summary == nil {
		return
	}
	for i := range summary.Roots {
		summary.Roots[i].Count = 0
		for _, skill := range summary.Skills {
			if filepath.Clean(skill.Root) == filepath.Clean(summary.Roots[i].Path) {
				summary.Roots[i].Count++
			}
		}
	}
	// A shadowed skill already says which one wins; only names the CLI
	// loads twice are worth a diagnostics entry.
	first := make(map[string]Skill)
	for _, skill := range summary.Skills {
		if skill.State == StateOverridden {
			continue
		}
		if previous, ok := first[skill.InvokeName]; ok && previous.RealFile != skill.RealFile {
			summary.Issues = append(summary.Issues, Issue{Kind: IssueDuplicateName, Path: skill.File, Name: skill.InvokeName, Detail: previous.File})
		} else if !ok {
			first[skill.InvokeName] = skill
		}
	}
	setRemovals(summary)
	markFixes(summary)
}
