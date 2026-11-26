package app

import (
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestAlertDispatchBatchesOnceAndDropsStoppedEvents(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "deliver", true: "stop"}[stop], func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			delivered := make(chan runway.Event, 1)
			d := &alertDispatch{post: func(fn func()) { close(entered); <-release; fn() }, apply: func(e runway.Event) { delivered <- e }}
			d.publish(runway.Event{Kind: runway.EventAlerts, Alerts: []alerts.Alert{{ID: "a"}}})
			<-entered
			d.publish(runway.Event{Kind: runway.EventAlerts, Alerts: []alerts.Alert{{ID: "b"}}})
			if stop {
				d.close()
			}
			close(release)
			select {
			case e := <-delivered:
				if stop {
					t.Fatal("stopped alerts delivered")
				}
				if len(e.Alerts) != 2 || e.Alerts[0].ID != "a" || e.Alerts[1].ID != "b" {
					t.Fatal("alerts dropped or reordered")
				}
			case <-time.After(30 * time.Millisecond):
				if !stop {
					t.Fatal("alerts not delivered")
				}
			}
		})
	}
}

// Gemini reset alerts dedupe apart from the Codex ones and notify with the
// Gemini copy; Codex alerts keep their IDs and copy.
func TestGeminiResetAlertsUseTheirOwnIDsAndCopy(t *testing.T) {
	prefs := store.DefaultPreferences()
	prefs.Language = store.Language(l10n.English)
	gem := newGeminiResetBridge(prefs, nil)
	codex := newResetBridge(nil, prefs, nil)
	detected := alerts.Alert{ID: "rate-limit-reset:detected:2125851902009311306:1795651200", Kind: alerts.KindRateLimitResetDetected}
	gem.stash([]alerts.Alert{detected})
	codex.stash([]alerts.Alert{detected})
	g, c := gem.PendingAlerts(time.Now()), codex.PendingAlerts(time.Now())
	if len(g) != 1 || g[0].ID != "rate-limit-reset:gemini:detected:2125851902009311306:1795651200" {
		t.Fatalf("gemini alerts = %+v", g)
	}
	if len(c) != 1 || c[0].ID != detected.ID {
		t.Fatalf("codex alerts = %+v", c)
	}
	l := l10n.New(l10n.English)
	if n := alertNotification(l, g[0], time.Now()); n.Title != "Gemini reset detected" || !strings.Contains(n.Body, "Gemini") {
		t.Errorf("gemini notification = %+v", n)
	}
	if n := alertNotification(l, c[0], time.Now()); n.Title != "Codex reset detected" || !strings.Contains(n.Body, "Codex") {
		t.Errorf("codex notification = %+v", n)
	}
}
