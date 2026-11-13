package skills

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errGeminiConfig = errors.New("skills: invalid Gemini config.json")

// geminiConfig is the part of <GeminiHome>/config/config.json that switches
// installed plugins. A plugin without an entry is enabled.
type geminiConfig struct {
	Plugins map[string]struct {
		Enabled *bool `json:"enabled"`
	} `json:"plugins"`
}

func scanGeminiPlugins(ctx context.Context, env Env, summary *Summary) error {
	base := filepath.Join(env.GeminiHome, "config", "plugins")
	entries, err := readDirIfPresent(base)
	if err != nil || len(entries) == 0 {
		return err
	}
	config, err := readGeminiConfig(filepath.Join(env.GeminiHome, "config", "config.json"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		root := newRoot(env, filepath.Join(base, name, "skills"), ScopePlugin)
		if !root.Exists {
			continue
		}
		summary.Roots = append(summary.Roots, root)
		found, issues, err := scanGeminiRoot(ctx, env, root)
		if err != nil {
			return err
		}
		on := true
		if state, ok := config.Plugins[name]; ok && state.Enabled != nil {
			on = *state.Enabled
		}
		for _, skill := range found {
			skill.Plugin = name
			skill.InvokeName = name + ":" + skill.Name
			skill.Deletable = false
			skill.ToggleBlock = ReasonPluginCollection
			if !on && skill.State != StateNotLoaded {
				skill.State, skill.Reason = StateOff, ReasonPluginOff
			}
			summary.Skills = append(summary.Skills, skill)
		}
		summary.Issues = append(summary.Issues, issues...)
	}
	return nil
}

func readGeminiConfig(path string) (geminiConfig, error) {
	var config geminiConfig
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || err == nil && len(strings.TrimSpace(string(data))) == 0 {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, errGeminiConfig
	}
	return config, nil
}
