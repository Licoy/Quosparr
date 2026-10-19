package app

import (
	"os"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// System holds the calls into the running OS and mygo that the settings
// make. Several need App.Run (or must not touch the real machine during
// tests and dev runs), so tests and the dev-domain mode replace them. A nil
// func means "not supported here".
type System struct {
	// PreferredLanguages are the OS languages, for the automatic language.
	PreferredLanguages func() []string
	// SetAppearance makes the app follow light, dark or the system.
	SetAppearance func(store.Appearance)

	// OpenAtLogin reports the system's login item state. LoginSupported
	// false hides nothing but disables the switch.
	LoginSupported bool
	OpenAtLogin    func() (bool, error)
	SetOpenAtLogin func(on bool) error
	// LoginStatus is the finer state behind OpenAtLogin (a login item that
	// waits for the user's approval); nil when the system has none.
	LoginStatus func() (LoginStatus, error)
	// ApplyLogin preserves the verified status from applying a login change.
	ApplyLogin func(bool) (LoginStatus, error)
	// LoginAsync is only true for a backend that never dispatches to the UI
	// thread, so Flush can wait for it safely while quitting on that thread.
	LoginAsync bool

	// StatusBarStyles is true where the status item can show the styles of
	// the status bar settings (the macOS menu bar). PanelBackground is
	// true where the translucent/opaque choice applies. PanelAnimation is
	// true where the native panel can animate. Widgets is true where
	// home-screen widgets exist (macOS 14+).
	StatusBarStyles bool
	PanelBackground bool
	PanelAnimation  bool
	Widgets         bool

	// FontFamilies lists the installed font families the display font can
	// be; nil offers only the system's.
	FontFamilies func() []string

	// SetAutomaticUpdateChecks applies the "check automatically" choice to
	// the updater; nil until the updater is wired.
	SetAutomaticUpdateChecks func(on bool)
}

// DefaultSystem is the System of this OS.
func DefaultSystem() System {
	mac := runtime.GOOS == "darwin"
	exe, _ := os.Executable()
	item := ProductionLoginItem(exe)
	async := loginBackendAsync(item.Backend)
	return LoginSystem(item, System{
		LoginAsync:         async,
		PreferredLanguages: l10n.SystemPreferredLanguages,
		SetAppearance:      setMygoAppearance,
		FontFamilies:       installedFontFamilies,
		StatusBarStyles:    mac,
		PanelBackground:    mac,
		PanelAnimation:     mac,
		Widgets:            mac && macOSAtLeast(14),
	})
}

// DevSystem is DefaultSystem without the calls that change the machine
// (login item, updater): for manual runs against a scratch preferences
// domain.
func DevSystem() System {
	s := DefaultSystem()
	s.LoginSupported = false
	s.OpenAtLogin, s.SetOpenAtLogin, s.LoginStatus, s.ApplyLogin = nil, nil, nil, nil
	s.LoginAsync = false
	return s
}

func setMygoAppearance(a store.Appearance) {
	switch a {
	case store.AppearanceDark:
		mygo.Theme.SetSource(mygo.ThemeDark)
	case store.AppearanceLight:
		mygo.Theme.SetSource(mygo.ThemeLight)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}
