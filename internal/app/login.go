package app

import (
	"errors"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// LoginStatus is the state of the system login item, like the Swift
// LoginItemStatus.
type LoginStatus int

const (
	// LoginUnavailable: development and command-line runs never manage the
	// system's login items.
	LoginUnavailable LoginStatus = iota
	// LoginNotRegistered: no login item (never made, or removed by the
	// user in the system settings).
	LoginNotRegistered
	// LoginEnabled: the app starts at login.
	LoginEnabled
	// LoginRequiresApproval: registered, but the user still has to allow it
	// in System Settings (macOS 13+).
	LoginRequiresApproval
)

// Registered is true for a login item the app asked for: enabled, or
// waiting for approval. The stored choice follows this, as in Swift.
func (s LoginStatus) Registered() bool { return s == LoginEnabled || s == LoginRequiresApproval }

// Errors of changing the login item (Swift LoginItemError).
var (
	ErrLoginRegister   = errors.New("could not register the login item")
	ErrLoginUnregister = errors.New("could not remove the login item")
)

// LoginBackend reads and changes the system login item.
type LoginBackend interface {
	Status() (LoginStatus, error)
	Set(on bool) error
}

// LoginItem is the Swift LoginItemApplier: it changes the login item only
// where that is allowed, and checks the result.
type LoginItem struct {
	Backend      LoginBackend
	CanMutateSys bool
}

// Status is the login item's state; LoginUnavailable where the system must
// not be touched.
func (l LoginItem) Status() (LoginStatus, error) {
	if !l.CanMutateSys {
		return LoginUnavailable, nil
	}
	return l.Backend.Status()
}

// Apply registers or removes the login item and returns the state it
// reached. The backend's own error does not decide: the item either ended up
// as asked (a registration still waiting for approval counts) or it did not.
func (l LoginItem) Apply(on bool) (LoginStatus, error) {
	if !l.CanMutateSys {
		return LoginUnavailable, nil
	}
	cur, err := l.Backend.Status()
	if err != nil {
		return cur, err
	}
	// Like Swift, nothing is asked of the system when it is as wanted: on
	// Windows registering again would also clear the user's "disabled" mark
	// in Task Manager.
	if (on && cur.Registered()) || (!on && cur == LoginNotRegistered) {
		return cur, nil
	}
	setErr := l.Backend.Set(on)
	got, err := l.Backend.Status()
	if err != nil {
		return got, err
	}
	switch {
	case on && !got.Registered():
		return got, loginFailure(ErrLoginRegister, setErr)
	case !on && got != LoginNotRegistered:
		return got, loginFailure(ErrLoginUnregister, setErr)
	}
	return got, nil
}

// loginFailure is the Swift LoginItemError, with the system's reason.
func loginFailure(kind, cause error) error {
	if cause == nil {
		return kind
	}
	return &loginItemError{kind: kind, cause: cause}
}

// loginItemError keeps diagnostics available without rendering platform output.
type loginItemError struct{ kind, cause error }

func (e *loginItemError) Error() string        { return "login item operation failed" }
func (e *loginItemError) Is(target error) bool { return errors.Is(e.kind, target) }
func (e *loginItemError) Unwrap() error        { return e.cause }

// mygoLoginBackend uses mygo's login API: SMAppService or a launch agent on
// macOS, an XDG autostart entry on Linux, the Run key on Windows. mygo's
// own state check says "off" for an item the user turned off in the system
// settings (Task Manager, the desktop's startup applications), which is why
// the stored choice follows the system and not the other way around.
type mygoLoginBackend struct{}

func (mygoLoginBackend) Status() (LoginStatus, error) {
	if mygo.App.OpenAtLogin() {
		return LoginEnabled, nil
	}
	return LoginNotRegistered, nil
}

func (mygoLoginBackend) Set(on bool) error { return mygo.App.SetOpenAtLogin(on) }

// nativeLoginBackend never calls MyGo's main-thread dispatcher, including
// when the native service fails. Only unavailable platforms choose MyGo.
type nativeLoginBackend struct{}

func (nativeLoginBackend) Status() (LoginStatus, error) {
	st, ok := nativeLoginStatus()
	if !ok {
		return LoginUnavailable, errors.New("could not read SMAppService login status")
	}
	return st, nil
}

func (nativeLoginBackend) Set(on bool) error {
	err, handled := nativeLoginSet(on)
	if !handled {
		return errors.New("SMAppService login registration is unavailable")
	}
	return err
}

// loginHostSupported says whether this process may manage login items:
// released builds (not `go run` or `mygo dev` ones), and on macOS an app
// bundle, as in Swift's supportsHost.
func loginHostSupported(dev bool, goos, exe string) bool {
	if dev {
		return false
	}
	if goos == "darwin" {
		return inAppBundle(exe)
	}
	return true
}

// LoginSystem fills the login fields of System from item. These backend calls
// ask the system; Settings owns the snapshot used by rendering.
func LoginSystem(item LoginItem, s System) System {
	s.LoginSupported = item.CanMutateSys
	if !item.CanMutateSys {
		s.OpenAtLogin, s.SetOpenAtLogin, s.LoginStatus, s.ApplyLogin = nil, nil, nil, nil
		s.LoginAsync = false
		return s
	}
	s.LoginStatus = func() (LoginStatus, error) { return item.Status() }
	s.OpenAtLogin = func() (bool, error) {
		st, err := item.Status()
		return st.Registered(), err
	}
	s.SetOpenAtLogin = func(on bool) error {
		_, err := item.Apply(on)
		return err
	}
	s.ApplyLogin = item.Apply
	return s
}

// ProductionLoginItem is the login item of this process (exe is its
// executable path).
func ProductionLoginItem(exe string) LoginItem {
	var backend LoginBackend = mygoLoginBackend{}
	if nativeLoginAvailable() {
		backend = nativeLoginBackend{}
	}
	backend = legacyMigratingLoginBackend(exe, backend)
	return LoginItem{
		Backend:      backend,
		CanMutateSys: loginHostSupported(mygo.IsDev(), runtime.GOOS, exe),
	}
}

// inAppBundle reports whether exe lies inside a macOS app bundle.
func inAppBundle(exe string) bool {
	return strings.Contains(exe, ".app/Contents/MacOS/")
}

// LoginStatus is the state of the login item for the settings page: it tells
// "waiting for approval" apart from enabled. LoginUnavailable when the
// system offers no status.
func (s *Settings) LoginStatus() LoginStatus {
	return s.loginStatus
}
