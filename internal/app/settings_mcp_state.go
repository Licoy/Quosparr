package app

import (
	"context"
	"slices"
	"strings"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
)

type settingsMCP struct {
	provider   store.Provider
	search     string
	task       Task
	cancel     context.CancelFunc
	generation uint64
	snapshot   mcp.Snapshot
	loaded     bool
	stale      bool
	selected   map[string]bool
	open       string
	confirm    bool
	// confirmKey names the one server a row or detail page asked to
	// delete; empty, the confirmation deletes the selection.
	confirmKey string
	work       Task
	// pending holds the state a switch was just turned to until the
	// rescan after the write reports it.
	pending   map[string]bool
	notice    string
	noticeErr bool
}

func (u *settingsUI) mcpState() *settingsMCP {
	if u.mcp == nil {
		u.mcp = &settingsMCP{provider: store.ProviderCodex, selected: map[string]bool{}}
	}
	return u.mcp
}

func (u *settingsUI) mcpActions() MCPActions {
	if u.opts.MCP == nil {
		return MCPActions{}
	}
	return u.opts.MCP()
}

func (s *settingsMCP) switchProvider(provider store.Provider) {
	if s.provider == provider {
		return
	}
	s.provider, s.search, s.open = provider, "", ""
	s.snapshot, s.loaded, s.stale, s.notice, s.noticeErr = mcp.Snapshot{}, false, false, "", false
	s.selected, s.confirm, s.confirmKey, s.pending = map[string]bool{}, false, "", nil
}

// confirmDelete asks to delete one server, or the selection for "".
func (s *settingsMCP) confirmDelete(id string) {
	s.confirm, s.confirmKey = true, id
}

// deleteTargets are the servers the open confirmation deletes.
func (s *settingsMCP) deleteTargets() []mcp.Server {
	if s.confirmKey == "" {
		return s.selectedServers()
	}
	if server, ok := s.find(s.confirmKey); ok && server.Deletable {
		return []mcp.Server{server}
	}
	return nil
}

func (s *settingsMCP) filtered(l l10n.Localizer) []mcp.Server {
	query := strings.ToLower(strings.TrimSpace(s.search))
	result := make([]mcp.Server, 0, len(s.snapshot.Servers))
	for _, server := range s.snapshot.Servers {
		if query == "" || containsAnyFold(query, server.Name, server.Transport, server.Command, server.URL, mcpProblemText(l, server.Problem)) {
			result = append(result, server)
		}
	}
	slices.SortStableFunc(result, func(a, b mcp.Server) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return result
}

func (s *settingsMCP) selectedServers() []mcp.Server {
	var result []mcp.Server
	for _, server := range s.snapshot.Servers {
		if s.selected[server.ID] && server.Deletable {
			result = append(result, server)
		}
	}
	return result
}

// pruneSelection keeps the selected servers a rescan still lists; an open
// server that is gone closes in mcpPage.
func (s *settingsMCP) pruneSelection() {
	for id := range s.selected {
		if _, ok := s.find(id); !ok {
			delete(s.selected, id)
		}
	}
}

func (u *settingsUI) startMCPScan(actions MCPActions) {
	s := u.mcpState()
	if s.cancel != nil {
		s.cancel()
	}
	s.generation++
	gen, provider := s.generation, s.provider
	if actions.Query == nil {
		s.task = Task{Err: errDetailUnavailable}
		return
	}
	s.task = Task{Running: true}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		snapshot, err := actions.Query(ctx, provider)
		u.post(func() {
			if ctx.Err() != nil || gen != s.generation || provider != s.provider {
				return
			}
			s.cancel, s.task = nil, Task{Err: err}
			if err == nil {
				s.snapshot, s.loaded, s.stale, s.pending = snapshot, true, false, nil
				s.pruneSelection()
			}
		})
	}()
}

// cancelMCPTasks stops read-only scans. A confirmed delete keeps running so
// its real result can still be reported.
func (u *settingsUI) cancelMCPTasks() {
	if u.mcp == nil {
		return
	}
	if u.mcp.cancel != nil {
		u.mcp.cancel()
		u.mcp.cancel = nil
	}
	u.mcp.generation++
}
