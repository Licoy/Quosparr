package codexdesktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// These fields contain local metadata only; deletion/migration operations,
// cloud bindings, selection, writable roots and remote hosts are not copied.
var projectGlobals = []string{
	"local-projects", "app-server-project-id-by-legacy-project-id-by-host",
	"project-order", "pinned-project-ids", "project-appearances",
	"sidebar-project-thread-orders", "thread-project-assignments",
	"thread-project-membership-host-ids", "pinned-thread-ids",
	"electron-saved-workspace-roots", "electron-workspace-root-labels",
}
var projectAtoms = []string{"unified-sidebar-project-order-v1"}

type selection struct {
	part                     partition
	projects, threads, roots map[string]bool
	summary                  Summary
	names                    []string
	host                     string
}

func (v *selection) skip(reason string) {
	v.summary.Skipped++
	for _, r := range v.summary.Reasons {
		if r == reason {
			return
		}
	}
	v.summary.Reasons = append(v.summary.Reasons, reason)
}
func validated(p partition, cat catalog, home string) (partition, Summary, []string, error) {
	v := selection{part: partition{object{}, object{}}, projects: map[string]bool{}, threads: map[string]bool{}, roots: map[string]bool{}, host: "local:" + home}
	projects, ok := decodeObject(p.Globals["local-projects"])
	if !ok {
		return v.part, v.summary, nil, ErrUnknownFormat
	}
	hosts, ok := decodeObject(p.Globals["app-server-project-id-by-legacy-project-id-by-host"])
	if !ok {
		return v.part, v.summary, nil, ErrUnknownFormat
	}
	mapping, ok := decodeObject(hosts[v.host])
	if !ok {
		return v.part, v.summary, nil, ErrUnknownFormat
	}
	selected, selectedMapping := object{}, object{}
	for _, id := range sortedKeys(projects) {
		item, ok := decodeObject(projects[id])
		if !ok || item == nil {
			return v.part, v.summary, nil, ErrUnknownFormat
		}
		if cloudProject(id, item) {
			v.skip("cloud_project")
			continue
		}
		var itemID string
		if json.Unmarshal(item["id"], &itemID) != nil || itemID != id {
			return v.part, v.summary, nil, ErrConflict
		}
		var dbID string
		if json.Unmarshal(mapping[id], &dbID) != nil || dbID == "" {
			v.skip("missing_project_mapping")
			continue
		}
		dbp, found := cat.Projects[dbID]
		if !found {
			v.skip("missing_project")
			continue
		}
		var roots []string
		if json.Unmarshal(item["rootPaths"], &roots) != nil || len(roots) == 0 {
			v.skip("project_roots_mismatch")
			continue
		}
		valid := true
		for i, root := range roots {
			info, err := os.Stat(root)
			if err != nil || !info.IsDir() {
				valid = false
			}
			if !filepath.IsAbs(root) {
				valid = false
			}
			roots[i] = filepath.Clean(root)
		}
		if !valid || !reflect.DeepEqual(roots, dbp.Roots) {
			v.skip("project_roots_mismatch")
			continue
		}
		clean := object{}
		for _, key := range []string{"id", "name", "rootPaths", "createdAt", "updatedAt"} {
			if raw, ok := item[key]; ok {
				clean[key] = raw
			}
		}
		selected[id], selectedMapping[id] = encode(clean), encode(dbID)
		v.projects[id] = true
		v.names = append(v.names, dbp.Name)
		v.summary.Projects++
		for _, root := range roots {
			v.roots[root] = true
		}
	}
	if len(selected) > 0 {
		v.part.Globals["local-projects"] = encode(selected)
		v.part.Globals["app-server-project-id-by-legacy-project-id-by-host"] = encode(object{v.host: encode(selectedMapping)})
	}
	if err := v.selectThreads(p, cat); err != nil {
		return v.part, v.summary, nil, err
	}
	if err := v.selectMetadata(p); err != nil {
		return v.part, v.summary, nil, err
	}
	sort.Strings(v.summary.Reasons)
	return v.part, v.summary, v.names, nil
}
func cloudProject(id string, item object) bool {
	if strings.HasPrefix(id, "g-p-") {
		return true
	}
	for _, key := range []string{"chatGptBacking", "chatGptBackingCreation", "retiredChatGptBacking"} {
		if raw := item[key]; raw != nil && string(raw) != "null" {
			return true
		}
	}
	var pending bool
	_ = json.Unmarshal(item["chatGptBackingRecoveryPending"], &pending)
	return pending
}
func (v *selection) selectThreads(p partition, cat catalog) error {
	assignments, ok := decodeObject(p.Globals["thread-project-assignments"])
	if !ok {
		return ErrUnknownFormat
	}
	hosts, ok := decodeObject(p.Globals["thread-project-membership-host-ids"])
	if !ok {
		return ErrUnknownFormat
	}
	projectless := map[string]bool{}
	if raw := p.Globals["projectless-thread-ids"]; raw != nil {
		var ids []string
		if json.Unmarshal(raw, &ids) != nil {
			return ErrUnknownFormat
		}
		for _, id := range ids {
			projectless[id] = true
		}
	}
	selected, selectedHosts := object{}, object{}
	for _, thread := range sortedKeys(assignments) {
		if projectless[thread] {
			v.skip("projectless_thread")
			continue
		}
		item, ok := decodeObject(assignments[thread])
		if !ok || item == nil {
			return ErrUnknownFormat
		}
		var kind, id, host, origin string
		if json.Unmarshal(item["projectKind"], &kind) != nil || json.Unmarshal(item["projectId"], &id) != nil {
			return ErrUnknownFormat
		}
		if raw := item["hostId"]; raw != nil && json.Unmarshal(raw, &host) != nil {
			return ErrUnknownFormat
		}
		if raw := item["projectOrigin"]; raw != nil && json.Unmarshal(raw, &origin) != nil {
			return ErrUnknownFormat
		}
		var membership string
		if raw := hosts[thread]; raw != nil && json.Unmarshal(raw, &membership) != nil {
			return ErrUnknownFormat
		}
		if kind != "local" || origin == "chatgpt" || (host != "" && host != "local") || (membership != "" && membership != "local") {
			v.skip("remote_thread")
			continue
		}
		if !v.projects[id] {
			v.skip("unmapped_thread")
			continue
		}
		if !cat.Threads[thread] {
			v.skip("missing_thread")
			continue
		}
		clean := object{"projectKind": encode("local"), "projectId": encode(id)}
		if host != "" {
			clean["hostId"] = encode(host)
		}
		selected[thread] = encode(clean)
		v.threads[thread] = true
		v.summary.Threads++
		if membership != "" {
			selectedHosts[thread] = encode(membership)
		}
	}
	if len(selected) > 0 {
		v.part.Globals["thread-project-assignments"] = encode(selected)
	}
	if len(selectedHosts) > 0 {
		v.part.Globals["thread-project-membership-host-ids"] = encode(selectedHosts)
	}
	return nil
}
func (v *selection) selectMetadata(p partition) error {
	for _, key := range []string{"project-order", "pinned-project-ids"} {
		if err := copyIDs(v.part.Globals, p.Globals, key, v.projects); err != nil {
			return err
		}
	}
	if err := copyIDs(v.part.Globals, p.Globals, "pinned-thread-ids", v.threads); err != nil {
		return err
	}
	if err := copyIDs(v.part.Globals, p.Globals, "electron-saved-workspace-roots", v.roots); err != nil {
		return err
	}
	for _, key := range []string{"project-appearances", "electron-workspace-root-labels", "sidebar-project-thread-orders"} {
		values, ok := decodeObject(p.Globals[key])
		if !ok {
			return ErrUnknownFormat
		}
		selected := object{}
		for id, raw := range values {
			allowed := v.projects[id]
			if key == "electron-workspace-root-labels" {
				allowed = v.roots[id]
			}
			if !allowed {
				continue
			}
			if key == "sidebar-project-thread-orders" {
				tmp := object{key: raw}
				out := object{}
				if err := copyIDs(out, tmp, key, v.threads); err != nil {
					return err
				}
				raw = out[key]
				if raw == nil {
					continue
				}
			}
			selected[id] = raw
		}
		if len(selected) > 0 {
			v.part.Globals[key] = encode(selected)
		}
	}
	// Unified sidebar keys are an app-specific encoding; copy only keys whose
	// supported local-project encoding has been verified, never opaque values.
	sidebarKeys := map[string]bool{}
	for id := range v.projects {
		sidebarKeys["codex:project:"+id] = true
	}
	if err := copyIDs(v.part.Atoms, p.Atoms, "unified-sidebar-project-order-v1", sidebarKeys); err != nil {
		return err
	}
	return nil
}
func copyIDs(dst, src object, key string, allowed map[string]bool) error {
	raw, found := src[key]
	if !found {
		return nil
	}
	var ids []string
	if json.Unmarshal(raw, &ids) != nil {
		return ErrUnknownFormat
	}
	out := []string{}
	for _, id := range ids {
		if allowed[id] {
			out = append(out, id)
		}
	}
	if len(out) > 0 {
		dst[key] = encode(out)
	}
	return nil
}
func sortedKeys(o object) []string {
	keys := make([]string, 0, len(o))
	for key := range o {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
