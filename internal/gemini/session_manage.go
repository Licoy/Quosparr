package gemini

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/sessions"
)

// Deletion errors. A refused deletion changes nothing.
var (
	// ErrSessionClientRunning means a client whose files a deletion touches
	// is running, or ClientRunning cannot say.
	ErrSessionClientRunning = errors.New("gemini: quit Antigravity before deleting its conversations")
	// ErrSessionBackup means a summaries database could not be backed up.
	ErrSessionBackup = errors.New("gemini: conversation summaries could not be backed up")
	// ErrSessionSelection means a selection is not a scanned conversation.
	ErrSessionSelection = errors.New("gemini: invalid conversation selection")
)

const (
	sessionMaxBrainFiles = 10000
	sessionBackupKeep    = 10
	sessionBackupLayout  = "20060102T150405.000000000Z"
)

// ListManaged returns every conversation of both clients, newest first,
// with the files a deletion moves: the brain/<id> folder, the per-client
// annotations/<id>.pbtxt, presence/<id>.lock and implicit/<id>.pb, and the
// conversation database with its WAL and shared-memory files, last.
func (s SessionScanner) ListManaged(ctx context.Context) ([]sessions.ManagedSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot := s.pricingSnapshot()
	entries, _, err := s.sessionList(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]sessions.ManagedSession, 0, len(entries))
	for _, entry := range entries {
		files, problem, err := sessionManagedFiles(entry)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		item := sessions.Item{
			ID: entry.id, Title: entry.title(), ProjectName: entry.projectName(), Cwd: entry.cwd(),
			UpdatedAt: entry.updatedAt(), State: sessions.StateRecent,
		}
		conv, err := s.sessionConversationOf(ctx, entry)
		switch {
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case err != nil:
			problem = "unreadable conversation database"
		default:
			sessionFillItem(&item, conv, sessionWindowTotals(sessionPrice(conv, entry.updatedAt(), s.loc(), snapshot)))
			if conv.malformed > 0 && problem == "" {
				problem = fmt.Sprintf("%d malformed conversation record(s)", conv.malformed)
			}
		}
		if problem != "" {
			item.State = sessions.StateNeedsAttention
		}
		result = append(result, sessions.ManagedSession{Item: item, Provider: sessions.ProviderGemini, Path: entry.path, Root: entry.root.path, Files: files, Problem: problem})
	}
	return result, nil
}

func sessionFillItem(item *sessions.Item, conv sessionConversation, totals SessionUsageTotals) {
	api := totals.APITotals()
	item.Totals = sessions.Totals{
		TotalTokens: int(api.TotalTokens), UncachedInputTokens: int(api.UncachedInputTokens), CachedInputTokens: int(api.CachedInputTokens),
		OutputTokens: int(api.OutputTokens), Turns: int(api.Turns), Threads: int(api.Threads),
	}
	item.UnpricedTokens = totals.UnpricedTokens
	if totals.EstimatedUSD != nil {
		if value, err := swiftjson.ParseDecimal(totals.EstimatedUSD.String()); err == nil {
			item.EstimatedUSD = &value
		}
	}
	if conv.model != "" {
		model := conv.model
		item.Model = &model
	}
}

func sessionManagedFiles(entry sessionEntry) ([]sessions.ManagedFile, string, error) {
	root, id := entry.root.path, entry.id
	files, problem, err := sessionBrainFiles(root, filepath.Join(root, "brain", id))
	if err != nil {
		return nil, "", err
	}
	for _, rel := range []string{
		filepath.Join("annotations", id+".pbtxt"),
		filepath.Join("presence", id+".lock"),
		filepath.Join("implicit", id+".pb"),
		filepath.Join("conversations", id+".db-shm"),
		filepath.Join("conversations", id+".db-wal"),
	} {
		file, err := sessions.SnapshotManagedFile(root, filepath.Join(root, rel))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		files = append(files, file)
	}
	db, err := sessions.SnapshotManagedFile(root, entry.path)
	if err != nil {
		return nil, "", err
	}
	return append(files, db), problem, nil
}

// sessionBrainFiles lists the regular files of a conversation's brain
// folder. Links are never followed or moved; a folder that is a link, holds
// one, or is too large is reported and left out.
func sessionBrainFiles(root, dir string) ([]sessions.ManagedFile, string, error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() {
		return nil, "brain folder is not a directory", nil
	}
	var files []sessions.ManagedFile
	problem := ""
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			problem = "brain folder holds a link or special file"
			return nil
		}
		if len(files) >= sessionMaxBrainFiles {
			problem = "brain folder has too many files"
			return fs.SkipAll
		}
		file, err := sessions.SnapshotManagedFile(root, path)
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if problem == "brain folder has too many files" {
		files = nil
	}
	return files, problem, nil
}

// DeleteManaged moves the files of the selected conversations to the system
// Trash and removes their rows from every summaries database that lists
// them (the CLI's also lists App conversations). It runs only when
// ClientRunning reports every touched client stopped, and first copies each
// summaries database it will edit into BackupDir. A conversation whose files
// were not all moved keeps its rows.
func (s SessionScanner) DeleteManaged(ctx context.Context, selected []sessions.ManagedSession, trash func(string) error, now time.Time) ([]sessions.DeleteResult, error) {
	refuse := func(err error) ([]sessions.DeleteResult, error) {
		results := make([]sessions.DeleteResult, len(selected))
		for i, session := range selected {
			results[i] = sessions.DeleteResult{Session: session, Total: len(session.Files), Err: err}
		}
		return results, err
	}
	if len(selected) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return refuse(err)
	}
	roots := s.sessionRoots()
	ids := map[string]bool{}
	touched := map[string]bool{}
	for _, session := range selected {
		if !sessionValidSelection(session, roots) {
			return refuse(ErrSessionSelection)
		}
		ids[session.ID] = true
		touched[filepath.Clean(session.Root)] = true
	}
	holders := map[string][]string{}
	for _, root := range roots {
		path := filepath.Join(root.path, sessionSummariesFile)
		listed, err := sessionSummaryIDs(ctx, path, ids)
		if err != nil {
			return refuse(fmt.Errorf("read %s conversation summaries: %w", root.client, err))
		}
		if len(listed) > 0 {
			holders[root.path] = listed
			touched[root.path] = true
		}
	}
	for _, root := range roots {
		if touched[root.path] && (s.ClientRunning == nil || s.ClientRunning(root.path)) {
			return refuse(fmt.Errorf("%w (%s)", ErrSessionClientRunning, root.client))
		}
	}
	for _, root := range roots {
		if len(holders[root.path]) == 0 {
			continue
		}
		if err := s.sessionBackup(ctx, root, now); err != nil {
			return refuse(fmt.Errorf("%w: %s", ErrSessionBackup, sessionSafeError(err)))
		}
	}
	results, err := sessions.DeleteManaged(ctx, selected, map[string][]string{sessions.ProviderGemini: s.SessionRoots()}, trash)
	removed := map[string]bool{}
	for _, result := range results {
		if result.Err == nil && result.Removed == result.Total {
			removed[result.Session.ID] = true
			sessionPruneBrain(filepath.Join(result.Session.Root, "brain", result.Session.ID))
		}
	}
	var failures []error
	if err != nil {
		failures = append(failures, err)
	}
	for _, root := range roots {
		var gone []string
		for _, id := range holders[root.path] {
			if removed[id] {
				gone = append(gone, id)
			}
		}
		if len(gone) == 0 {
			continue
		}
		if err := sessionDeleteRows(ctx, filepath.Join(root.path, sessionSummariesFile), gone); err != nil {
			err = fmt.Errorf("conversation files moved to Trash, but the %s summary rows remain: %s", root.client, sessionSafeError(err))
			failures = append(failures, err)
			for i := range results {
				if slices.Contains(gone, results[i].Session.ID) && results[i].Err == nil {
					results[i].Err = err
				}
			}
		}
	}
	return results, errors.Join(failures...)
}

// sessionValidSelection accepts only <root>/conversations/<ID>.db of a known
// root, so the ID used for the summary rows is the deleted database.
func sessionValidSelection(session sessions.ManagedSession, roots []sessionRoot) bool {
	if session.Provider != sessions.ProviderGemini || session.ID == "" || strings.ContainsAny(session.ID, `/\`) || strings.HasPrefix(session.ID, ".") {
		return false
	}
	for _, root := range roots {
		if filepath.Clean(session.Root) == root.path && filepath.Clean(session.Path) == filepath.Join(root.path, "conversations", session.ID+".db") {
			return true
		}
	}
	return false
}

func sessionSummaryIDs(ctx context.Context, path string, ids map[string]bool) ([]string, error) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var listed []string
	_, err := sessionWithCopy(ctx, path, func(db *sqlite.DB, _ string) error {
		tables, err := sessionTables(ctx, db)
		if err != nil || !tables["conversation_summaries"] {
			return err
		}
		rows, err := db.Query(ctx, "SELECT conversation_id FROM conversation_summaries")
		if err != nil {
			return err
		}
		for _, row := range rows.Values {
			if id := sessionString(row[0]); ids[id] {
				listed = append(listed, id)
			}
		}
		return nil
	})
	sort.Strings(listed)
	return listed, err
}

// sessionBackup stores a consolidated copy (WAL merged) of a summaries
// database as <BackupDir>/gemini-<root>-conversation-summaries-<UTC>.db,
// keeping the newest ten.
func (s SessionScanner) sessionBackup(ctx context.Context, root sessionRoot, now time.Time) error {
	if s.BackupDir == "" || !filepath.IsAbs(s.BackupDir) {
		return errors.New("backup directory must be absolute")
	}
	var data []byte
	_, err := sessionWithCopy(ctx, filepath.Join(root.path, sessionSummariesFile), func(db *sqlite.DB, copyPath string) error {
		if _, err := db.Query(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			return err
		}
		var err error
		data, err = os.ReadFile(copyPath)
		return err
	})
	if err != nil {
		return err
	}
	if err := platform.EnsurePrivateDirectory(s.BackupDir); err != nil {
		return err
	}
	prefix := "gemini-" + filepath.Base(root.path) + "-conversation-summaries-"
	stamp := now.UTC()
	name := ""
	for {
		name = filepath.Join(s.BackupDir, prefix+stamp.Format(sessionBackupLayout)+".db")
		if _, err := os.Lstat(name); errors.Is(err, fs.ErrNotExist) {
			break
		}
		stamp = stamp.Add(time.Nanosecond)
	}
	if err := platform.WriteFileAtomic(name, data, 0o600); err != nil {
		return err
	}
	sessionPruneBackups(s.BackupDir, prefix)
	return nil
}

func sessionPruneBackups(dir, prefix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > sessionBackupKeep {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

// sessionDeleteRows removes the summary, parent and search rows of ids in
// one transaction on the live database, with every client stopped.
func sessionDeleteRows(ctx context.Context, path string, ids []string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("summaries database is not a regular file")
	}
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{BusyTimeout: 2 * time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	tables, err := sessionTables(ctx, db)
	if err != nil {
		return err
	}
	var statements []sqlite.Statement
	for _, table := range []string{"conversation_summaries", "conversation_parents", "conversations_fts"} {
		if !tables[table] {
			continue
		}
		for _, id := range ids {
			statements = append(statements, sqlite.Statement{SQL: "DELETE FROM " + table + " WHERE conversation_id = ?", Args: []any{id}})
		}
	}
	if len(statements) == 0 {
		return nil
	}
	_, err = db.Transaction(ctx, statements)
	if err != nil {
		return err
	}
	return db.Close()
}

// sessionPruneBrain removes the folders a deletion emptied, deepest first;
// a folder that still holds anything stays.
func sessionPruneBrain(dir string) {
	var dirs []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, path := range dirs {
		_ = os.Remove(path)
	}
}

// sessionSafeError keeps an error's message but not the paths it names.
func sessionSafeError(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Op + ": " + pathErr.Err.Error()
	}
	return err.Error()
}
