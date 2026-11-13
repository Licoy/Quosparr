package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrTransferProvider   = errors.New("skills: invalid transfer provider")
	ErrTransferConflict   = errors.New("skills: target skill already exists")
	ErrTransferSource     = errors.New("skills: transfer source changed since scan")
	ErrTransferOverlap    = errors.New("skills: transfer source and target overlap")
	ErrTransferUnsafeLink = errors.New("skills: transfer source contains an unsafe link")
	ErrTransferNotMovable = errors.New("skills: transfer source cannot be moved individually")
	ErrTransferPartial    = errors.New("skills: skill was copied but the source was not moved")
)

type TransferRequest struct {
	Skill  Skill
	Roots  []RootInfo
	Target Provider
	Move   bool
}

type TransferResult struct {
	Path                string
	SourceInSystemTrash bool
}

// CanMoveSkill reports whether moving this item cannot remove other skills.
func CanMoveSkill(skill Skill) bool {
	return skill.Scope == ScopeUser && skill.Deletable &&
		filepath.Clean(skill.Removal.Path) == filepath.Clean(skill.Dir) &&
		len(skill.Removal.Covers) <= 1
}

// TransferSkill copies one skill into another CLI's native user skills root.
// A move archives the source only after the complete copy is visible.
func TransferSkill(input Env, req TransferRequest, systemTrash func(string) error) (result TransferResult, err error) {
	env, err := normalizeEnv(input)
	if err != nil {
		return result, err
	}
	if req.Target == req.Skill.Provider {
		return result, ErrTransferOverlap
	}
	if req.Move && !CanMoveSkill(req.Skill) {
		return result, ErrTransferNotMovable
	}
	source, err := validateTransferSource(req.Skill)
	if err != nil {
		return result, err
	}
	sourceOpen := true
	defer func() {
		if sourceOpen {
			_ = source.root.Close()
		}
	}()
	targetRoot, err := transferTargetRoot(env, req.Target)
	if err != nil {
		return result, err
	}
	name := filepath.Base(filepath.Clean(req.Skill.Dir))
	if !validTransferName(name, req.Target) {
		return result, ErrTransferSource
	}
	resolvedTarget, err := resolveFuturePath(targetRoot)
	if err != nil {
		return result, err
	}
	if pathsOverlap(source.real, resolvedTarget) {
		return result, ErrTransferOverlap
	}
	if err := ensureTransferRoot(targetRoot); err != nil {
		return result, err
	}
	resolvedTarget, err = filepath.EvalSymlinks(targetRoot)
	if err != nil {
		return result, fmt.Errorf("skills: resolve transfer root: %w", err)
	}
	if pathsOverlap(source.real, resolvedTarget) {
		return result, ErrTransferOverlap
	}

	result.Path = filepath.Join(targetRoot, name)
	created, err := copySkill(source, targetRoot, name)
	if err != nil {
		return TransferResult{}, err
	}
	keep := false
	defer func() {
		if err != nil && !keep {
			err = errors.Join(err, created.cleanup())
		}
		_ = created.root.Close()
	}()
	if err = revalidateTransferSource(req.Skill, source); err != nil {
		return TransferResult{}, err
	}
	if !req.Move {
		keep = true
		return result, nil
	}
	if err = source.root.Close(); err != nil {
		keep = true
		return result, fmt.Errorf("%w: skills: close transfer source: %w", ErrTransferPartial, err)
	}
	sourceOpen = false
	trash := NewTrash(filepath.Join(env.Home, ".quosparr"), env.Home)
	_, result.SourceInSystemTrash, err = trash.MoveSkill(req.Skill, req.Roots, env.Now, systemTrash)
	if err != nil {
		keep = true
		return result, fmt.Errorf("%w: %w", ErrTransferPartial, err)
	}
	keep = true
	return result, nil
}

type transferSource struct {
	root     *os.Root
	real     string
	dirInfo  fs.FileInfo
	fileInfo fs.FileInfo
}

func validateTransferSource(skill Skill) (transferSource, error) {
	if filepath.Base(skill.File) != "SKILL.md" || filepath.Clean(skill.Dir) != filepath.Dir(filepath.Clean(skill.File)) {
		return transferSource{}, ErrTransferSource
	}
	realDir, err := filepath.EvalSymlinks(skill.Dir)
	if err != nil {
		return transferSource{}, fmt.Errorf("%w: %v", ErrTransferSource, err)
	}
	dirInfo, err := os.Stat(realDir)
	if err != nil || !dirInfo.IsDir() {
		return transferSource{}, fmt.Errorf("%w: %v", ErrTransferSource, err)
	}
	fileInfo, err := os.Stat(skill.File)
	if err != nil || !fileInfo.Mode().IsRegular() || !fileInfo.ModTime().Equal(skill.ModTime) {
		return transferSource{}, fmt.Errorf("%w: %v", ErrTransferSource, err)
	}
	source := transferSource{real: filepath.Clean(realDir), dirInfo: dirInfo, fileInfo: fileInfo}
	if err := revalidateTransferSource(skill, source); err != nil {
		return transferSource{}, err
	}
	root, err := os.OpenRoot(realDir)
	if err != nil {
		return transferSource{}, fmt.Errorf("%w: %v", ErrTransferSource, err)
	}
	openedDir, dirErr := root.Stat(".")
	openedFile, fileErr := root.Stat("SKILL.md")
	if dirErr != nil || fileErr != nil || !os.SameFile(dirInfo, openedDir) || !os.SameFile(fileInfo, openedFile) {
		_ = root.Close()
		return transferSource{}, ErrTransferSource
	}
	source.root = root
	return source, nil
}

func revalidateTransferSource(skill Skill, source transferSource) error {
	realFile, err := filepath.EvalSymlinks(skill.File)
	if err != nil || !within(source.real, realFile) || skill.RealFile != "" && filepath.Clean(realFile) != filepath.Clean(skill.RealFile) {
		return fmt.Errorf("%w: %v", ErrTransferSource, err)
	}
	dirInfo, dirErr := os.Stat(skill.Dir)
	fileInfo, fileErr := os.Stat(skill.File)
	if dirErr != nil || fileErr != nil || !os.SameFile(source.dirInfo, dirInfo) || !os.SameFile(source.fileInfo, fileInfo) || !fileInfo.ModTime().Equal(skill.ModTime) {
		return fmt.Errorf("%w: %v", ErrTransferSource, errors.Join(dirErr, fileErr))
	}
	return nil
}

func transferTargetRoot(env Env, provider Provider) (string, error) {
	switch provider {
	case ProviderCodex:
		return filepath.Join(env.CodexHome, "skills"), nil
	case ProviderClaude:
		return filepath.Join(env.ClaudeDir, "skills"), nil
	case ProviderGrok:
		return filepath.Join(env.GrokHome, "skills"), nil
	case ProviderGemini:
		return filepath.Join(env.GeminiHome, "config", "skills"), nil
	default:
		return "", ErrTransferProvider
	}
}

func validTransferName(name string, target Provider) bool {
	return name != "" && name != "." && name != ".." && !strings.HasPrefix(name, ".") && (target != ProviderClaude || name != "synced")
}
