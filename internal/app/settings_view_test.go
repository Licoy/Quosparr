package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// openSettingsView returns a window-less settings view on tab, in English.
func openSettingsView(t *testing.T, e *env, tab settingsTab) (*settingsUI, *ui.Tester) {
	t.Helper()
	if err := e.s.Load(); err != nil {
		t.Log("load:", err)
	}
	_ = e.s.LoadProxy()
	e.s.Apply()
	u := newSettingsUI(e.s, SettingsOptions{Version: "9.9.9"})
	u.tab = tab
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	e.s.OnChange(tt.Frame)
	return u, tt
}

func TestSettingsViewPillsChangePreferences(t *testing.T) {
	e := newEnv(t, nil)
	_, tt := openSettingsView(t, e, tabDisplay)
	l := e.s.L()
	if err := tt.Click(l.Text(appearanceKey(store.AppearanceDark))); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(l.Text(panelBackgroundKey(store.PanelBackgroundOpaque))); err != nil {
		t.Fatal(err)
	}
	e.settle()
	got := e.stored()
	if got.Appearance != store.AppearanceDark || got.MainPanelBackgroundStyle != store.PanelBackgroundOpaque {
		t.Errorf("stored appearance %v, background %v", got.Appearance, got.MainPanelBackgroundStyle)
	}
	if last := e.sys.appearance[len(e.sys.appearance)-1]; last != store.AppearanceDark {
		t.Errorf("appearance not applied live: %v", e.sys.appearance)
	}
}

func TestSettingsViewLanguageChoiceRelabelsThePage(t *testing.T) {
	e := newEnv(t, nil)
	_, tt := openSettingsView(t, e, tabGeneral)
	en := e.s.L()
	if !tt.HasText(en.Text(l10n.KeyLanguage)) {
		t.Fatalf("no language row; texts %q", tt.Texts())
	}
	// Open the dropdown, which shows the current choice, and pick French.
	if err := tt.Click(l10n.MenuTitle("system", en.Language())); err != nil {
		t.Fatalf("open dropdown: %v; texts %q", err, tt.Texts())
	}
	if err := tt.Click(l10n.MenuTitle("french", en.Language())); err != nil {
		t.Fatalf("choose: %v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	e.settle()
	if got := e.stored().Language; got != store.LanguageFrench {
		t.Fatalf("stored language = %v", got)
	}
	fr := e.s.L()
	if fr.Language() != l10n.French || !tt.HasText(fr.Text(l10n.KeyLanguage)) {
		t.Errorf("the page did not switch to French; texts %q", tt.Texts())
	}
}

func TestSettingsViewReadOnlyExplainsAndRefuses(t *testing.T) {
	e := newEnv(t, func(d *failDomain) { d.Put(store.KeyPreferences, []byte("{broken")) })
	_, tt := openSettingsView(t, e, tabDisplay)
	l := e.s.L()
	if !tt.HasText(tx(l, xSettingsUnreadable)) {
		t.Errorf("the problem is not shown; texts %q", tt.Texts())
	}
	// Whatever is clicked, nothing is saved.
	_ = tt.Click(l.Text(appearanceKey(store.AppearanceDark)))
	e.settle()
	if e.mem.Writes() != 0 {
		t.Error("a damaged config was written over")
	}
	if e.s.Prefs().Appearance != store.AppearanceSystem {
		t.Error("the read-only settings changed")
	}
}

func TestSettingsViewTabs(t *testing.T) {
	e := newEnv(t, nil)
	u, tt := openSettingsView(t, e, tabGeneral)
	l := e.s.L()
	for _, tab := range settingsTabs {
		if err := tt.Click(tab.title(l)); err != nil {
			t.Fatalf("tab %v: %v", tab, err)
		}
		tt.Frame()
		if u.tab != tab {
			t.Errorf("clicked %v, on %v", tab, u.tab)
		}
	}
	if !tt.HasText("9.9.9") {
		t.Errorf("the About tab does not show the version; texts %q", tt.Texts())
	}
}

func TestSettingsViewExplainsUnsupportedPlatformSettings(t *testing.T) {
	e := newEnv(t, nil)
	e.s = NewSettings(e.st, System{}, e.creds)
	_, tt := openSettingsView(t, e, tabDisplay)
	if !tt.HasText(tx(e.s.L(), xMacOnly)) {
		t.Errorf("no note on the status bar styles; texts %q", tt.Texts())
	}
}

func TestSettingsViewSavesDisablePanelAnimation(t *testing.T) {
	e := newEnv(t, nil)
	_, tt := openSettingsView(t, e, tabAdvanced)
	label := tx(e.s.L(), xDisablePanelAnimation)
	row, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing %q", label)
	}
	click := func() { tt.ClickAt(settingsWidth-48, row.Y+row.H/2) }
	if e.s.Prefs().DisableMainPanelAnimation {
		t.Fatal("disabling panel animation should default off")
	}
	click()
	e.settle()
	if !e.stored().DisableMainPanelAnimation {
		t.Fatal("disable-panel-animation choice was not saved")
	}
	reloaded := NewSettings(e.st, e.sys.system(), e.creds)
	if err := reloaded.Load(); err != nil || !reloaded.Prefs().DisableMainPanelAnimation {
		t.Fatalf("reload: disabled=%v err=%v", reloaded.Prefs().DisableMainPanelAnimation, err)
	}
	click()
	e.settle()
	if e.stored().DisableMainPanelAnimation {
		t.Fatal("restored panel animation was not saved")
	}
}

func TestSettingsViewDisablesPanelAnimationSettingWhenUnavailableOrReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(*failDomain)
		sys  System
	}{
		{"unsupported", nil, System{}},
		{"read only", func(d *failDomain) { d.Put(store.KeyPreferences, []byte("{")) }, System{PanelAnimation: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, tc.seed)
			e.s = NewSettings(e.st, tc.sys, e.creds)
			_, tt := openSettingsView(t, e, tabAdvanced)
			label := tx(e.s.L(), xDisablePanelAnimation)
			if tc.name == "unsupported" && !tt.HasText(tx(e.s.L(), xMacOnly)) {
				t.Fatalf("missing macOS-only explanation: %q", tt.Texts())
			}
			row, ok := tt.Find(label)
			if !ok {
				t.Fatalf("missing %q", label)
			}
			tt.ClickAt(settingsWidth-48, row.Y+row.H/2)
			e.settle()
			if e.s.Prefs().DisableMainPanelAnimation || e.mem.Writes() != 0 {
				t.Fatalf("disabled control changed preferences: disabled=%v writes=%d", e.s.Prefs().DisableMainPanelAnimation, e.mem.Writes())
			}
		})
	}
}

func TestSettingsProxySaveButtonFollowsUnsavedChanges(t *testing.T) {
	e := newEnv(t, nil)
	u, tt := openSettingsView(t, e, tabGeneral)
	tt.Frame()
	l := e.s.L()
	if u.proxyUnsaved() {
		t.Fatal("an untouched draft counts as unsaved")
	}
	u.draft = ProxyDraft{Mode: store.ProxyModeSystem, Host: "leftover", Port: "1"}
	if u.proxyUnsaved() {
		t.Fatal("fields the system mode ignores count as unsaved")
	}
	u.draft = ProxyDraft{Mode: store.ProxyModeHTTP, Host: "127.0.0.1", Port: "8080"}
	if !u.proxyUnsaved() {
		t.Fatal("a new proxy mode is not unsaved")
	}
	u.setProxyMsg("stale", true)
	tt.Frame()
	if err := tt.Click(l.Text(l10n.KeyNetworkProxySave)); err != nil {
		t.Fatal(err)
	}
	awaitSettingsTask(t, e, &u.proxyTask)
	tt.Frame()
	if u.proxyMsg != "" || u.toast != "" || !tt.HasText(l.Text(l10n.KeyNetworkProxySaved)) {
		t.Fatalf("save success was not a toast: message %q", u.proxyMsg)
	}
	if u.proxyUnsaved() || e.s.Proxy().Mode != store.ProxyModeHTTP {
		t.Fatal("the saved draft still counts as unsaved")
	}
}

func TestSettingsProxyTestMessageClears(t *testing.T) {
	old := proxyMsgTime
	proxyMsgTime = 10 * time.Millisecond
	t.Cleanup(func() { proxyMsgTime = old })
	e := newEnv(t, nil)
	u, tt := openSettingsView(t, e, tabGeneral)
	tt.Frame()
	u.flashProxyMsg("ok")
	deadline := time.Now().Add(time.Second)
	for u.proxyMsg != "" && time.Now().Before(deadline) {
		e.q.drain()
		time.Sleep(time.Millisecond)
	}
	if u.proxyMsg != "" {
		t.Fatal("the test result did not clear itself")
	}
	u.setProxyMsg("failed", true)
	tt.Frame()
	if err := tt.Click(e.s.L().Text(l10n.KeyNetworkProxyHTTP)); err != nil {
		t.Fatal(err)
	}
	if u.proxyMsg != "" {
		t.Fatal("switching the proxy mode kept the test result")
	}
	u.setProxyMsg("failed", true)
	tt.Frame()
	if err := tt.Click(tabAbout.title(e.s.L())); err != nil {
		t.Fatal(err)
	}
	if u.proxyMsg != "" {
		t.Fatal("switching the settings page kept the test result")
	}
}
