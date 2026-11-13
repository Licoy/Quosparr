package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/licoy/quosparr/internal/sessions"
)

// Client names of the two Antigravity data roots under GeminiHome.
const (
	SessionClientApp = "Antigravity"     // <GeminiHome>/antigravity
	SessionClientCLI = "Antigravity CLI" // <GeminiHome>/antigravity-cli
)

const (
	sessionSummariesFile = "conversation_summaries.db"
	sessionUntitled      = "Untitled session"
)

type sessionRoot struct {
	path   string
	client string
}

// SessionRoots are the data roots the scanner reads and DeleteManaged trusts:
// the desktop App's and the CLI's.
func (s SessionScanner) SessionRoots() []string {
	roots := s.sessionRoots()
	out := make([]string, len(roots))
	for i, root := range roots {
		out[i] = root.path
	}
	return out
}

// SessionClientOf names the client owning one of SessionRoots
// (SessionClientApp or SessionClientCLI), "" for any other path.
func (s SessionScanner) SessionClientOf(root string) string {
	for _, r := range s.sessionRoots() {
		if filepath.Clean(root) == r.path {
			return r.client
		}
	}
	return ""
}

func (s SessionScanner) sessionRoots() []sessionRoot {
	if strings.TrimSpace(s.GeminiHome) == "" {
		return nil
	}
	home := filepath.Clean(s.GeminiHome)
	return []sessionRoot{
		{path: filepath.Join(home, "antigravity"), client: SessionClientApp},
		{path: filepath.Join(home, "antigravity-cli"), client: SessionClientCLI},
	}
}

// sessionSummary is one conversation_summaries row. Every client keeps one
// such table; the CLI's also lists conversations of the desktop App.
type sessionSummary struct {
	title, preview, workspace string
	steps                     int64
	updatedAt                 time.Time
}

// sessionEntry is one conversation database found on disk.
type sessionEntry struct {
	id      string
	root    sessionRoot
	path    string
	state   sessionFileState
	summary sessionSummary
	listed  bool // a summaries row was found
}

func (e sessionEntry) updatedAt() time.Time {
	if !e.summary.updatedAt.IsZero() {
		return e.summary.updatedAt
	}
	return e.state.updatedAt()
}

func (e sessionEntry) title() string {
	if title := sessionOneLine(e.summary.title); title != "" {
		return title
	}
	if preview := sessionOneLine(e.summary.preview); preview != "" {
		return preview
	}
	return sessionUntitled
}

func (e sessionEntry) cwd() *string {
	if e.summary.workspace == "" {
		return nil
	}
	cwd := e.summary.workspace
	return &cwd
}

func (e sessionEntry) projectName() string {
	if e.summary.workspace == "" {
		return sessions.UnknownProject
	}
	return filepath.Base(e.summary.workspace)
}

// sessionList finds every conversation database, newest first, and counts
// the files it cannot read (the older one-file .pb conversations).
func (s SessionScanner) sessionList(ctx context.Context) ([]sessionEntry, int, error) {
	var entries []sessionEntry
	legacy := 0
	summaries := map[string]map[string]sessionSummary{}
	roots := s.sessionRoots()
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		rows, err := sessionReadSummaries(ctx, filepath.Join(root.path, sessionSummariesFile))
		if err != nil && ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		summaries[root.path] = rows
		found, old, err := sessionListRoot(root)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, found...)
		legacy += old
	}
	for i := range entries {
		entry := &entries[i]
		if row, ok := summaries[entry.root.path][entry.id]; ok {
			entry.summary, entry.listed = row, true
			continue
		}
		for _, root := range roots {
			if row, ok := summaries[root.path][entry.id]; ok {
				entry.summary, entry.listed = row, true
				break
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i].updatedAt(), entries[j].updatedAt()
		if !a.Equal(b) {
			return a.After(b)
		}
		return entries[i].path < entries[j].path
	})
	return entries, legacy, nil
}

func sessionListRoot(root sessionRoot) ([]sessionEntry, int, error) {
	dir := filepath.Join(root.path, "conversations")
	items, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var out []sessionEntry
	legacy := 0
	for _, item := range items {
		name := item.Name()
		if strings.HasPrefix(name, ".") || !item.Type().IsRegular() {
			continue
		}
		switch filepath.Ext(name) {
		case ".pb":
			legacy++
		case ".db":
			path := filepath.Join(dir, name)
			state, err := sessionStateOf(path)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return nil, 0, err
			}
			out = append(out, sessionEntry{id: strings.TrimSuffix(name, ".db"), root: root, path: path, state: state})
		}
	}
	return out, legacy, nil
}

// sessionReadSummaries reads one summaries table; a missing or unreadable
// database only means the titles fall back to the conversation itself.
func sessionReadSummaries(ctx context.Context, path string) (map[string]sessionSummary, error) {
	if _, err := os.Lstat(path); err != nil {
		return nil, nil
	}
	out := map[string]sessionSummary{}
	_, err := sessionWithCopy(ctx, path, func(db *sqlite.DB, _ string) error {
		columns, err := sessionColumns(ctx, db, "conversation_summaries")
		if err != nil || !columns["conversation_id"] {
			return sessionErrSchema
		}
		pick := func(name string) string {
			if columns[name] {
				return name
			}
			return "NULL"
		}
		query := "SELECT conversation_id, " + strings.Join([]string{pick("title"), pick("preview"), pick("step_count"),
			pick("last_modified_time"), pick("workspace_uris")}, ", ") + " FROM conversation_summaries"
		rows, err := db.Query(ctx, query)
		if err != nil {
			return err
		}
		for _, row := range rows.Values {
			id := sessionString(row[0])
			if id == "" {
				continue
			}
			out[id] = sessionSummary{
				title: sessionString(row[1]), preview: sessionString(row[2]), steps: sessionInt(row[3]),
				updatedAt: sessionParseTime(row[4]), workspace: sessionWorkspace(sessionString(row[5])),
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

var sessionTimeLayouts = []string{"2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999Z07:00", time.RFC3339Nano}

// sessionParseTime reads a summaries datetime; year 1 is the unset value.
func sessionParseTime(value any) time.Time {
	text := strings.TrimSpace(sessionString(value))
	for _, layout := range sessionTimeLayouts {
		if t, err := time.Parse(layout, text); err == nil && t.Year() > 1 {
			return t.UTC()
		}
	}
	return time.Time{}
}

// sessionWorkspace is the local folder of the first file:// workspace URI.
func sessionWorkspace(raw string) string {
	var uris []string
	if json.Unmarshal([]byte(raw), &uris) != nil {
		return ""
	}
	for _, uri := range uris {
		parsed, err := url.Parse(uri)
		if err != nil || parsed.Scheme != "file" || parsed.Path == "" {
			continue
		}
		return filepath.Clean(filepath.FromSlash(parsed.Path))
	}
	return ""
}

func sessionOneLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexAny(text, "\r\n"); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	if runes := []rune(text); len(runes) > 120 {
		text = string(runes[:120]) + "…"
	}
	return text
}
