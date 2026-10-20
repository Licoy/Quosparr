package runway

import (
	"context"
	"os"
	"testing"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/codexapi"
)

func TestQuotaThresholdsApplyInBothRefreshPaths(t *testing.T) {
	f := newFake(t)
	used := 99.6
	f.quotaFn = func(context.Context, codexapi.Auth) (codexapi.QuotaSnapshot, error) {
		q := f.quota("p", 100)
		q.Primary.UsedPercentExact = used
		q.Secondary = nil
		return q, nil
	}
	s, r := newService(t, f, func(o *Options) {
		o.Preferences.QuotaAlertsEnabled = true
		o.Preferences.QuotaAlertThresholds = []int{100}
	})
	refreshAll(t, s)
	if r.alertCount() != 0 {
		t.Fatal("full refresh rounded 99.6 to 100")
	}
	s.refreshQuotaNow(context.Background())
	if r.alertCount() != 0 {
		t.Fatal("quota refresh rounded 99.6 to 100")
	}
	used = 100
	s.refreshQuotaNow(context.Background())
	if r.alertCount() != 1 {
		t.Fatalf("quota refresh: %d", r.alertCount())
	}
	refreshAll(t, s)
	if r.alertCount() != 1 {
		t.Fatal("full refresh repeated quota-only alert")
	}
}

func TestDisabledAndStaleQuotaDoNotWriteProgress(t *testing.T) {
	f := newFake(t)
	s, r := newService(t, f, nil)
	refreshAll(t, s)
	s.mu.Lock()
	auth, gen := *s.auth, s.epoch
	s.mu.Unlock()
	q := f.quota("p", 100)
	s.deliverQuota(auth, gen, q)
	path := alerts.QuotaPath(s.opts.AlertsPath)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled state write: %v", err)
	}
	s.mu.Lock()
	s.prefs.QuotaAlertsEnabled = true
	s.epoch++
	s.mu.Unlock()
	s.deliverQuota(auth, gen, q)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale state write: %v", err)
	}
	if r.alertCount() != 0 {
		t.Fatal("disabled/stale alert delivered")
	}
}

func TestQuotaProgressSurvivesPreferenceChanges(t *testing.T) {
	f := newFake(t)
	s, r := newService(t, f, func(o *Options) { o.Preferences.QuotaAlertsEnabled = true })
	refreshAll(t, s)
	s.mu.Lock()
	auth, gen, p := *s.auth, s.epoch, s.prefs
	s.mu.Unlock()
	q := f.quota("p", 96)
	q.Secondary = nil
	s.deliverQuota(auth, gen, q)
	if r.alertCount() != 1 {
		t.Fatal("missing 95 alert")
	}
	p.QuotaAlertThresholds = []int{0, 80, 90, 100}
	s.SetPreferences(p)
	q.Primary.UsedPercentExact = 90
	s.deliverQuota(auth, gen, q)
	if r.alertCount() != 1 {
		t.Fatal("preference save reset progress")
	}
	q.Primary.UsedPercentExact = 100
	s.deliverQuota(auth, gen, q)
	if r.alertCount() != 2 {
		t.Fatal("higher configured tier did not notify")
	}
}

func TestQuotaAccountsUseFullIdentity(t *testing.T) {
	for _, workspace := range []string{"shared", ""} {
		t.Run(workspace, func(t *testing.T) {
			f := newFake(t)
			s, r := newService(t, f, func(o *Options) { o.Preferences.QuotaAlertsEnabled = true })
			q := f.quota("p", 96)
			q.Secondary = nil
			for _, email := range []string{"one@example.invalid", "two@example.invalid", "one@example.invalid"} {
				auth := testAuth(t, workspace, email, email)
				s.mu.Lock()
				s.clearAccountScopedLocked(&auth)
				gen := s.epoch
				s.mu.Unlock()
				s.deliverQuota(auth, gen, q)
			}
			if r.alertCount() != 2 {
				t.Fatalf("got %d alerts for two identities", r.alertCount())
			}
		})
	}
}
