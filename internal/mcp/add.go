package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

type AddItem struct{ Name, Status, Details string }
type AddPlan struct {
	Provider         Provider
	Items            []AddItem
	source           Source
	original, output []byte
	info, parent     fs.FileInfo
	ancestor         string
	exists           bool
	added, existing  []string
}
type AddResult struct{ Added, Existing []string }

// Path is the configuration file the plan writes.
func (p AddPlan) Path() string { return p.source.Path }

func PrepareAdd(ctx context.Context, source Source, drafts []Draft) (AddPlan, error) {
	var plan AddPlan
	if err := ctx.Err(); err != nil {
		return plan, err
	}
	if err := validateSource(source, true); err != nil {
		return plan, draftError("invalid", "source")
	}
	if len(drafts) == 0 || len(drafts) > 100 {
		return plan, draftError("size", "drafts")
	}
	ancestor, parent, err := addAncestor(source.Path)
	if err != nil {
		return plan, err
	}
	data, info, exists, err := readRegular(source.Path)
	if err != nil {
		return plan, draftError("read", "config")
	}
	root, servers, err := addDocument(source.Provider, data)
	if err != nil {
		return plan, err
	}
	plan = AddPlan{Provider: source.Provider, source: source, original: bytes.Clone(data), info: info, parent: parent, ancestor: ancestor, exists: exists}
	seen := map[string]bool{}
	additions := map[string]any{}
	for _, d := range drafts {
		if seen[d.Name] {
			return AddPlan{}, draftError("duplicate", "name")
		}
		seen[d.Name] = true
		config, err := draftConfig(source.Provider, d)
		if err != nil {
			return AddPlan{}, err
		}
		if source.Provider == ProviderGrok {
			disabled, valid := disabledList(root)
			if _, present := root["disabled_mcp_servers"]; present && !valid {
				return AddPlan{}, draftError("invalid", "disabled_mcp_servers")
			}
			if slices.Contains(disabled, d.Name) {
				return AddPlan{}, draftError("conflict", "disabled_mcp_servers")
			}
		}
		summary := make(map[string]any, len(config))
		for key, value := range config {
			switch v := value.(type) {
			case map[string]string:
				fields := make(map[string]any, len(v))
				for name, item := range v {
					fields[name] = item
				}
				summary[key] = fields
			case []string:
				items := make([]any, len(v))
				for i, item := range v {
					items[i] = item
				}
				summary[key] = items
			default:
				summary[key] = value
			}
		}
		details, err := redactedDetails(summary)
		if err != nil {
			return AddPlan{}, draftError("invalid", "server")
		}
		status := "add"
		if old, present := servers[d.Name]; present {
			if !equivalentServer(old, config) {
				return AddPlan{}, draftError("conflict", "name")
			}
			status = "exists"
			plan.existing = append(plan.existing, d.Name)
		} else {
			additions[d.Name] = config
			plan.added = append(plan.added, d.Name)
		}
		plan.Items = append(plan.Items, AddItem{Name: d.Name, Status: status, Details: details})
	}
	plan.output, err = encodeAdd(source.Provider, data, root, servers, additions)
	if len(plan.output) > maxConfigSize {
		return AddPlan{}, draftError("size", "config")
	}
	if err != nil {
		return AddPlan{}, err
	}
	return plan, nil
}

func Add(ctx context.Context, source Source, plan AddPlan, now time.Time) (AddResult, error) {
	result := AddResult{}
	if source != plan.source || source.Provider != plan.Provider || plan.parent == nil {
		return result, draftError("invalid", "plan")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		ancestor, parent, err := addAncestor(source.Path)
		if err != nil || ancestor != plan.ancestor || !os.SameFile(parent, plan.parent) {
			return draftError("changed", "parent")
		}
		data, info, exists, err := readRegular(source.Path)
		if err != nil || exists != plan.exists || exists && (!os.SameFile(info, plan.info) || fileFingerprint(data, info) != fileFingerprint(plan.original, plan.info)) {
			return draftError("changed", "config")
		}
		return nil
	}
	if err := check(); err != nil {
		return result, err
	}
	if len(plan.added) == 0 {
		return AddResult{Existing: slices.Clone(plan.existing)}, nil
	}
	if err := validateBackupDir(source.BackupDir); err != nil {
		return result, draftError("backup", "config")
	}
	if plan.ancestor != filepath.Dir(source.Path) {
		parent, err := createAddParents(source.Path, plan.ancestor, plan.parent)
		if err != nil {
			return result, err
		}
		plan.ancestor = filepath.Dir(source.Path)
		plan.parent = parent
	}
	policy := platform.BackupPolicy{Dir: source.BackupDir, Prefix: string(source.Provider) + "-mcp-config-", Ext: filepath.Ext(source.Path), Keep: 10}
	err := platform.EditPrivateFileWithBackupChecked(source.Path, policy, now, func(old []byte) ([]byte, error) {
		if err := check(); err != nil {
			return nil, err
		}
		if !bytes.Equal(old, plan.original) {
			return nil, draftError("changed", "config")
		}
		return bytes.Clone(plan.output), nil
	}, check)
	if err != nil {
		var detail *DraftError
		if errors.As(err, &detail) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return result, err
		}
		return result, draftError("write", "config")
	}
	written, _, exists, err := readRegular(source.Path)
	if err != nil || !exists || !bytes.Equal(written, plan.output) {
		return result, draftError("verification", "written_config")
	}
	return AddResult{Added: slices.Clone(plan.added), Existing: slices.Clone(plan.existing)}, nil
}

// addAncestor records the closest existing directory without following a final symlink.
func addAncestor(path string) (string, fs.FileInfo, error) {
	for current := filepath.Dir(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", nil, draftError("unsafe", "parent")
			}
			// Windows Lstat loads the file ID lazily; capture it before the
			// directory can be replaced while the user reviews the plan.
			if !os.SameFile(info, info) {
				return "", nil, draftError("unsafe", "parent")
			}
			return current, info, nil
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(current) == current {
			return "", nil, draftError("unsafe", "parent")
		}
	}
}

func createAddParents(path, ancestor string, identity fs.FileInfo) (fs.FileInfo, error) {
	current, info, err := addAncestor(path)
	if err != nil || current != ancestor || !os.SameFile(info, identity) {
		return nil, draftError("changed", "parent")
	}
	var missing []string
	for dir := filepath.Dir(path); dir != ancestor; dir = filepath.Dir(dir) {
		missing = append(missing, dir)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		before, err := os.Lstat(current)
		if err != nil || !os.SameFile(before, info) || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
			return nil, draftError("changed", "parent")
		}
		if platform.CreatePrivateDirectory(missing[i]) != nil {
			return nil, draftError("changed", "parent")
		}
		if platform.VerifyOwnerOnly(missing[i], 0700) != nil {
			return nil, draftError("permissions", "parent")
		}
		current = missing[i]
		info, err = os.Lstat(current)
		// Freeze newly created directory identities for the next recheck too.
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, info) {
			return nil, draftError("changed", "parent")
		}
	}
	return info, nil
}
func sameConfig(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}
