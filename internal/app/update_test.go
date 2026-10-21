package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/l10n"
)

func TestClassifyUnavailable(t *testing.T) {
	for _, c := range []struct {
		goos, exe string
		dev       bool
		want      UpdateBlock
	}{
		{"darwin", "/Applications/Quosparr.app/Contents/MacOS/Quosparr", false, UpdateBlockedLocation},
		{"linux", "/opt/quosparr/quosparr", false, UpdateBlockedPackage},
		{"linux", "/usr/bin/quosparr", false, UpdateBlockedPackage},
		{"linux", "/home/u/.local/quosparr.app/quosparr", false, UpdateBlockedLocation},
		{"windows", `C:\Program Files\Quosparr\Quosparr.exe`, false, UpdateBlockedLocation},
		{"linux", "/opt/quosparr/quosparr", true, UpdateBlockedDev},
	} {
		if got := classifyUnavailable(c.goos, c.exe, c.dev); got != c.want {
			t.Errorf("%s %s dev=%v: got %v, want %v", c.goos, c.exe, c.dev, got, c.want)
		}
	}
}

func TestUpdateLanguageCoversEveryLanguage(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range l10n.Languages() {
		tag := UpdateLanguage(l)
		if tag == "" || seen[tag] {
			t.Errorf("%s -> %q (empty or duplicate)", l, tag)
		}
		seen[tag] = true
	}
}

// ---- the per-cycle client ----

type countingTransport struct{ closed atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no network in tests")
}
func (c *countingTransport) CloseIdleConnections() { c.closed.Add(1) }

func newCountingClient() (*http.Client, *countingTransport) {
	tr := &countingTransport{}
	return &http.Client{Transport: tr}, tr
}

func TestCycleClientsShareOneClientPerCycle(t *testing.T) {
	var made []*countingTransport
	cc := newCycleClients(func(context.Context) (*http.Client, error) {
		c, tr := newCountingClient()
		made = append(made, tr)
		return c, nil
	})
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	a, _ := cc.Client(ctx1)
	b, _ := cc.Client(ctx1) // the install of the same cycle
	if a != b || len(made) != 1 {
		t.Fatalf("check and install must share a client: %p %p, %d made", a, b, len(made))
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	c, _ := cc.Client(ctx2)
	if c == a || len(made) != 2 {
		t.Fatal("a new cycle must get a new client")
	}
	if made[0].closed.Load() != 1 {
		t.Errorf("the old cycle's connections were not dropped: %d", made[0].closed.Load())
	}
}

func TestCycleClientsSettingsApplyFromTheNextCycle(t *testing.T) {
	route := "direct"
	cc := newCycleClients(func(context.Context) (*http.Client, error) {
		c, _ := newCountingClient()
		c.Timeout = map[string]time.Duration{"direct": time.Second, "proxy": 2 * time.Second}[route]
		return c, nil
	})
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	first, _ := cc.Client(ctx1)
	route = "proxy" // the user saves a proxy in the middle of the cycle
	same, _ := cc.Client(ctx1)
	if same != first {
		t.Error("the running cycle must keep its network context")
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	next, _ := cc.Client(ctx2)
	if next == first || next.Timeout == first.Timeout {
		t.Error("the next cycle must take the new settings")
	}
}

func TestCycleClientsClosesWhenTheCycleEnds(t *testing.T) {
	var tr *countingTransport
	cc := newCycleClients(func(context.Context) (*http.Client, error) {
		var c *http.Client
		c, tr = newCountingClient()
		return c, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = cc.Client(ctx)
	cancel()
	waitFor(t, func() bool { return tr.closed.Load() == 1 })
}

func TestCycleClientsNeverFallBackToDirect(t *testing.T) {
	boom := errors.New("proxy settings are broken")
	cc := newCycleClients(func(context.Context) (*http.Client, error) { return nil, boom })
	c, err := cc.Client(context.Background())
	if c != nil || !errors.Is(err, boom) {
		t.Errorf("got %v, %v", c, err)
	}
}

// ---- the service ----

type updateFake struct {
	svc       *UpdateService
	enabled   bool
	routeErr  error
	checks    int
	autoCalls []bool
	dialogs   []string
	buttons   [][]string
	answer    int
	opened    []string
}

func newUpdateFake() *updateFake {
	f := &updateFake{enabled: true}
	f.svc = &UpdateService{
		enabled:  func() bool { return f.enabled },
		checkUI:  func() { f.checks++ },
		setAuto:  func(on bool) { f.autoCalls = append(f.autoCalls, on) },
		route:    func() error { return f.routeErr },
		exe:      func() string { return "/opt/quosparr/quosparr" },
		dev:      func() bool { return false },
		goos:     "linux",
		openURL:  func(u string) error { f.opened = append(f.opened, u); return nil },
		runAsync: func(fn func()) { fn() },
		confirm: func(title, message string, buttons []string) int {
			f.dialogs = append(f.dialogs, message)
			f.buttons = append(f.buttons, buttons)
			return f.answer
		},
	}
	return f
}

func TestApplyAutomaticChecksIsTheOnlyTruth(t *testing.T) {
	for _, c := range []struct {
		name   string
		on, ok bool
		want   bool
	}{
		{"on", true, true, true},
		{"off", false, true, false},
		{"settings unusable", true, false, false},
	} {
		f := newUpdateFake()
		f.svc.ApplyAutomaticChecks(c.on, c.ok)
		if !slices.Equal(f.autoCalls, []bool{c.want}) {
			t.Errorf("%s: updater told %v, want %v", c.name, f.autoCalls, c.want)
		}
	}
}

func TestApplyAutomaticChecksAlwaysTellsTheUpdaterFirst(t *testing.T) {
	// Even "off" is written the first time: the updater's own file must not
	// keep an older value.
	f := newUpdateFake()
	f.svc.ApplyAutomaticChecks(false, true)
	f.svc.ApplyAutomaticChecks(false, true)
	f.svc.ApplyAutomaticChecks(true, true)
	if !slices.Equal(f.autoCalls, []bool{false, true}) {
		t.Errorf("calls = %v", f.autoCalls)
	}
}

func TestManualCheckIgnoresTheAutomaticPreference(t *testing.T) {
	f := newUpdateFake()
	f.svc.ApplyAutomaticChecks(false, true)
	f.svc.CheckNow(l10n.New(l10n.English))
	if f.checks != 1 || len(f.dialogs) != 0 {
		t.Errorf("checks %d, dialogs %v", f.checks, f.dialogs)
	}
}

func TestManualCheckThatCannotRunExplainsAndOffersTheDownloadPage(t *testing.T) {
	l := l10n.New(l10n.English)
	t.Run("package manager", func(t *testing.T) {
		f := newUpdateFake()
		f.enabled = false
		f.svc.CheckNow(l)
		if f.checks != 0 {
			t.Error("a check that cannot run must not show the update window (which would say up to date)")
		}
		if len(f.dialogs) != 1 || !strings.Contains(f.dialogs[0], "package manager") {
			t.Fatalf("dialogs = %v", f.dialogs)
		}
		if !slices.Equal(f.opened, []string{releasesURL}) {
			t.Errorf("opened %v", f.opened)
		}
	})
	t.Run("read-only location, user closes", func(t *testing.T) {
		f := newUpdateFake()
		f.enabled, f.svc.exe, f.answer = false, func() string { return "/Volumes/Quosparr/Quosparr.app/Contents/MacOS/x" }, 1
		f.svc.goos = "darwin"
		f.svc.CheckNow(l)
		if len(f.opened) != 0 || len(f.dialogs) != 1 || !strings.Contains(f.dialogs[0], "cannot write") {
			t.Errorf("opened %v dialogs %v", f.opened, f.dialogs)
		}
	})
	t.Run("development build has no download to offer", func(t *testing.T) {
		f := newUpdateFake()
		f.enabled = false
		f.svc.dev = func() bool { return true }
		f.svc.CheckNow(l)
		if len(f.buttons[0]) != 1 || len(f.opened) != 0 {
			t.Errorf("buttons %v opened %v", f.buttons, f.opened)
		}
	})
	t.Run("network settings the build cannot honour", func(t *testing.T) {
		f := newUpdateFake()
		f.routeErr = errors.New("proxy settings are broken")
		f.svc.CheckNow(l)
		if f.checks != 0 || len(f.dialogs) != 1 || !strings.Contains(f.dialogs[0], "proxy") {
			t.Errorf("checks %d dialogs %v", f.checks, f.dialogs)
		}
	})
}

func TestUpdateTextsAreTranslated(t *testing.T) {
	en := l10n.New(l10n.English)
	for _, lang := range l10n.Languages() {
		l := l10n.New(lang)
		for _, b := range []UpdateBlock{UpdateBlockedDev, UpdateBlockedPackage, UpdateBlockedLocation, UpdateBlockedNetwork} {
			if blockText(l, b) == "" || (lang != l10n.English && blockText(l, b) == blockText(en, b)) {
				t.Errorf("%s: block %d is not translated", lang, b)
			}
		}
	}
}

// ---- mygo's updater: where it keeps its state ----

func TestUseUpdaterKeepsItsStateInTheRunwayDirectory(t *testing.T) {
	home := t.TempDir()
	if err := DirectMygoPaths(mygo.App, home); err != nil {
		t.Fatal(err)
	}
	defer func(c *http.Client) { http.DefaultClient = c }(http.DefaultClient)
	svc, err := UseUpdater(UpdateOptions{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	// The settings are the truth: the updater's file gets exactly that.
	svc.ApplyAutomaticChecks(true, true)
	raw, err := os.ReadFile(filepath.Join(home, ".quosparr", "updater.json"))
	if err != nil {
		t.Fatalf("updater.json is not in ~/.quosparr: %v", err)
	}
	want := `"automaticChecks":true`
	if !strings.Contains(string(raw), want) {
		t.Errorf("updater.json = %s, want %s", raw, want)
	}
	if strings.Contains(string(raw), "automaticDownloads") {
		t.Errorf("nothing may be downloaded without the user's say: %s", raw)
	}
	// A second use is refused, not a crash.
	if _, err := UseUpdater(UpdateOptions{}); err == nil {
		t.Error("a second UseUpdater should fail")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}
