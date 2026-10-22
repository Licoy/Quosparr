package app

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

func TestFloatingAppMatcherFollowsTheList(t *testing.T) {
	codex := foreground{kind: fgApp, keys: []string{"com.openai.codex"}}
	other := foreground{kind: fgApp, keys: []string{"com.apple.safari"}}
	desktop := foreground{kind: fgDesktop}
	passing := foreground{}
	keys := map[string]bool{"com.openai.codex": true}
	cases := []struct {
		filter      store.FloatingAppFilter
		fg          foreground
		visible, ok bool
	}{
		{store.FloatingAppFilterOff, other, true, true},
		{store.FloatingAppFilterOff, passing, true, true},
		{store.FloatingAppFilterAllow, codex, true, true},
		{store.FloatingAppFilterAllow, other, false, true},
		{store.FloatingAppFilterAllow, desktop, true, true},
		{store.FloatingAppFilterAllow, passing, false, false},
		{store.FloatingAppFilterBlock, codex, false, true},
		{store.FloatingAppFilterBlock, other, true, true},
		{store.FloatingAppFilterBlock, desktop, true, true},
		{store.FloatingAppFilterBlock, passing, false, false},
	}
	for _, tc := range cases {
		m := floatingAppMatcher{filter: tc.filter, keys: keys}
		if v, ok := m.visible(tc.fg); v != tc.visible || ok != tc.ok {
			t.Errorf("%q with %+v: (%v, %v), want (%v, %v)", tc.filter, tc.fg, v, ok, tc.visible, tc.ok)
		}
	}
}

func TestFloatingFollowShowsForAnAppAtOnceAndHidesOnceSteady(t *testing.T) {
	var d floatingFollow
	if !d.step(false, false) || d.visible {
		t.Fatal("the first look does not set the state")
	}
	if !d.step(true, true) || !d.visible {
		t.Fatal("an app of the list does not show the window at once")
	}
	if d.step(false, false) || !d.visible {
		t.Fatal("one look hid the window")
	}
	if d.step(true, false) || !d.visible {
		t.Fatal("going back changed the state")
	}
	d.step(false, false)
	if !d.step(false, false) || d.visible {
		t.Fatal("two looks did not hide the window")
	}
	// The desktop shows it only once steady, past the moment of no window
	// while switching apps.
	if d.step(true, false) || d.visible {
		t.Fatal("one look of the desktop showed the window")
	}
	if !d.step(true, false) || !d.visible {
		t.Fatal("the desktop did not show the window")
	}
}

func TestDesktopEntryKeys(t *testing.T) {
	content := "[Desktop Entry]\nName=Visual Studio Code\nExec=env FOO=1 /usr/share/code/code --unity-launch %F\nStartupWMClass=Code\n\n[Desktop Action new-window]\nExec=other\n"
	if got, want := desktopEntryKeys("code.desktop", content), []string{"code", "code", "code"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %q, want %q", got, want)
	}
	flatpak := "[Desktop Entry]\nExec=/usr/bin/flatpak run --branch=stable org.gnome.Maps\n"
	if got, want := desktopEntryKeys("org.gnome.Maps.desktop", flatpak), []string{"org.gnome.maps"}; !reflect.DeepEqual(got, want) {
		t.Errorf("flatpak keys = %q, want %q", got, want)
	}
	if got := desktopEntryValue(content, "Name"); got != "Visual Studio Code" {
		t.Errorf("name = %q", got)
	}
}

func TestFloatingAppsDialogKeepsItsOrder(t *testing.T) {
	codex := store.FloatingApp{ID: "codex", Name: "Codex"}
	safari := store.FloatingApp{ID: "safari", Name: "Safari"}
	notes := store.FloatingApp{ID: "notes", Name: "Notes"}
	d := &floatingAppsDialog{picked: []store.FloatingApp{codex}, listed: []store.FloatingApp{codex}}
	d.list(false, safari, codex, notes)
	d.toggle(notes, true)
	d.toggle(codex, false)
	d.toggle(codex, false)
	if got := d.rows(); !slices.Equal(got, []store.FloatingApp{codex, safari, notes}) {
		t.Errorf("rows = %v", got)
	}
	if !slices.Equal(d.picked, []store.FloatingApp{notes}) {
		t.Errorf("picked = %v", d.picked)
	}
	d.query = "SAF"
	if got := d.rows(); !slices.Equal(got, []store.FloatingApp{safari}) {
		t.Errorf("search rows = %v", got)
	}
}

func TestFloatingAppsSummary(t *testing.T) {
	l := l10n.New(l10n.SimplifiedChinese)
	apps := []store.FloatingApp{{ID: "a", Name: "Codex"}, {ID: "com.example.b"}, {ID: "c"}}
	for n, want := range map[int]string{0: "尚未选择应用", 2: "Codex, com.example.b", 3: "已选 3 个应用"} {
		if got := floatingAppsSummary(l, apps[:n]); got != want {
			t.Errorf("%d apps: %q, want %q", n, got, want)
		}
	}
}

func TestFloatingFollowKeyChangesWithTheList(t *testing.T) {
	f := store.FloatingWindow{Apps: []store.FloatingApp{{ID: "a"}}}
	if floatingFollowKey(f) != "" {
		t.Error("the window follows apps with the filter off")
	}
	if !foregroundSupported() {
		t.Skip("no app in front to follow here")
	}
	f.AppFilter = store.FloatingAppFilterAllow
	allow := floatingFollowKey(f)
	f.HideOffList = true
	allowHiding := floatingFollowKey(f)
	f.AppFilter = store.FloatingAppFilterBlock
	block := floatingFollowKey(f)
	f.HideOffList = false
	if block != floatingFollowKey(f) {
		t.Error("hiding off the list changes the block list's key")
	}
	f.Apps = append(f.Apps, store.FloatingApp{ID: "b"})
	if allow == "" || allow == block || allow == allowHiding || allowHiding == block || block == floatingFollowKey(f) {
		t.Errorf("keys do not tell the settings apart: %q %q %q", allow, allowHiding, block)
	}
}

func TestFloatingFollowLowersOnlyForTheAllowList(t *testing.T) {
	cases := []struct {
		filter store.FloatingAppFilter
		hide   bool
		lowers bool
	}{
		{store.FloatingAppFilterAllow, false, true},
		{store.FloatingAppFilterAllow, true, false},
		{store.FloatingAppFilterBlock, false, false},
		{store.FloatingAppFilterBlock, true, false},
		{store.FloatingAppFilterOff, false, false},
	}
	for _, tc := range cases {
		if got := floatingFollowLowers(store.FloatingWindow{AppFilter: tc.filter, HideOffList: tc.hide}); got != tc.lowers {
			t.Errorf("%q hiding %v: lowers %v, want %v", tc.filter, tc.hide, got, tc.lowers)
		}
	}
}

func TestFloatingOffListChoosesLowerOrHide(t *testing.T) {
	if !foregroundSupported() {
		t.Skip("no app in front to follow here")
	}
	u := cardSettingsUI(t, store.LanguageEnglish)
	l := u.L()
	u.tab = tabFloating
	u.s.SetFloatingWindow(func(f *store.FloatingWindow) { f.AppFilter = store.FloatingAppFilterBlock })
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight*3)
	if tt.HasText(tx(l, xFloatingOffList)) {
		t.Fatal("the block list offers what to do off the list")
	}
	u.s.SetFloatingWindow(func(f *store.FloatingWindow) { f.AppFilter = store.FloatingAppFilterAllow })
	tt.Frame()
	for _, want := range []string{tx(l, xFloatingOffList), tx(l, xFloatingOffListLower), tx(l, xFloatingOffListLowerHint)} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing", want)
		}
	}
	if err := tt.Click(tx(l, xFloatingOffListLower)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(l, xFloatingOffListHide)); err != nil {
		t.Fatal(err)
	}
	if !u.s.Prefs().FloatingWindow.HideOffList {
		t.Fatal("choosing hide did not save it")
	}
	if !tt.HasText(tx(l, xFloatingOffListHideHint)) {
		t.Error("the hint does not follow the choice")
	}
}

func TestFloatingAppFilterChoosesAppsInADialog(t *testing.T) {
	if !foregroundSupported() {
		t.Skip("no app in front to follow here")
	}
	u := cardSettingsUI(t, store.LanguageEnglish)
	l := u.L()
	u.tab = tabFloating
	codex := store.FloatingApp{ID: "com.openai.codex", Name: "Codex"}
	safari := store.FloatingApp{ID: "com.apple.Safari", Name: "Safari"}
	u.s.SetFloatingWindow(func(f *store.FloatingWindow) {
		f.AppFilter = store.FloatingAppFilterAllow
		f.Apps = []store.FloatingApp{codex}
	})
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight*3)
	for _, want := range []string{tx(l, xFloatingAppFilterAllowHint), "Codex", tx(l, xFloatingChooseApps)} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing", want)
		}
	}
	u.floatingApps = &floatingAppsDialog{picked: []store.FloatingApp{codex}, listed: []store.FloatingApp{codex, safari}, done: make(chan struct{})}
	tt.Frame()
	if !tt.HasText(tx(l, xFloatingAppsTitle)) || !tt.HasText("Safari") {
		t.Fatal("the dialog does not list the apps")
	}
	if dir := os.Getenv("RUNWAY_FLOATING_QA"); dir != "" {
		exportReviewPNG(t, tt, filepath.Join(dir, "floating-apps.png"))
	}
	if err := tt.Click("Safari"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(l.Text(l10n.KeyOk)); err != nil {
		t.Fatal(err)
	}
	if got := u.s.Prefs().FloatingWindow.Apps; !slices.Equal(got, []store.FloatingApp{codex, safari}) {
		t.Errorf("apps = %v", got)
	}
	if u.floatingApps != nil {
		t.Error("the dialog stays open")
	}
}
