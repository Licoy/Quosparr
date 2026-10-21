package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
)

func (u *settingsUI) mcpDeleteDialog(c *ui.Context) {
	s := u.mcp
	if s == nil || !s.confirm {
		return
	}
	items := s.deleteTargets()
	l := u.L()
	message := tx(l, xMCPDeleteConfirmMessage, l.Text(providerKey(s.provider)), len(items))
	if ui.AlertDialog(c.Key("mcp-delete-confirm"), &s.confirm, tx(l, xMCPDeleteConfirmTitle), message,
		l.Text(l10n.KeyCancel), tx(l, xMCPDeleteSelected)) == 1 {
		u.deleteMCPServers(u.mcpActions(), items)
	}
	if !s.confirm {
		s.confirmKey = ""
	}
}

func (u *settingsUI) deleteMCPServers(actions MCPActions, servers []mcp.Server) {
	s := u.mcpState()
	if len(servers) == 0 || actions.Delete == nil || s.task.Running || s.work.Running || s.stale {
		return
	}
	provider := s.provider
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.generation++
	s.task = Task{}
	var results []mcp.DeleteResult
	runTask(&s.work, u.post, func() error {
		var err error
		results, err = actions.Delete(context.Background(), provider, servers)
		return err
	}, func(err error) {
		s.notice, s.noticeErr = mcpDeleteResultText(u.L(), results, err)
		if s.provider != provider {
			return
		}
		if s.applyDeleteResults(results) > 0 {
			s.stale = true
			u.startMCPScan(actions)
		}
	})
}

func (s *settingsMCP) applyDeleteResults(results []mcp.DeleteResult) int {
	removed := make(map[string]bool)
	for _, result := range results {
		if result.Err == nil {
			removed[result.Server.ID] = true
			delete(s.selected, result.Server.ID)
		}
	}
	if len(removed) == 0 {
		return 0
	}
	servers := s.snapshot.Servers[:0]
	for _, server := range s.snapshot.Servers {
		if !removed[server.ID] {
			servers = append(servers, server)
		}
	}
	s.snapshot.Servers = servers
	if removed[s.open] {
		s.open = ""
	}
	return len(removed)
}

func mcpDeleteResultText(l l10n.Localizer, results []mcp.DeleteResult, overall error) (string, bool) {
	if len(results) == 0 {
		return tx(l, xMCPDeleteFailed, overall), true
	}
	failed := 0
	var details []string
	for _, result := range results {
		if result.Err != nil {
			failed++
			details = append(details, fmt.Sprintf("%s: %s", result.Server.Name, result.Err))
		}
	}
	if failed > 0 || overall != nil {
		if overall != nil && len(details) == 0 {
			details = append(details, overall.Error())
		}
		return tx(l, xMCPDeletePartial, len(results)-failed, failed, strings.Join(details, "; ")), true
	}
	return tx(l, xMCPDeleteSuccess, len(results)), false
}

// toggleMCP writes one server's new state, then rescans: the write changes
// the snapshot fingerprint every later action relies on.
func (u *settingsUI) toggleMCP(actions MCPActions, server mcp.Server, enabled bool) {
	s := u.mcpState()
	if actions.SetEnabled == nil || !server.Toggleable || s.task.Running || s.work.Running || s.stale {
		return
	}
	provider := s.provider
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.generation++
	s.task = Task{}
	s.pending = map[string]bool{server.ID: enabled}
	runTask(&s.work, u.post, func() error {
		return actions.SetEnabled(context.Background(), provider, server, enabled)
	}, func(err error) {
		s.notice, s.noticeErr = mcpToggleText(u.L(), provider, server.Name, enabled, err)
		if s.provider != provider {
			return
		}
		if err != nil {
			s.pending = nil
		}
		s.stale = true
		u.startMCPScan(actions)
	})
}

func mcpToggleText(l l10n.Localizer, provider store.Provider, name string, enabled bool, err error) (string, bool) {
	if err != nil {
		return tx(l, xMCPToggleFailed, name, err), true
	}
	hint := tx(l, xMCPApplyCodex)
	if provider == store.ProviderGrok {
		hint = tx(l, xMCPApplyGrok)
	}
	if enabled {
		return tx(l, xMCPEnabledNotice, name, hint), false
	}
	return tx(l, xMCPDisabledNotice, name, hint), false
}
