package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

var pluginManifestDirs = map[Provider][]string{
	ProviderCodex:  {".codex-plugin", ".claude-plugin", ".grok-plugin", ".cursor-plugin"},
	ProviderClaude: {".claude-plugin", ".codex-plugin", ".grok-plugin", ".cursor-plugin"},
	ProviderGrok:   {".grok-plugin", ".claude-plugin", ".codex-plugin", ".cursor-plugin"},
	ProviderGemini: {"", ".claude-plugin", ".codex-plugin", ".grok-plugin", ".cursor-plugin"},
}

type originPluginCache struct {
	env Env

	manifests map[string]cachedPluginManifest
	claude    cachedClaudeOriginFiles
	grok      cachedGrokRegistry
}

type cachedPluginManifest struct {
	manifest originPluginManifest
	ok       bool
}

type originPluginManifest struct {
	Name       string          `json:"name"`
	Skills     json.RawMessage `json:"skills"`
	Homepage   string          `json:"homepage"`
	Repository json.RawMessage `json:"repository"`
	Version    string          `json:"version"`
}

func newOriginPluginCache(env Env) *originPluginCache {
	return &originPluginCache{env: env, manifests: make(map[string]cachedPluginManifest)}
}

func (c *originPluginCache) candidate(skill Skill) originCandidate {
	if c == nil || skill.RealFile == "" || skill.Scope != ScopePlugin && skill.Plugin == "" {
		return originCandidate{}
	}
	root, manifest, ok := c.pluginRoot(skill)
	candidate := originCandidate{from: OriginPlugin}
	if ok {
		candidate = pluginManifestCandidate(manifest, root, cleanEvaluatedPath(filepath.Dir(skill.RealFile)))
	}
	if root == "" {
		return candidate
	}
	switch skill.Provider {
	case ProviderClaude:
		return c.claudeCandidate(skill, root, candidate)
	case ProviderCodex:
		return c.codexCandidate(skill, root, candidate)
	case ProviderGrok:
		return c.grokCandidate(skill, root, candidate)
	default:
		return candidate
	}
}

func (c *originPluginCache) pluginRoot(skill Skill) (string, originPluginManifest, bool) {
	if strings.TrimSpace(skill.Root) == "" {
		return "", originPluginManifest{}, false
	}
	start := cleanEvaluatedPath(filepath.Dir(skill.RealFile))
	boundary := cleanEvaluatedPath(skill.Root)
	if skill.Scope == ScopePlugin {
		boundary = filepath.Dir(boundary)
	}
	for dir := start; within(boundary, dir); dir = filepath.Dir(dir) {
		for _, marker := range pluginManifestDirs[skill.Provider] {
			manifest, ok := c.pluginManifest(filepath.Join(dir, marker, "plugin.json"))
			if ok {
				return dir, manifest, true
			}
		}
		if dir == boundary || filepath.Dir(dir) == dir {
			break
		}
	}
	if skill.Scope == ScopePlugin {
		return boundary, originPluginManifest{}, false
	}
	return "", originPluginManifest{}, false
}

func cleanEvaluatedPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func (c *originPluginCache) pluginManifest(path string) (originPluginManifest, bool) {
	path = filepath.Join(cleanEvaluatedPath(filepath.Dir(path)), filepath.Base(path))
	if cached, ok := c.manifests[path]; ok {
		return cached.manifest, cached.ok
	}
	data, err := readOriginRegular(path)
	var manifest originPluginManifest
	ok := err == nil && json.Unmarshal(data, &manifest) == nil
	c.manifests[path] = cachedPluginManifest{manifest: manifest, ok: ok}
	return manifest, ok
}

func pluginManifestCandidate(manifest originPluginManifest, root, skillDir string) originCandidate {
	repository := repositoryValue(manifest.Repository)
	repo, _, _ := NormalizeRepoURL(repository)
	ref, prefix := "", ""
	tree := false
	if treeRepo, treeRef, treePath := parseRepoTreeURL(repository); treeRepo != "" {
		repo = treeRepo
		ref, prefix = treeRef, treePath
		tree = true
	}
	candidate := originCandidate{
		from: OriginPlugin, repo: repo, ref: ref,
		homepage: normalizeHomepage(manifest.Homepage), version: strings.TrimSpace(manifest.Version),
	}
	if tree {
		if rel, err := filepath.Rel(root, skillDir); err == nil {
			candidate.skillPath = joinOriginPath(prefix, filepath.ToSlash(rel))
		}
	}
	return candidate
}

func repositoryValue(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	var object struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return strings.TrimSpace(object.URL)
	}
	return ""
}

func joinOriginPath(parts ...string) string {
	var clean []string
	for _, part := range parts {
		raw := strings.TrimSpace(part)
		if raw == "" || raw == "." {
			continue
		}
		part = cleanOriginSkillPath(raw)
		if part == "" {
			return ""
		}
		clean = append(clean, part)
	}
	return cleanOriginSkillPath(strings.Join(clean, "/"))
}

func relativeOriginPath(root, skillDir string) string {
	rel, err := filepath.Rel(root, skillDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ""
	}
	return cleanOriginSkillPath(filepath.ToSlash(rel))
}
