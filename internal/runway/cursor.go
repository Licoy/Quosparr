package runway

import (
	"context"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

type CursorAvailability int

const (
	CursorDisabled CursorAvailability = iota
	CursorLoading
	CursorReady
	CursorNotLoggedIn
	CursorReauthRequired
	CursorFailed
)

// CursorState describes the current desktop account without switching or CLI actions.
// It contains no credential material; IdentityID is an opaque generation key.
type CursorState struct {
	Availability            CursorAvailability
	IdentityID, Email, Plan string
	Meters                  []Meter
	Lines                   []DetailLine
	UpdatedAt               time.Time
	Refreshing, Stale       bool
	LastError               string
}

// LiveMeters are the meters compact views may show: a stale reading is kept
// for the panel but never shown as a current percentage elsewhere.
func (s CursorState) LiveMeters() []Meter {
	if s.Stale {
		return nil
	}
	return s.Meters
}

// CursorProvider follows the same lock rules as ClaudeProvider. There is no
// RefreshLocal: this integration does not read editor conversations.
type CursorProvider interface {
	State(time.Time) CursorState
	Refresh(context.Context, bool) error
	SetOnChange(func())
}

func (s *Service) RefreshCursor(force bool) <-chan struct{} {
	s.mu.Lock()
	c := s.opts.Cursor
	wanted := s.prefs.CursorMonitoringEnabled && (force || s.quotaWantedLocked(store.ProviderCursor))
	s.mu.Unlock()
	if c == nil || !wanted {
		return closedChan()
	}
	return s.run(func() { _ = c.Refresh(s.root, force); s.publish() })
}

func CursorStatusText(l l10n.Localizer, s CursorState) string {
	if s.Stale {
		return l.Text(l10n.KeyStatusError)
	}
	if len(s.Meters) > 0 {
		return percentText(s.Meters[0])
	}
	switch s.Availability {
	case CursorDisabled:
		return l.Text(l10n.KeyCursorOff)
	case CursorLoading:
		return l.Text(l10n.KeyStatusWait)
	case CursorNotLoggedIn, CursorReauthRequired:
		return l.Text(l10n.KeyStatusLogin)
	case CursorReady:
		return l.Text(l10n.KeyStatusUnknown)
	default:
		return l.Text(l10n.KeyStatusError)
	}
}
