package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/store"
)

// These previews use only in-memory preferences and synthetic public account metadata.
func TestExportSettingsSidebarReview(t *testing.T) {
	if os.Getenv("RUNWAY_SIDEBAR_QA") == "" {
		t.Skip("set RUNWAY_SIDEBAR_QA=1 to export offscreen PNGs")
	}
	dir := filepath.Join("..", "..", ".build", "v1-sidebar-qa")
	names := map[settingsTab]string{
		tabGeneral: "general", tabAccounts: "accounts", tabDisplay: "display", tabFloating: "floating",
		tabModules: "modules", tabNotifications: "notifications", tabSessions: "sessions", tabSkills: "skills",
		tabMCP: "mcp", tabModelPricing: "model-pricing", tabAdvanced: "advanced", tabAbout: "about",
	}
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageRussian} {
		output := dir
		if language != store.LanguageEnglish {
			output = filepath.Join(dir, string(language))
		}
		if err := os.MkdirAll(output, 0755); err != nil {
			t.Fatal(err)
		}
		for _, dark := range []bool{false, true} {
			appearance := "light"
			if dark {
				appearance = "dark"
			}
			snap := uiReviewSnapshot()
			a, panel := newPanel(t, func(p *store.Preferences) { p.Language = language }, snap, PanelActions{})
			a.settings.sys = (&fakeSys{}).system()
			if err := a.settings.LoadProxy(); err != nil {
				t.Fatal(err)
			}
			u := newSettingsUI(a.settings, SettingsOptions{Version: "1.0.0"})
			for _, tab := range settingsTabs {
				u.tab = tab
				view := ui.NewTester(func(c *ui.Context) {
					if u.tab != tabAccounts {
						u.view(c)
						return
					}
					applyThemeColor(c, u.s.prefs.ThemeColor, u.s.prefs.FontFamily)
					u.shell(c, func(c *ui.Context) {
						ui.Scroll(c).Grow(1).Padding(4, 24, 24, 24).Gap(10).Children(func() { sidebarAccountFixture(c, u, snap.Accounts.Sidebar) })
					})
				}, settingsWidth, settingsHeight)
				view.SetTitleBar(ui.TitleBar{Height: 40, Left: 80})
				view.SetDark(dark)
				exportReviewPNG(t, view, filepath.Join(output, names[tab]+"-"+appearance+".png"))
			}
			width, height := a.panelSizeWithin(1080, 1040)
			panel.SetSize(width, height)
			panel.SetDark(dark)
			exportReviewPNG(t, panel, filepath.Join(output, "panel-codex-"+appearance+".png"))
			a.settings.prefs.SelectedProvider = store.ProviderGrok
			a.snap.Grok.Accounts = snap.Accounts
			a.snap.Grok.Accounts.Display.DisplayName = "Grok workspace"
			panel.Frame()
			exportReviewPNG(t, panel, filepath.Join(output, "panel-grok-"+appearance+".png"))
		}
	}
}

func sidebarAccountFixture(c *ui.Context, u *settingsUI, rows []codex.ManagedAccount) {
	l := u.L()
	f := accountFormOf(u)
	accountProviderHeader(c, u, f)
	accountToolbar(c, u, f, accountCountTitle(l, len(rows)), accountFolder{"Codex", "/codex"}, func(*ui.Menu) {}, func() string { return "" }, func() string { return "" })
	accountRows(c, len(rows), func(i int) {
		renderCodexSettingsRow(c, u, f, nil, rows[i], "ada", u.s.Prefs().AccountPrivacyModeEnabled)
	})
}
