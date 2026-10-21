package app

import (
	"errors"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/licoy/quosparr/internal/l10n"
)

// releasesURL is the formal download entry, offered when an installation
// cannot update itself.
const releasesURL = "https://github.com/Licoy/Quosparr/releases/latest"

// UpdateInterval is the time between background update checks while the
// app runs; mygo runs the first one of a launch shortly after it starts.
const UpdateInterval = time.Hour

// UpdateBlock says why an update check cannot run at all.
type UpdateBlock int

const (
	// UpdateOK: the check can run.
	UpdateOK UpdateBlock = iota
	// UpdateBlockedDev: a development build.
	UpdateBlockedDev
	// UpdateBlockedPackage: installed by a package manager (a .deb).
	UpdateBlockedPackage
	// UpdateBlockedLocation: the install location is not writable.
	UpdateBlockedLocation
	// UpdateBlockedNetwork: the network (proxy) settings cannot be used, so
	// nothing is sent; there is no direct fallback.
	UpdateBlockedNetwork
)

// classifyUnavailable names why mygo's Updater.Enabled is false. exe is the
// path of the running executable.
func classifyUnavailable(goos, exe string, dev bool) UpdateBlock {
	if dev {
		return UpdateBlockedDev
	}
	if goos == "linux" {
		for _, p := range []string{"/usr/", "/opt/"} {
			if strings.HasPrefix(exe, p) {
				return UpdateBlockedPackage
			}
		}
	}
	return UpdateBlockedLocation
}

// UpdateService is the app's side of the mygo updater: the native update
// window comes from mygo, the rules around it from here.
//
//   - automaticallyChecksForUpdates in the preferences is the only truth for
//     background checks; the updater's own copy is overwritten from it
//     before its scheduler starts and on every change;
//   - nothing is downloaded or installed without the user's say (the update
//     window's "automatically download" box is the user's, off by default);
//   - a manual check is always allowed, and never says "up to date" when
//     the check could not run.
type UpdateService struct {
	// Seams over mygo, replaced by tests.
	enabled  func() bool
	checkUI  func()
	setAuto  func(bool)
	route    func() error // nil: the request route is fine
	exe      func() string
	dev      func() bool
	goos     string
	openURL  func(string) error
	confirm  func(title, message string, buttons []string) int
	runAsync func(func())

	lastAuto    bool // what the updater was last told
	autoKnown   bool
	autoApply   sync.Mutex
	autoMu      sync.Mutex
	autoPending *bool
	autoRunning bool
	autoIdle    sync.WaitGroup
}

// UpdateOptions configures UseUpdater.
type UpdateOptions struct {
	// Language is the updater window's language tag (see UpdateLanguage).
	Language string
	// Icon is the PNG shown in the update window.
	Icon []byte
	// Client returns the client of each update request. Nil uses the
	// network context of internal/netx with the update policy.
	Client ClientFunc
}

// UpdateLanguage maps the app's language to the update window's tag. The
// window takes its language once, when it first shows.
func UpdateLanguage(l l10n.Language) string {
	switch l {
	case l10n.SimplifiedChinese:
		return "zh-Hans"
	case l10n.TraditionalChinese:
		return "zh-Hant"
	case l10n.Korean:
		return "ko"
	case l10n.Japanese:
		return "ja"
	case l10n.Russian:
		return "ru"
	case l10n.French:
		return "fr"
	}
	return "en"
}

// UseUpdater registers mygo's native update window and gives its requests
// the user's network settings. It must run before App.Run, after
// DirectMygoPaths (the updater keeps updater.json in ~/.quosparr) and
// once only. Background checks are off until ApplyAutomaticChecks.
func UseUpdater(opts UpdateOptions) (*UpdateService, error) {
	if opts.Icon == nil {
		opts.Icon = appIconPNG
	}
	s := newUpdateService()
	client := opts.Client
	if client == nil {
		client = netxUpdateClient
	}
	// Before the updater can run: its requests use http.DefaultClient (see
	// updaterTransport), which now sends them through the user's settings.
	configureUpdaterHTTP(client)
	err := usePlugin(native.New(updater.Options{
		Interval:               UpdateInterval,
		DisableAutomaticChecks: true,
		Language:               opts.Language,
		Icon:                   opts.Icon,
	}))
	if err != nil {
		return nil, err
	}
	return s, nil
}

// usePlugin is mygo.Use, with its panic turned into an error.
func usePlugin(p mygo.Plugin) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("updater: " + panicText(r))
		}
	}()
	mygo.Use(p)
	return nil
}

func panicText(r any) string {
	if e, ok := r.(error); ok {
		return e.Error()
	}
	if s, ok := r.(string); ok {
		return s
	}
	return "plugin failed"
}

func newUpdateService() *UpdateService {
	s := &UpdateService{
		enabled:  mygo.Updater.Enabled,
		checkUI:  updater.CheckForUpdates,
		setAuto:  updater.SetAutomaticChecks,
		goos:     runtime.GOOS,
		dev:      func() bool { return developmentPackage || mygo.IsDev() },
		openURL:  mygo.Shell.OpenExternal,
		runAsync: func(f func()) { go f() },
	}
	s.exe = func() string { p, _ := os.Executable(); return p }
	s.route = updateRoute
	s.confirm = func(title, message string, buttons []string) int {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Type: mygo.MessageInfo, Title: title, Message: title, Detail: message, Buttons: buttons,
		})
		if err != nil {
			return len(buttons) - 1
		}
		return res.Button
	}
	return s
}

// Block says whether a check can run now and, if not, why.
func (s *UpdateService) Block() UpdateBlock {
	if !s.enabled() {
		return classifyUnavailable(s.goos, s.exe(), s.dev())
	}
	if s.route != nil && s.route() != nil {
		return UpdateBlockedNetwork
	}
	return UpdateOK
}

// ApplyAutomaticChecks makes the updater's background checks follow the
// preference. allowed is false while the settings or the proxy are
// unusable: no network task may run then. Call it before App.Run and after each change; the updater's scheduler
// reads the value when it starts.
func (s *UpdateService) ApplyAutomaticChecks(on, allowed bool) {
	s.autoApply.Lock()
	defer s.autoApply.Unlock()
	on = on && allowed
	if s.autoKnown && s.lastAuto == on {
		return
	}
	s.lastAuto, s.autoKnown = on, true
	s.setAuto(on)
}

// RequestAutomaticChecks applies user changes without waiting for mygo's
// synchronous state-file write. One worker preserves order and coalesces
// pending changes; the initial ApplyAutomaticChecks still runs before Run.
func (s *UpdateService) RequestAutomaticChecks(on, allowed bool) {
	on = on && allowed
	s.autoMu.Lock()
	s.autoPending = &on
	if s.autoRunning {
		s.autoMu.Unlock()
		return
	}
	s.autoRunning = true
	s.autoIdle.Add(1)
	s.autoMu.Unlock()
	go s.drainAutomaticChecks()
}

func (s *UpdateService) drainAutomaticChecks() {
	defer s.autoIdle.Done()
	for {
		s.autoMu.Lock()
		on := s.autoPending
		s.autoPending = nil
		if on == nil {
			s.autoRunning = false
			s.autoMu.Unlock()
			return
		}
		s.autoMu.Unlock()
		s.ApplyAutomaticChecks(*on, true)
	}
}

// FlushAutomaticChecks waits for state-file writes, which never wait for
// main-thread delivery. Call after Settings.Flush and background shutdown.
func (s *UpdateService) FlushAutomaticChecks() { s.autoIdle.Wait() }

// Reapply applies the stored preference to the updater, for start-up and for
// events that change whether network tasks may run (a fixed proxy).
func (s *UpdateService) Reapply(settings *Settings) {
	s.ApplyAutomaticChecks(settings.Prefs().AutomaticallyChecksForUpdates, settings.BackgroundAllowed())
}

// CheckNow is the user's "Check for Updates": it opens mygo's update window
// when the check can run, else says why not and offers the download page.
// It is not blocked by the automatic-check preference. Call it on the main
// thread (a button click); l is the language in effect.
func (s *UpdateService) CheckNow(l l10n.Localizer) {
	s.runAsync(func() {
		block := s.Block()
		if block == UpdateOK {
			s.checkUI()
			return
		}
		s.explain(l, block)
	})
}

// explain tells the user why updates cannot run and offers the download
// page. It blocks on a dialog: call it off the main thread.
func (s *UpdateService) explain(l l10n.Localizer, block UpdateBlock) {
	if s.confirm(tx(l, xUpdateUnavailableTitle), blockText(l, block), blockButtons(l, block)) == 0 && block != UpdateBlockedDev {
		if err := s.openURL(releasesURL); err != nil {
			log.Println("updater: open download page:", err)
		}
	}
}

func blockText(l l10n.Localizer, b UpdateBlock) string {
	switch b {
	case UpdateBlockedDev:
		return tx(l, xUpdateUnavailableDev)
	case UpdateBlockedPackage:
		return tx(l, xUpdateUnavailablePackage)
	case UpdateBlockedNetwork:
		return tx(l, xUpdateNetworkBlocked)
	}
	return tx(l, xUpdateUnavailableLocation)
}

// blockButtons: the download page first (the default), then Close. A
// development build has no release to download.
func blockButtons(l l10n.Localizer, b UpdateBlock) []string {
	if b == UpdateBlockedDev {
		return []string{tx(l, xUpdateClose)}
	}
	return []string{tx(l, xUpdateOpenDownloadPage), tx(l, xUpdateClose)}
}
