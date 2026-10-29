package skills

import (
	"errors"
	"strings"
)

// SourceKind is where an imported skill was read from.
type SourceKind string

// SourceKind values.
const (
	SourceGitHub SourceKind = "github"
	SourceFolder SourceKind = "folder"
	SourceZip    SourceKind = "zip"
)

// Import limits. A source is read into memory before anything is written,
// so what the preview shows is exactly what gets installed. Only the
// skills' folders are kept, so the download limits can be far larger than
// what is kept: a large repository costs bandwidth, not memory.
const (
	MaxImportBytes     = 64 << 20 // file contents kept from one source
	MaxImportFiles     = 10000    // files kept from one source
	MaxImportFileBytes = 16 << 20 // one file
	MaxImportDownload  = 1 << 30  // compressed bytes read from GitHub, or a zip's size
	MaxImportExpanded  = 4 << 30  // bytes decompressed, kept or skipped
	MaxImportSkills    = 100
	maxImportDepth     = 24
	maxImportLinkHops  = 8
)

// Source is what an import reads. A GitHub source names a public
// repository, a ref ("" is the default branch) and a folder in it.
type Source struct {
	Kind        SourceKind
	Owner, Repo string
	Ref         string
	Subpath     string // slash-separated folder in the repository
	Path        string // the local folder or zip file
	// treePath is what followed /tree/ in a pasted URL; a branch name may
	// contain slashes, so the split into Ref and Subpath is only a guess.
	treePath string
}

// Repository is the source's https URL on GitHub, or "".
func (s Source) Repository() string {
	if s.Kind != SourceGitHub {
		return ""
	}
	return "https://github.com/" + s.Owner + "/" + s.Repo
}

// Label names the source for display: owner/repo[/folder][@ref] or the
// local path.
func (s Source) Label() string {
	if s.Kind != SourceGitHub {
		return s.Path
	}
	label := s.Owner + "/" + s.Repo
	if s.Subpath != "" {
		label += "/" + s.Subpath
	}
	if s.Ref != "" {
		label += "@" + s.Ref
	}
	return label
}

// ImportError carries a stable code for localization and, at most, a path
// inside the source, a ref or an HTTP status; never file content.
type ImportError struct {
	Code   string
	Detail string
}

// ImportError codes.
const (
	ImportInvalidSource  = "invalidSource"  // not a GitHub repository, folder or zip
	ImportNotFound       = "notFound"       // missing or private repository, ref or folder
	ImportRateLimited    = "rateLimited"    // GitHub API rate limit
	ImportNetwork        = "network"        // the request failed
	ImportHTTP           = "http"           // an unexpected HTTP status
	ImportTooLarge       = "tooLarge"       // a size limit was reached
	ImportTooManyFiles   = "tooManyFiles"   // the file limit was reached
	ImportInvalidPath    = "invalidPath"    // an unsafe or unusable path in the source
	ImportInvalidArchive = "invalidArchive" // the zip or tarball cannot be read
	ImportCaseConflict   = "caseConflict"   // two paths differ only in case
	ImportNoSkills       = "noSkills"       // no SKILL.md was found
	ImportExists         = "exists"         // the target folder already exists
	ImportUnsafeRoot     = "unsafeRoot"     // the target skills folder is a link or a file
	ImportChanged        = "changed"        // the target changed since the review
	ImportWrite          = "write"          // writing the files failed
	ImportNotUpdatable   = "notUpdatable"   // not a skill Quosparr installed from GitHub
	ImportModified       = "modified"       // the installed copy was edited locally
	ImportMissing        = "missing"        // the skill is gone from its source
)

func (e *ImportError) Error() string {
	if e.Detail == "" {
		return "skills import: " + e.Code
	}
	return "skills import: " + e.Code + ": " + e.Detail
}

func importError(code, detail string) error { return &ImportError{Code: code, Detail: detail} }

// importCode is the code of an import error, or "".
func importCode(err error) string {
	var detail *ImportError
	if errors.As(err, &detail) {
		return detail.Code
	}
	return ""
}

// importSkippedDirs are build output and version control folders never
// read from a source; importSkippedFiles are operating system litter.
var (
	importSkippedDirs = map[string]bool{
		".git": true, ".hg": true, ".svn": true, "node_modules": true,
		"__pycache__": true, "__pypackages__": true, ".venv": true, "__MACOSX": true,
	}
	importSkippedFiles = map[string]bool{".DS_Store": true, "Thumbs.db": true, "desktop.ini": true}
	// skillSkippedNames are repository plumbing left out of an installed
	// skill even when the skill is the repository itself.
	skillSkippedNames = map[string]bool{".github": true, ".gitignore": true, ".gitattributes": true, ".gitmodules": true}
)

// cleanImportPath checks a slash-separated relative path from a source:
// no absolute paths, empty, "." or ".." parts, backslashes, NUL or control
// characters, and no deeper than maxImportDepth.
func cleanImportPath(raw string) (string, bool) {
	raw = strings.TrimSuffix(raw, "/")
	if raw == "" || strings.HasPrefix(raw, "/") || strings.ContainsAny(raw, "\\:") {
		return "", false
	}
	parts := strings.Split(raw, "/")
	if len(parts) > maxImportDepth {
		return "", false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, " .") != part {
			return "", false
		}
		for _, r := range part {
			if r < 0x20 || r == 0x7f {
				return "", false
			}
		}
	}
	return raw, true
}

// sanitizeImportName turns a skill name into its folder name: lowercase
// letters, digits, dots and underscores, other runs as one hyphen, no
// leading or trailing dot or hyphen.
func sanitizeImportName(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), ".-")
	if len(out) > 128 {
		out = strings.Trim(out[:128], ".-")
	}
	return out
}
