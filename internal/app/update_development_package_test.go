//go:build quosparr_dev_package

package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/l10n"
)

func TestPackagedDevelopmentUpdateExplainsMissingUpdates(t *testing.T) {
	// mygo build uses production mode even for our development CI packages.
	t.Setenv("MYGO_ENV", "production")
	if mygo.IsDev() || mygo.Updater.Enabled() {
		t.Fatal("fixture must be production mode without a linked update feed")
	}
	for _, goos := range []string{"darwin", "windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			s := newUpdateService()
			s.goos = goos
			s.exe = func() string { return "/opt/quosparr/quosparr" }
			s.runAsync = func(fn func()) { fn() }
			var message string
			s.confirm = func(_ string, text string, buttons []string) int {
				message = text
				if len(buttons) != 1 {
					t.Error("development package must not offer the production download feed")
				}
				return 0
			}
			s.openURL = func(string) error { t.Error("unexpected external URL"); return nil }
			s.checkUI = func() { t.Error("disabled CI updater tried to check") }
			if got := s.Block(); got != UpdateBlockedDev {
				t.Fatalf("packaged development build misclassified: got %v, want %v", got, UpdateBlockedDev)
			}
			s.CheckNow(l10n.New(l10n.English))
			if message == "" || strings.Contains(message, "cannot write") || strings.Contains(message, "package manager") {
				t.Fatalf("incorrect update explanation: %q", message)
			}
		})
	}
}
