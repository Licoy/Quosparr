package app

import (
	"image/color"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestSettingsAccountRowActionsAreIconButtons(t *testing.T) {
	s := NewSettings(store.New(store.NewMemoryDomain(), nil), System{}, nil)
	s.prefs = store.DefaultPreferences()
	s.prefs.Language = store.LanguageEnglish
	s.applyLocale()
	u := newSettingsUI(s, SettingsOptions{})
	l := u.L()
	f := accountFormOf(u)
	rt := &liveApp{svc: runway.New(runway.Options{Preferences: s.Prefs()})}
	t.Cleanup(rt.svc.Close)
	account := codex.ManagedAccount{ID: "fixture", DisplayName: "Fixture", Note: "Old note"}
	active := ""
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Padding(16).Children(func() { renderCodexSettingsRow(c, u, f, rt, account, rowPlace{}, active, false) })
	}, 620, 320)
	if tt.HasText(l.Text(l10n.KeyAccountsActions)) {
		t.Fatal("actions menu still shown")
	}
	for _, label := range []string{l.Text(l10n.KeyAccountsMakeCurrent), l.Text(l10n.KeyAlias), l.Text(l10n.KeyAccountsNote), l.Text(l10n.KeyAccountsMoveUp), l.Text(l10n.KeyAccountsMoveDown), l.Text(l10n.KeyAccountsDelete)} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("missing %q: %q", label, tt.Texts())
		}
	}
	note, _ := tt.Find(l.Text(l10n.KeyAccountsNote))
	tt.Move(note.X+note.W/2, note.Y+note.H/2)
	if tt.Cursor() != ui.CursorPointer {
		t.Fatalf("action button cursor: %v", tt.Cursor())
	}
	if err := tt.Click(l.Text(l10n.KeyAccountsNote)); err != nil {
		t.Fatal(err)
	}
	if f.mode != "note" || f.editID != "fixture" || f.note != "Old note" {
		t.Fatalf("note editor did not open: %#v", f)
	}
	if err := tt.Click(l.Text(l10n.KeyAccountsMakeCurrent)); err != nil {
		t.Fatal(err)
	}
	if f.mode != "switch" || f.editID != "fixture" {
		t.Fatalf("switch confirmation did not open: %#v", f)
	}
	f.mode, active = "", "fixture"
	tt.Frame()
	if _, ok := tt.Find(l.Text(l10n.KeyAccountsMakeCurrent)); ok {
		t.Fatal("current account offers make current")
	}
	tick, _ := tt.Find(l.Text(l10n.KeyAccountsIsCurrentLogin))
	tt.Move(tick.X+tick.W/2, tick.Y+tick.H/2)
	if tt.Cursor() != ui.CursorDefault {
		t.Fatalf("disabled button cursor: %v", tt.Cursor())
	}
	if err := tt.Click(l.Text(l10n.KeyAccountsIsCurrentLogin)); err != nil {
		t.Fatal(err)
	}
	if f.mode != "" {
		t.Fatalf("disabled tick acted: %q", f.mode)
	}
}

func TestMoveActionsStopAtTheEnds(t *testing.T) {
	l := l10n.New(l10n.English)
	for _, c := range []struct {
		i, n     int
		up, down bool
	}{{0, 1, false, false}, {0, 3, false, true}, {1, 3, true, true}, {2, 3, true, false}} {
		a := moveActions(l, placeAt(c.i, c.n), func(int) {})
		if !a[0].disabled != c.up || !a[1].disabled != c.down {
			t.Fatalf("row %d of %d: up=%v down=%v", c.i, c.n, !a[0].disabled, !a[1].disabled)
		}
	}
}

// codexOrderFixture is a settings page over three stored accounts, the
// middle one current.
func codexOrderFixture(t *testing.T) (*env, *accountForm, *ui.Tester, *codex.Store) {
	t.Helper()
	st := codex.NewStore(t.TempDir(), t.TempDir())
	idx := codex.AccountIndex{ActiveAccountID: "b"}
	for i, name := range []string{"Alpha", "Bravo", "Charlie"} {
		id := string(rune('a' + i))
		idx.Accounts = append(idx.Accounts, codex.ManagedAccount{ID: id, DisplayName: name, SortIndex: i, AuthMode: codex.AuthModeOAuth, CreatedAt: time.Unix(0, 0)})
	}
	if err := st.SaveIndex(idx); err != nil {
		t.Fatal(err)
	}
	svc := runway.New(runway.Options{Sources: runway.Sources{LoadIndex: st.LoadIndex}})
	t.Cleanup(svc.Close)
	svc.ReloadAccounts()
	if n := len(svc.Snapshot().Accounts.Sidebar); n != 3 {
		t.Fatalf("fixture accounts not loaded: %d %q", n, svc.Snapshot().Codex.LastError)
	}
	rt := &liveApp{codex: st, svc: svc}
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	e.s.SetAccountPrivacyMode(false)
	e.settle()
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Padding(16).Gap(8).Children(func() { renderCodexAccounts(c, u, f, rt) })
	}, 620, 900)
	return e, f, tt, st
}

func shownOrder(t *testing.T, tt *ui.Tester, names ...string) []string {
	t.Helper()
	out := slices.Clone(names)
	slices.SortFunc(out, func(a, b string) int {
		ra, oka := tt.Find(a)
		rb, okb := tt.Find(b)
		if !oka || !okb {
			t.Fatalf("missing %q or %q: %q", a, b, tt.Texts())
		}
		return int(ra.Y - rb.Y)
	})
	return out
}

func TestCodexSettingsRowsKeepStoredOrderSoMovesShow(t *testing.T) {
	e, f, tt, st := codexOrderFixture(t)
	l := e.s.L()
	if got := shownOrder(t, tt, "Charlie", "Bravo", "Alpha"); !slices.Equal(got, []string{"Alpha", "Bravo", "Charlie"}) {
		t.Fatalf("settings pinned the current account: %v", got)
	}
	// The first match is the first row's: its move up does nothing.
	if err := tt.Click(l.Text(l10n.KeyAccountsMoveUp)); err != nil || f.busy {
		t.Fatalf("first row moved up: %v", err)
	}
	if err := tt.Click(l.Text(l10n.KeyAccountsMoveDown)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for f.busy && time.Now().Before(deadline) {
		e.q.drain()
		time.Sleep(5 * time.Millisecond)
	}
	tt.Frame()
	if got := shownOrder(t, tt, "Alpha", "Bravo", "Charlie"); !slices.Equal(got, []string{"Bravo", "Alpha", "Charlie"}) {
		t.Fatalf("move down did not show: %v", got)
	}
	idx, err := st.LoadIndex()
	if err != nil || idx.ActiveAccountID != "b" {
		t.Fatalf("move changed the current account: %v %q", err, idx.ActiveAccountID)
	}
}

func TestAccountToolbarPrivacyFollowsRefresh(t *testing.T) {
	e, _, tt, _ := codexOrderFixture(t)
	l := e.s.L()
	refresh, _ := tt.Find(l.Text(l10n.KeyAccountsRefreshAll))
	privacy, ok := tt.Find(l.Text(l10n.KeyAccountPrivacyMode))
	export, _ := tt.Find(l.Text(l10n.KeyAccountsExport))
	if !ok || privacy.X <= refresh.X || privacy.X >= export.X {
		t.Fatalf("privacy button not after refresh: %v %v %v", refresh, privacy, export)
	}
	if err := tt.Click(l.Text(l10n.KeyAccountPrivacyMode)); err != nil {
		t.Fatal(err)
	}
	e.settle()
	tt.Frame()
	if !e.s.Prefs().AccountPrivacyModeEnabled || tt.HasText("Alpha") {
		t.Fatalf("privacy mode did not turn on: %q", tt.Texts())
	}
}

func TestSettingsAccountRowHoverFades(t *testing.T) {
	_, _, tt, _ := codexOrderFixture(t)
	name, _ := tt.Find("Alpha")
	x, y := 22, int(name.Y+name.H/2)
	wash := func() color.RGBA { return tt.Image().RGBAAt(x, y) }
	rest := wash()
	tt.Move(float32(x), float32(y))
	var seen []color.RGBA
	for range 20 {
		tt.Advance(20 * time.Millisecond)
		seen = append(seen, wash())
	}
	full := seen[len(seen)-1]
	between := slices.ContainsFunc(seen, func(c color.RGBA) bool { return c != rest && c != full })
	if full == rest || !between {
		t.Fatalf("hover did not fade in: %v %v", rest, seen)
	}
	tt.Move(600, 890)
	tt.Advance(time.Second)
	if wash() != rest {
		t.Fatalf("hover did not fade out: %v", wash())
	}
}
