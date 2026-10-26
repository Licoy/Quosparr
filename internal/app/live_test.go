package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

const liveCredID = "0F1E2D3C-4B5A-6978-8899-AABBCCDDEEFF"

type recordingCreds struct {
	allows []bool
	item   netx.Credentials
	err    error
}

func (s *recordingCreds) Load(id string, allow bool) (netx.Credentials, error) {
	s.allows = append(s.allows, allow)
	if s.err != nil {
		return netx.Credentials{}, s.err
	}
	if id != liveCredID {
		return netx.Credentials{}, netx.ErrCredentialsUnavailable
	}
	return s.item, nil
}

func (s *recordingCreds) Save(netx.Credentials, string) error { return s.err }
func (s *recordingCreds) Delete(string) error                 { return s.err }

func TestPlatformCredentialsInteractOnce(t *testing.T) {
	raw := &recordingCreds{item: netx.Credentials{Username: "proxy-user", Password: "proxy-pass"}}
	p := newPlatformCredentials(raw)
	if _, err := p.Netx().Load(liveCredID, true); err != nil {
		t.Fatal(err)
	}
	got, err := p.Load(liveCredID)
	if err != nil || got.Username != "proxy-user" || got.Password != "proxy-pass" {
		t.Fatalf("later load allow interaction or mismatch: %v", err)
	}
	if _, err := p.Netx().Load(liveCredID, true); err != nil {
		t.Fatal(err)
	}
	if len(raw.allows) != 3 || !raw.allows[0] || raw.allows[1] || raw.allows[2] {
		t.Fatalf("allows = %v", raw.allows)
	}
}

func TestPlatformCredentialsMapStoreErrors(t *testing.T) {
	raw := &netx.MemoryCredentialStore{Err: netx.ErrCredentialStoreFailed}
	p := newPlatformCredentials(raw)
	if err := p.Save(liveCredID, ProxyCredentials{Username: "u", Password: "p"}); !errors.Is(err, ErrCredentialStoreFailed) {
		t.Fatalf("save err = %v", err)
	}
	if _, err := p.Load(liveCredID); !errors.Is(err, ErrCredentialStoreFailed) {
		t.Fatalf("load err = %v", err)
	}
	missing := newPlatformCredentials(&netx.MemoryCredentialStore{})
	if _, err := missing.Load(liveCredID); !errors.Is(err, store.ErrProxyCredentialsUnavailable) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestTrayFromStatusKeepsTheComponents(t *testing.T) {
	view := runway.StatusBarView{
		Style: store.StatusBarStyleBattery, Battery: store.BatteryVertical, HideIcons: true, Tooltip: "Quosparr\nCodex: 5h 25%",
		Components: []runway.StatusBarComponent{{Provider: store.ProviderCodex, Cells: []runway.StatusBarCell{{Provider: store.ProviderCodex, Remaining: 25}}}},
	}
	got := TrayFromStatus(view)
	want := TrayRender{Style: view.Style, Battery: view.Battery, HideIcons: true, Components: view.Components, ToolTip: view.Tooltip}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tray = %#v", got)
	}
}

func TestAlertNotificationUsesExistingKeys(t *testing.T) {
	l := l10n.ForPreference(string(l10n.English), nil)
	threshold := 80
	got := alertNotification(l, alerts.Alert{
		ID: "quota:5-hour:80:0", Kind: alerts.KindQuota, Name: "5-hour", Threshold: &threshold,
	}, time.Now())
	if got.Title != l.Text(l10n.KeyQuotaAlertTitle) || !strings.Contains(got.Body, "80%") || !strings.Contains(got.Body, l.Text(l10n.KeyFiveHourUsage)) {
		t.Fatalf("notification = %#v", got)
	}
	if got.ID != "quota:5-hour:80:0" {
		t.Fatalf("id = %s", got.ID)
	}
}

func TestProbeProxyDoesNotInstallTheDraft(t *testing.T) {
	netx.Block(errors.New("held"))
	t.Cleanup(func() { netx.Configure(netx.SystemContext()) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	err := probeProxy(context.Background(), store.DefaultNetworkProxy(), nil, srv.URL, netx.WithSystemProxy(func(*url.URL) (*url.URL, error) {
		return nil, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := netx.Current(); err == nil {
		t.Fatal("probe replaced the blocked network context")
	}
}

func TestProbeProxyAcceptsAnyAnswerFromTheSite(t *testing.T) {
	direct := netx.WithSystemProxy(func(*url.URL) (*url.URL, error) { return nil, nil })
	for status, ok := range map[int]bool{
		http.StatusOK: true, http.StatusNotFound: true, http.StatusForbidden: true,
		http.StatusProxyAuthRequired: false, http.StatusBadGateway: false,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		err := probeProxy(context.Background(), store.DefaultNetworkProxy(), nil, srv.URL, direct)
		srv.Close()
		if (err == nil) != ok {
			t.Errorf("status %d: err = %v", status, err)
		}
	}
}

func TestAccountsPageMasksAndDropsThePlaceholder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GROK_HOME", filepath.Join(home, "grok"))
	e := newEnv(t, nil)
	rt := newRuntime(BackgroundOptions{Home: home, Settings: e.s, Version: "test"})
	setLive(rt)
	t.Cleanup(func() {
		rt.svc.Close()
		if rt.repo != nil {
			_ = rt.repo.Close()
		}
		setLive(nil)
	})
	acc, err := rt.codex.Upsert(codex.APIKeyAuth("sk-test-synthetic"), codex.UpsertOptions{MakeActive: true})
	if err != nil {
		t.Fatal(err)
	}
	acc.Alias = "person@example.com"
	if err := rt.codex.UpdateMetadata(acc); err != nil {
		t.Fatal(err)
	}
	// Match the account command path: publish the changed index before drawing.
	rt.svc.ReloadAccounts()
	u, tt := openSettingsView(t, e, tabAccounts)
	e.s.SetAccountPrivacyMode(true)
	tt.Frame()
	l := e.s.L()
	if tt.HasText(tx(l, xAccountsComingSoon)) {
		t.Fatal("coming soon placeholder is still shown")
	}
	// The toolbar keeps a privacy toggle; the display section stays in Display.
	if _, ok := tt.Find(l.Text(l10n.KeyAccountPrivacyMode)); !ok || tt.HasText(tx(l, xPrivacyDisplaySection)) {
		t.Fatal("accounts page lacks the toolbar privacy toggle or shows the display section")
	}
	if !tt.HasText("p***@***.com") || tt.HasText("person@example.com") {
		t.Fatalf("privacy mask missing; texts %q", tt.Texts())
	}
	u.tab = tabDisplay
	tt.Frame()
	if !tt.HasText(l.Text(l10n.KeyAccountPrivacyMode)) || !tt.HasText(tx(l, xPrivacyDisplaySection)) {
		t.Fatal("display privacy card or switch missing")
	}
}

func TestStartBackgroundStopsOnATempHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GROK_HOME", filepath.Join(home, "grok"))
	e := newEnv(t, nil)
	_ = e.s.Load()
	e.s.Update(func(p *store.Preferences) { p.ShowsRateLimitResetToday = false; p.ModelPricing.AutoSync = false })
	stop := StartBackground(context.Background(), BackgroundOptions{Home: home, Settings: e.s})
	if LiveService() == nil {
		t.Fatal("service was not installed")
	}
	stop()
	if LiveService() != nil {
		t.Fatal("service was not stopped")
	}
}
