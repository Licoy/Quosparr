package skills

import (
	"errors"
	"io"
	"os"
	"time"
)

type originCandidate struct {
	from                         OriginFrom
	repo, ref, branch, skillPath string
	homepage, version, installer string
	commit                       string
	installedAt, updatedAt       time.Time
}

type originScanner struct {
	env     Env
	records map[string]Record
	locks   *originLockCache
	git     *originGitCache
	plugins *originPluginCache
}

func newOriginScanner(env Env) *originScanner {
	if env.Getenv == nil {
		env.Getenv = func(string) string { return "" }
	}
	records, _ := LoadRecords(env.Home)
	return &originScanner{env: env, records: records, locks: &originLockCache{env: env}, git: newOriginGitCache(env), plugins: newOriginPluginCache(env)}
}

func (s *originScanner) fill(skills []Skill) {
	for i := range skills {
		front := candidateFromOrigin(skills[i].Origin)
		rec, imported := s.records[recordKey(skills[i].Dir)]
		imported = imported && skills[i].Scope == ScopeUser
		candidates := []originCandidate{s.locks.candidate(skills[i]), s.git.candidate(skills[i]), s.plugins.candidate(skills[i]), front}
		if imported {
			candidates = append([]originCandidate{recordCandidate(rec)}, candidates...)
		}
		skills[i].Origin = mergeOriginCandidates(candidates...)
		if imported {
			applyRecord(&skills[i].Origin, rec)
		}
	}
}

func recordCandidate(rec Record) originCandidate {
	return originCandidate{from: OriginQuosparr, repo: rec.Repository, ref: rec.Ref, branch: rec.Ref, skillPath: rec.SkillPath,
		installer: string(rec.Kind), commit: rec.Commit, installedAt: rec.InstalledAt, updatedAt: rec.UpdatedAt}
}

// applyRecord keeps a Quosparr import as the origin even when it came from
// a local folder or zip, which leaves no repository to choose it by.
func applyRecord(o *Origin, rec Record) {
	o.From, o.Installer, o.Source = OriginQuosparr, string(rec.Kind), rec.Source
	o.InstalledAt, o.UpdatedAt = rec.InstalledAt, rec.UpdatedAt
	if rec.Kind == SourceGitHub {
		o.Commit = rec.Commit
	}
}

func candidateFromOrigin(origin Origin) originCandidate {
	return originCandidate{from: origin.From, repo: origin.Repository, ref: origin.Branch, branch: origin.Branch,
		skillPath: origin.SkillPath, homepage: origin.Homepage, version: origin.Version, installer: origin.Installer,
		installedAt: origin.InstalledAt, updatedAt: origin.UpdatedAt}
}

func mergeOriginCandidates(candidates ...originCandidate) Origin {
	var result Origin
	skipHomepage := -1
	if candidate, index, promoted, ok := originRepositoryCandidate(candidates); ok {
		result.From = candidate.from
		result.Repository, result.GitHub = normalizedCandidateRepo(candidate.repo)
		result.SkillPath = cleanOriginSkillPath(candidate.skillPath)
		result.Branch = candidate.branch
		result.Folder = originFolder(result.Repository, candidate.ref, result.SkillPath)
		result.Installer = candidate.installer
		result.InstalledAt, result.UpdatedAt = candidate.installedAt, candidate.updatedAt
		if promoted {
			skipHomepage = index
		}
	}
	applyOriginLabels(&result, candidates, skipHomepage)
	if result.From == OriginNone {
		return Origin{Version: result.Version}
	}
	if result.Repository != "" {
		applyOriginTimes(&result, candidates)
	}
	return result
}

func originRepositoryCandidate(candidates []originCandidate) (originCandidate, int, bool, bool) {
	for index, candidate := range candidates {
		if candidate.repo != "" {
			return candidate, index, false, true
		}
	}
	for index, candidate := range candidates {
		if repo, _, ok := NormalizeRepoURL(candidate.homepage); ok && stringsHasGitHub(repo) {
			candidate.repo, candidate.homepage = repo, ""
			return candidate, index, true, true
		}
	}
	return originCandidate{}, -1, false, false
}

func applyOriginLabels(result *Origin, candidates []originCandidate, skipHomepage int) {
	for index, candidate := range candidates {
		if index == skipHomepage {
			continue
		}
		homepage := normalizeHomepage(candidate.homepage)
		if homepage != "" && homepage != result.Repository {
			result.Homepage = homepage
			if result.From == OriginNone {
				result.From = candidate.from
			}
			break
		}
	}
	for _, candidate := range candidates {
		if result.Version == "" && candidate.version != "" {
			result.Version = candidate.version
		}
	}
}

func applyOriginTimes(result *Origin, candidates []originCandidate) {
	for _, candidate := range candidates {
		if result.InstalledAt.IsZero() && !candidate.installedAt.IsZero() {
			result.InstalledAt = candidate.installedAt
		}
		if result.UpdatedAt.IsZero() && !candidate.updatedAt.IsZero() {
			result.UpdatedAt = candidate.updatedAt
		}
	}
}

func normalizedCandidateRepo(raw string) (string, string) {
	repo, github, ok := NormalizeRepoURL(raw)
	if !ok {
		return "", ""
	}
	return repo, github
}

func stringsHasGitHub(repo string) bool {
	return len(repo) > len("https://github.com/") && repo[:len("https://github.com/")] == "https://github.com/"
}

func readOriginRegular(path string) ([]byte, error) {
	return readOriginFile(path, 64<<10, false)
}

func readOriginManifest(path string) ([]byte, error) {
	return readOriginFile(path, 1<<20, true)
}

func readOriginFile(path string, limit int64, follow bool) ([]byte, error) {
	var info os.FileInfo
	var err error
	if follow {
		info, err = os.Stat(path)
	} else {
		info, err = os.Lstat(path)
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		if err != nil {
			return nil, err
		}
		return nil, errNotRegular
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		if err != nil {
			return nil, err
		}
		return nil, errNotRegular
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("skills: origin file too large")
	}
	return data, nil
}
