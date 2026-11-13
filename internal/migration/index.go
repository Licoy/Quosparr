package migration

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/platform"
)

func copyIndex(source, target string, exists bool) error {
	old, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := validateIndex(old, filepath.Base(source)); err != nil {
		return err
	}
	if !exists {
		return copyPrivateFile(source, target)
	}
	current, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if err := validateIndex(current, filepath.Base(target)); err != nil {
		return err
	}
	merged, err := mergeIndex(old, current)
	if err != nil {
		return err
	}
	return platform.WriteFileAtomic(target, merged, 0o600)
}

func validateIndex(data []byte, name string) error {
	version := 0
	var err error
	if name == "grok-index.json" {
		var idx grok.AccountIndex
		idx, err = grok.ParseAccountIndex(data)
		version = idx.Version
	} else if name == "claude-index.json" {
		var idx claude.AccountIndex
		idx, err = claude.ParseAccountIndex(data)
		version = idx.Version
	} else if name == "gemini-index.json" {
		var idx gemini.AccountIndex
		idx, err = gemini.ParseAccountIndex(data)
		version = idx.Version
	} else {
		var idx codex.AccountIndex
		idx, err = codex.ParseIndex(data)
		version = idx.Version
	}
	// Never include decoder output: malformed external fields can contain secrets.
	if err != nil || version != 1 {
		return errors.New("cannot migrate invalid or unsupported account index")
	}
	var doc indexDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return errors.New("invalid account index")
	}
	seen := map[string]bool{}
	for _, row := range doc.Accounts {
		id, _, err := rowIdentity(row)
		if err != nil || seen[id] {
			return errors.New("invalid or duplicate account identity")
		}
		seen[id] = true
	}
	return nil
}

type indexDocument struct {
	Version  int                          `json:"version"`
	Accounts []map[string]json.RawMessage `json:"accounts"`
}

// Keep the new root fields, current selection and duplicate-ID metadata exactly
// as stored. Append missing rows after the existing order; retain unknown fields.
func mergeIndex(old, current []byte) ([]byte, error) {
	var previous, next indexDocument
	if err := json.Unmarshal(old, &previous); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(current, &next); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	order := -1
	for _, row := range next.Accounts {
		id, sortIndex, err := rowIdentity(row)
		if err != nil || seen[id] {
			return nil, errors.New("invalid or duplicate new account identity")
		}
		seen[id] = true
		order = max(order, sortIndex)
	}
	oldIDs := map[string]bool{}
	for _, row := range previous.Accounts {
		id, _, err := rowIdentity(row)
		if err != nil || oldIDs[id] {
			return nil, errors.New("invalid or duplicate previous account identity")
		}
		oldIDs[id] = true
		if seen[id] {
			continue
		}
		if order == math.MaxInt {
			return nil, errors.New("account order exceeds supported range")
		}
		order++
		row["sortIndex"], _ = json.Marshal(order)
		next.Accounts = append(next.Accounts, row)
	}
	if len(next.Accounts) == len(seen) {
		return current, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(current, &root); err != nil {
		return nil, err
	}
	root["accounts"], _ = json.Marshal(next.Accounts)
	return json.MarshalIndent(root, "", "  ")
}

func rowIdentity(row map[string]json.RawMessage) (string, int, error) {
	var id string
	var order int
	if err := json.Unmarshal(row["id"], &id); err != nil {
		return "", 0, err
	}
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", 0, errors.New("invalid account ID")
	}
	if err := json.Unmarshal(row["sortIndex"], &order); err != nil {
		return "", 0, err
	}
	return id, order, nil
}
