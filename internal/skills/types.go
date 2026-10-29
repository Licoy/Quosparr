// Package skills discovers the global Agent Skills each CLI loads (Codex,
// Claude Code, Grok), reports whether the CLI would use them, and removes a
// user-owned skill into Quosparr's restorable trash (~/.quosparr/skills-trash).
// Permanent removal goes through the system trash. It only reads official
// configuration; enabling or disabling a skill is written by the provider
// packages (codex, claude, grok).
package skills

import "time"

// Provider is the CLI a summary belongs to. Values match store.Provider.
type Provider string

// Provider values.
const (
	ProviderCodex  Provider = "codex"
	ProviderClaude Provider = "claude"
	ProviderGrok   Provider = "grok"
)

// Scope says who owns a skill's location.
type Scope string

// Scope values.
const (
	ScopeUser    Scope = "user"    // a user skills directory; deletable
	ScopeSystem  Scope = "system"  // Codex $CODEX_HOME/skills/.system
	ScopeBundled Scope = "bundled" // Grok $GROK_HOME/bundled/skills
	ScopeAdmin   Scope = "admin"   // /etc/codex/skills, Claude managed settings
	ScopePlugin  Scope = "plugin"  // an installed plugin, managed by the CLI
	ScopeSynced  Scope = "synced"  // Claude <config>/skills/synced (claude.ai)
)

// State is whether the CLI would load and offer the skill.
type State string

// State values.
const (
	StateOn         State = "on"
	StateOff        State = "off"        // disabled by configuration
	StateOverridden State = "overridden" // a same-name skill of higher priority wins
	StateNotLoaded  State = "notLoaded"  // on disk, but outside what the CLI scans
)

// Reason explains State (and why a skill cannot be toggled).
type Reason string

// Reason values.
const (
	ReasonNone             Reason = ""
	ReasonConfigPath       Reason = "configPath"       // Codex [[skills.config]] path = …, enabled = false
	ReasonConfigName       Reason = "configName"       // Codex [[skills.config]] name = …, enabled = false
	ReasonBundledOff       Reason = "bundledOff"       // Codex skills.bundled.enabled = false; Claude disableBundledSkills
	ReasonOverride         Reason = "override"         // Claude skillOverrides[name] = "off"
	ReasonPermissionDeny   Reason = "permissionDeny"   // Claude permissions.deny Skill(name)
	ReasonDisabledList     Reason = "disabledList"     // Grok [skills].disabled
	ReasonIgnored          Reason = "ignored"          // Grok [skills].ignore path prefix
	ReasonCompatOff        Reason = "compatOff"        // Grok [compat.claude|cursor] skills = false or env
	ReasonPluginOff        Reason = "pluginOff"        // the plugin is not enabled
	ReasonShadowed         Reason = "shadowed"         // see Skill.OverriddenBy
	ReasonNested           Reason = "nested"           // Claude only loads <skills>/<name>/SKILL.md
	ReasonTooDeep          Reason = "tooDeep"          // beyond the CLI's scan depth
	ReasonMissingDesc      Reason = "missingDesc"      // Codex refuses a skill without description
	ReasonInvalidFront     Reason = "invalidFront"     // frontmatter cannot be parsed
	ReasonHiddenDir        Reason = "hiddenDir"        // inside a dot directory the CLI skips
	ReasonReadOnlyScope    Reason = "readOnlyScope"    // system / bundled / admin / plugin / synced
	ReasonPluginCollection Reason = "pluginCollection" // part of a plugin; toggled through the plugin
)

// Field is one display-only frontmatter or interface value, e.g.
// {"allowed-tools", "Read, Grep"}.
type Field struct {
	Key   string
	Value string
}

// Removal is exactly what Delete moves to the trash for a skill.
type Removal struct {
	Path   string   // the directory, or the symlink itself when IsLink
	IsLink bool     // only the link is removed, never its target
	Covers []string // invoke names of every skill that goes with it (including this one)
}

// Skill is one SKILL.md as a CLI sees it.
type Skill struct {
	ID               string // stable within a summary: Provider + "\x00" + File
	Provider         Provider
	Name             string // frontmatter name, or the directory name
	InvokeName       string // what the user types: "plugin:skill" for namespaced skills
	DisplayName      string // Codex agents/openai.yaml interface.display_name
	Description      string // single line
	ShortDescription string // metadata.short-description or interface.short_description
	File             string // SKILL.md as discovered, through any symlink (absolute)
	Dir              string // filepath.Dir(File)
	RealFile         string // File with symlinks resolved; used to match shared copies
	Scope            Scope
	Root             string // the scanned root directory (absolute)
	RootLabel        string // Root with the home directory shown as "~"
	Plugin           string // plugin name when Scope is plugin or the skill lives in a plugin collection
	LinkPath         string // the symlink on the way from Root to File, if any
	LinkTarget       string
	State            State
	Reason           Reason
	OverriddenBy     string // File of the winning skill when Reason is shadowed
	SharedWith       []Provider
	Fields           []Field
	ModTime          time.Time // SKILL.md modification time
	Deletable        bool
	Removal          Removal // valid when Deletable
	Toggleable       bool    // the provider package can switch it on or off
	ToggleBlock      Reason  // why not, when !Toggleable
	Origin           Origin  // where it came from, as far as local files tell
}

// OriginFrom says which local file an Origin was read from.
type OriginFrom string

// OriginFrom values, in the order a scan trusts them.
const (
	OriginNone        OriginFrom = ""
	OriginQuosparr    OriginFrom = "quosparr"    // ~/.quosparr/skill-sources.json, written when Quosparr installs a skill
	OriginLock        OriginFrom = "lock"        // ~/.agents/.skill-lock.json, written by the skills CLI
	OriginGit         OriginFrom = "git"         // remote "origin" of an enclosing git checkout
	OriginPlugin      OriginFrom = "plugin"      // the plugin or its marketplace manifest
	OriginFrontmatter OriginFrom = "frontmatter" // homepage / repository keys of SKILL.md
)

// Origin is where a skill came from. Every URL is https with credentials,
// query and fragment removed; anything else is dropped.
type Origin struct {
	From        OriginFrom
	Repository  string // the source repository
	GitHub      string // "owner/repo" when Repository is on github.com
	Homepage    string // a declared homepage other than Repository
	Folder      string // the skill's folder inside Repository, when known
	SkillPath   string // that folder relative to the repository root ("skills/foo")
	Branch      string // the checked-out branch of a git checkout
	Version     string // plugin or frontmatter version
	Installer   string // "skills", "git", the plugin marketplace name, or the SourceKind Quosparr imported from
	Source      string // the local folder or zip Quosparr imported from
	Commit      string // the commit Quosparr imported at
	InstalledAt time.Time
	UpdatedAt   time.Time
}

// FileEntry is one file or directory under a skill's folder.
type FileEntry struct {
	Path    string // slash-separated, relative to the skill's folder
	Size    int64  // regular files only
	Dir     bool
	Link    bool // a symlink, listed but never followed
	Skipped bool // a directory (.git, node_modules) listed but not walked
	ModTime time.Time
}

// Detail is what Inspect reads when a skill's page opens.
type Detail struct {
	Files     []FileEntry // sorted by Path, at most MaxDetailFiles
	FileCount int         // regular files seen
	DirCount  int
	Size      int64 // bytes of the regular files seen
	Truncated bool  // a limit stopped the walk; the counts are lower bounds
}

// TrashKind is what a trash entry holds.
type TrashKind string

// TrashKind values.
const (
	TrashSkill      TrashKind = "skill"      // a skill folder, a collection, or a link to one
	TrashBrokenLink TrashKind = "brokenLink" // a dangling link from the diagnostics list
)

// TrashEntry is one item in Quosparr's skills trash. Restore moves Item
// back to Original; Purge hands the entry to the system trash.
type TrashEntry struct {
	ID            string
	Kind          TrashKind
	Provider      Provider // the CLI whose page it was deleted from
	SharedWith    []Provider
	Name          string
	InvokeName    string
	DisplayName   string
	Description   string
	Covers        []string // invoke names that went with it
	Original      string   // absolute path it was moved from
	OriginalLabel string   // Original with the home directory shown as "~"
	Root          string   // the user skills root it lived under
	IsLink        bool
	LinkTarget    string
	Origin        Origin
	DeletedAt     time.Time
	Size          int64  // bytes of regular files, links not followed; -1 when unknown
	Item          string // the moved file, directory or link inside the trash
}

// IssueKind is a problem worth showing in the diagnostics list.
type IssueKind string

// IssueKind values.
const (
	IssueBrokenLink    IssueKind = "brokenLink"    // a symlink in a skills root whose target is gone
	IssueNested        IssueKind = "nested"        // a SKILL.md the CLI will not load (see Skill.Reason)
	IssueMissingDesc   IssueKind = "missingDesc"   // no description
	IssueInvalidFront  IssueKind = "invalidFront"  // frontmatter does not parse
	IssueNameTooLong   IssueKind = "nameTooLong"   // longer than the CLI accepts
	IssueDuplicateName IssueKind = "duplicateName" // several skills share an invoke name
	IssueTruncated     IssueKind = "truncated"     // a root hit the scan limits
)

// Issue is one diagnostics entry. Path is a file or link path, never content.
type Issue struct {
	Kind      IssueKind
	Path      string
	Name      string // skill or link name, when known
	Detail    string // e.g. the dangling link target, or the other path of a duplicate
	Removable bool   // a broken link that DeleteBrokenLink may remove
	Fixes     []Fix  // the one-click fixes on offer, in the order to show
	Lift      NestedFix
}

// RootInfo describes one scanned location, present or not.
type RootInfo struct {
	Path   string
	Label  string
	Scope  Scope
	Exists bool
	Count  int
}

// Summary is the result of scanning one provider.
type Summary struct {
	Provider  Provider
	Skills    []Skill // in the order the CLI prefers them
	Issues    []Issue
	Roots     []RootInfo
	ScannedAt time.Time
}

// Env carries the directories a scan needs. Empty values fall back to the
// provider defaults under Home.
type Env struct {
	Home      string // user home directory
	CodexHome string // resolved CODEX_HOME
	ClaudeDir string // resolved CLAUDE_CONFIG_DIR
	GrokHome  string // resolved GROK_HOME
	GOOS      string // runtime.GOOS unless a test overrides it
	// Getenv reads environment variables (Grok GROK_CLAUDE_SKILLS_ENABLED,
	// GROK_CURSOR_SKILLS_ENABLED). Nil reads none.
	Getenv func(string) string
	Now    time.Time
}

// Limits shared by every scan.
const (
	MaxFrontmatterBytes = 64 << 10
	MaxBodyBytes        = 256 << 10
	MaxDetailFiles      = 400  // entries Inspect lists
	MaxDetailWalk       = 5000 // entries Inspect visits before it stops counting
	MaxDetailDepth      = 8
)
