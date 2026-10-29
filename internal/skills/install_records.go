package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

// Record is how Quosparr installed a skill folder: its origin, and what an
// update compares against. It holds no secrets.
type Record struct {
	Kind        SourceKind `json:"kind"`
	Repository  string     `json:"repository,omitempty"` // https://github.com/owner/repo
	Ref         string     `json:"ref,omitempty"`        // "" follows the default branch
	SkillPath   string     `json:"skillPath,omitempty"`  // the skill's folder in the repository
	Commit      string     `json:"commit,omitempty"`
	TreeSHA     string     `json:"treeSha,omitempty"` // GitHub tree SHA of SkillPath at Commit
	Source      string     `json:"source,omitempty"`  // the local folder or zip
	ContentHash string     `json:"contentHash"`       // of the files written, see contentHash
	InstalledAt time.Time  `json:"installedAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// RecordsFileName is the install record under ~/.quosparr, keyed by the
// absolute folder each skill was installed to.
const RecordsFileName = "skill-sources.json"

const maxRecordsBytes = 4 << 20

var recordsMu sync.Mutex

type recordsFile struct {
	Version int               `json:"version"`
	Skills  map[string]Record `json:"skills"`
}

func recordsPath(home string) string {
	return filepath.Join(platform.RunwayDir(home), RecordsFileName)
}

// LoadRecords reads the install record; a missing file is empty.
func LoadRecords(home string) (map[string]Record, error) {
	recordsMu.Lock()
	defer recordsMu.Unlock()
	return loadRecords(home)
}

func loadRecords(home string) (map[string]Record, error) {
	data, err := readOriginFile(recordsPath(home), maxRecordsBytes, false)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	var file recordsFile
	if err := json.Unmarshal(data, &file); err != nil || file.Version != 1 {
		return nil, errors.New("skills: unreadable install record")
	}
	if file.Skills == nil {
		file.Skills = map[string]Record{}
	}
	return file.Skills, nil
}

// editRecords changes the record under its lock and drops the folders
// that no longer exist.
func editRecords(home string, edit func(map[string]Record)) error {
	recordsMu.Lock()
	defer recordsMu.Unlock()
	records, err := loadRecords(home)
	if err != nil {
		return err
	}
	edit(records)
	for dir := range records {
		if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
			delete(records, dir)
		}
	}
	data, err := json.MarshalIndent(recordsFile{Version: 1, Skills: records}, "", "  ")
	if err != nil {
		return err
	}
	if err := platform.EnsurePrivateDirectory(platform.RunwayDir(home)); err != nil {
		return err
	}
	return platform.WriteFileAtomic(recordsPath(home), append(data, '\n'), 0o600)
}

// contentHash identifies the files of a skill: paths and contents. File
// modes are left out, since Windows keeps no executable bit.
func contentHash(files map[string]importFile) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		data := files[p].data
		h.Write([]byte(p + "\x00" + strconv.Itoa(len(data)) + "\x00"))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// installedHash reads an installed skill folder back the way an import
// reads one and hashes what it would install.
func installedHash(dir string) (string, error) {
	snap, err := LoadFolder(context.Background(), dir)
	if err != nil {
		return "", err
	}
	c := Candidate{}
	(&discoverer{snap: snap}).collect(&c)
	return contentHash(c.data), nil
}

// recordRepo splits a recorded https://github.com/owner/repo.
func recordRepo(rec Record) (string, string, bool) {
	rest, ok := strings.CutPrefix(rec.Repository, "https://github.com/")
	owner, repo, found := strings.Cut(rest, "/")
	if !ok || !found || !validGitHubName(owner) || !validGitHubName(repo) {
		return "", "", false
	}
	return owner, repo, true
}

func recordKey(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Clean(dir)
}
