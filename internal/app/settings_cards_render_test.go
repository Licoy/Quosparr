package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
)

// These previews never open a native window, read credentials, or save preferences.
func TestExportSettingsCardsReview(t *testing.T) {
	if os.Getenv("RUNWAY_SETTINGS_CARDS_QA") == "" {
		t.Skip("set RUNWAY_SETTINGS_CARDS_QA=1 to export offscreen PNGs")
	}
	dir := filepath.Join("..", "..", ".build", "v1-settings-layout-qa")
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese} {
		for _, width := range []int{settingsMinWidth, settingsWidth} {
			output := dir
			if language != store.LanguageEnglish || width != settingsWidth {
				output = filepath.Join(dir, fmt.Sprintf("%s-%d", language, width))
			}
			if err := os.MkdirAll(output, 0755); err != nil {
				t.Fatal(err)
			}
			for _, dark := range []bool{false, true} {
				appearance := "light"
				if dark {
					appearance = "dark"
				}
				u := cardSettingsUI(t, language)
				for _, tc := range networkCardCases {
					u.draft = ProxyDraft{Mode: tc.mode, UsesAuth: tc.auth, Host: "127.0.0.1", Port: "8080"}
					card := networkCardTester(u, width, dark)
					exportReviewPNG(t, card, filepath.Join(output, "general-network-"+tc.name+"-"+appearance+".png"))
					u.tab = tabGeneral
					full := ui.NewTester(u.view, width, settingsHeight)
					full.SetDark(dark)
					exportReviewPNG(t, full, filepath.Join(output, "general-"+tc.name+"-"+appearance+".png"))
				}
				u.draft = ProxyDraft{Mode: store.ProxyModeSystem}
				exportSettingsCardPages(t, u, output, width, dark, appearance)
			}
		}
	}
}

func exportSettingsCardPages(t *testing.T, u *settingsUI, output string, width int, dark bool, appearance string) {
	t.Helper()
	names := map[settingsTab]string{
		tabGeneral: "general", tabAccounts: "accounts", tabDisplay: "display", tabStatusBar: "statusbar", tabFloating: "floating",
		tabModules: "modules", tabNotifications: "notifications", tabSessions: "sessions", tabSkills: "skills",
		tabMCP: "mcp", tabAdvanced: "advanced", tabAbout: "about",
	}
	for _, tab := range settingsTabs {
		u.tab = tab
		view := ui.NewTester(u.view, width, settingsHeight)
		view.SetDark(dark)
		exportReviewPNG(t, view, filepath.Join(output, names[tab]+"-"+appearance+".png"))
	}
	// The tall, local page makes every display card header visible in one image.
	all := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Padding(16).Gap(14).Children(func() { u.displayPage(c) })
	}, width-settingsSidebarWidth, 1800)
	all.SetDark(dark)
	exportReviewPNG(t, all, filepath.Join(output, "display-all-cards-"+appearance+".png"))
}
