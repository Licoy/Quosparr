package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/sessions"
)

func (u *settingsUI) sessionNotice(c *ui.Context) {
	s := u.sessionsState()
	listNotice(c, "session-notice-close", s.notice, s.noticeErr, u.text(l10n.KeyOk), func() { s.notice, s.noticeErr = "", false })
}

func (u *settingsUI) sessionDeleteDialog(c *ui.Context) {
	s := u.sessions
	if s == nil || !s.confirm {
		return
	}
	items := s.deleteTargets()
	l := u.L()
	message := tx(l, xSessionsDeleteConfirmMessage, l.Text(providerKey(s.provider)), len(items))
	if ui.AlertDialog(c.Key("session-delete-confirm"), &s.confirm, tx(l, xSessionsDeleteConfirmTitle), message,
		l.Text(l10n.KeyCancel), tx(l, xSessionsDeleteSelected)) == 1 {
		u.deleteSessions(u.sessionActions(), items)
	}
	if !s.confirm {
		s.confirmKey = ""
	}
}

func (u *settingsUI) deleteSessions(actions SessionActions, items []sessions.ManagedSession) {
	s := u.sessionsState()
	if len(items) == 0 || actions.Delete == nil || s.work.Running {
		return
	}
	provider := s.provider
	var results []sessions.DeleteResult
	runTask(&s.work, u.post, func() error {
		var err error
		results, err = actions.Delete(context.Background(), items)
		return err
	}, func(err error) {
		s.clearSelection()
		s.open = ""
		s.notice, s.noticeErr = sessionDeleteResultText(u.L(), results, err)
		if s.provider == provider {
			u.startSessionsScan(actions)
		}
	})
}

// geminiSessionRefusal is the text of a deletion the Gemini scanner refused
// as a whole; such a refusal changes nothing.
func geminiSessionRefusal(l l10n.Localizer, err error) (string, bool) {
	switch {
	case errors.Is(err, gemini.ErrSessionClientRunning):
		return tx(l, xSessionsGeminiRunning), true
	case errors.Is(err, gemini.ErrSessionBackup):
		return tx(l, xSessionsGeminiBackup), true
	case errors.Is(err, gemini.ErrSessionSelection):
		return tx(l, xSessionsGeminiSelection), true
	}
	return "", false
}

func sessionDeleteResultText(l l10n.Localizer, results []sessions.DeleteResult, overall error) (string, bool) {
	if text, ok := geminiSessionRefusal(l, overall); ok {
		return text, true
	}
	if len(results) == 0 {
		return tx(l, xSessionsDeleteFailed, overall), true
	}
	failed, removed, total := 0, 0, 0
	var details []string
	for _, result := range results {
		removed += result.Removed
		total += result.Total
		if result.Err == nil {
			continue
		}
		failed++
		name := result.Session.Title
		if name == "" {
			name = filepath.Base(result.Session.Path)
		}
		details = append(details, fmt.Sprintf("%s (%d/%d): %s", name, result.Removed, result.Total, result.Err))
	}
	if failed > 0 || overall != nil {
		if overall != nil && len(details) == 0 {
			details = append(details, overall.Error())
		}
		return tx(l, xSessionsDeletePartial, len(results)-failed, failed, removed, total, strings.Join(details, "; ")), true
	}
	return tx(l, xSessionsDeleteSuccess, len(results), removed), false
}
