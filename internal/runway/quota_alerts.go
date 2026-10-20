package runway

import (
	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
)

// deliverQuota keeps generation validation and persistence in the same lock
// scope, so a switch cannot record an old account's candidate as delivered.
func (s *Service) deliverQuota(auth codex.Auth, gen uint64, q codexapi.QuotaSnapshot) {
	s.mu.Lock()
	if !s.isCurrentLocked(auth, gen) || !s.prefs.QuotaAlertsEnabled || auth.IsAPIKey() {
		s.mu.Unlock()
		return
	}
	candidates := alerts.QuotaAlerts(quotaAlertInput(q), s.prefs.QuotaAlertThresholds)
	var err error
	if s.opts.AlertsPath != "" {
		candidates, err = (alerts.QuotaStore{Path: alerts.QuotaPath(s.opts.AlertsPath)}).Unseen(codex.MatchKey(auth), candidates)
	}
	if err != nil {
		s.lastErr = err.Error()
		s.publishLocked()
	}
	s.mu.Unlock()
	if err == nil && len(candidates) > 0 {
		s.emit(Event{Kind: EventAlerts, Alerts: candidates})
	}
}
