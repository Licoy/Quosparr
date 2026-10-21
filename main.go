// Quosparr is a menu-bar / system-tray monitor for local Codex quota,
// reset credits and API-equivalent cost.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/app"
	"github.com/licoy/quosparr/internal/migration"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/sqliteruntime"
)

// mygoJSON is the packaging config, which holds the product version.
//
//go:embed mygo.json
var mygoJSON []byte

// appVersion is the version of the packaged app, or, run with go run or
// go build, the version in mygo.json.
func appVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	var cfg struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(mygoJSON, &cfg) != nil {
		return ""
	}
	return cfg.Version
}

func main() {
	if os.Getenv("QUOSPARR_DEV_VERIFY_SQLITE") == "1" {
		if err := sqliteruntime.Verify(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("SQLite runtime: load and parameterized query passed")
		return
	}
	command := app.ParseArgs(os.Args[1:])
	switch command {
	case app.CommandHelp:
		fmt.Print(app.HelpText)
		return
	case app.CommandSelfCheck:
		home, _ := platform.Home()
		app.SelfCheck(os.Stdout, home, appVersion())
		return
	}
	os.Exit(run(command))
}

// run starts the app. Startup order (plan 3.5): single-instance lock ->
// config -> legacy migration -> proxy credentials -> system state ->
// background tasks. It returns the exit code.
func run(command app.Command) int {
	// A process that Quosparr itself started (the relaunch after an update)
	// may start while the old one still holds the lock; read the mark before
	// anything else, then leave one for the processes this one starts.
	relaunched := app.TakeRelaunchMark()
	app.MarkRelaunchable()

	mg := mygo.App
	mg.SetName("Quosparr")

	// Dev mode (a scratch preferences domain, see app.DevDomainEnv) never
	// hands over to a running real instance and never changes the machine.
	st, dev, err := app.OpenStore()
	if err != nil {
		log.Println("preferences store:", err)
		return 1
	}

	// A second Go instance hands over to the first, which opens the requested UI.
	// This comes before the file lock only so that hand-over works; nothing
	// is read or written until the file lock is held.
	if !dev && !mg.RequestSingleInstanceLock() {
		return 0
	}

	home, err := platform.Home()
	if err != nil {
		log.Println("home directory:", err)
		return 1
	}
	if !dev {
		legacyLock, err := platform.AcquireLegacyLock(home, func(path string) (*platform.Lock, error) {
			return app.AcquireInstanceLock(path, relaunched)
		})
		if errors.Is(err, platform.ErrLocked) {
			// CodexRunway runs: explain it (and offer to quit it) instead of
			// exiting silently. Nothing has been read or written yet.
			if err := app.RunLegacyBlocked(); err != nil {
				log.Println("previous app running:", err)
				return 1
			}
			return 0
		}
		if err != nil {
			log.Println("previous app lock:", err)
			return 1
		}
		defer legacyLock.Release()
		if migrated, err := migration.Run(home); err != nil {
			log.Println("app data migration:", err)
			return 1
		} else if migrated {
			log.Println("migrated previous app data to ~/.quosparr; original preserved")
		}
	}

	// The same file the Swift app locks: the two can never run together.
	// A relaunched process waits for the old one to release the lock; the lock
	// file is never removed.
	lock, err := app.AcquireInstanceLock(platform.LockPath(home), relaunched)
	if errors.Is(err, platform.ErrLocked) {
		return 0 // another Quosparr (Swift or Go) is running: exit quietly
	}
	if err != nil {
		log.Println("single-instance lock:", err)
		return 1
	}
	defer lock.Release()

	if err := app.DirectMygoPaths(mg, home); err != nil {
		log.Println("data directory:", err)
		return 1
	}

	sys := app.DefaultSystem()
	if dev {
		sys = app.DevSystem()
	}
	// The "check automatically" switch reaches the updater through the
	// System; the updater is created once the settings are loaded.
	var (
		settings *app.Settings
		upd      *app.UpdateService
	)
	if !dev {
		sys.SetAutomaticUpdateChecks = func(on bool) {
			if upd != nil {
				upd.RequestAutomaticChecks(on, settings.BackgroundAllowed())
			}
		}
	}
	creds := app.NewPlatformCredentials()
	settings = app.NewSettings(st, sys, creds)
	app.UseProxy(settings, func() {
		if upd != nil {
			upd.RequestAutomaticChecks(settings.Prefs().AutomaticallyChecksForUpdates, settings.BackgroundAllowed())
		}
	})

	// LoadPreferences already imports the Swift app's legacy domain when the
	// current one has no settings, so MigrateLegacy is not a separate step.
	// A load error leaves the settings read-only and nothing is saved over
	// the damaged value; a proxy error blocks the background tasks.
	// The login item is reconciled once the app runs (App.Ready).
	seams := app.Seams{
		LoadConfig:  settings.Load,
		VerifyProxy: settings.LoadProxy,
	}
	report := seams.RunSteps()
	app.InstallStartupProxy(settings, creds)
	for _, p := range report.Problems {
		log.Println("startup:", p)
	}
	firstLaunch := false
	if !settings.ReadOnly() {
		firstLaunch, err = settings.InitializeFirstLaunch()
		if err != nil {
			log.Println("initial preferences:", err)
			return 1
		}
	}
	settings.Apply() // language and appearance, before any window exists

	// The updater keeps its state in ~/.quosparr (DirectMygoPaths above).
	// The preference is the only truth for background checks: it is applied
	// before the updater's scheduler starts (when the app is ready). A dev run
	// leaves the updater's state alone.
	upd, err = app.UseUpdater(app.UpdateOptions{
		Language: app.UpdateLanguage(settings.L().Language()),
	})
	if err != nil {
		log.Println("updater:", err)
	}
	if upd != nil && !dev {
		// mygo checks soon after each launch (when checks are on), then hourly.
		upd.Reapply(settings)
	}
	notifier := app.NewNotifier()
	actions := app.Actions{
		TestNotification: notifier.TestAction(settings.L, app.ShowDialog),
		Refresh:          app.RequestLiveRefresh,
		RepairIndex:      app.RequestLiveSessionRepair,
		SelfCheck: func() string {
			var buf strings.Builder
			app.SelfCheck(&buf, home, appVersion())
			return buf.String()
		},
		OpenFolder: app.OpenFolder,
	}
	if upd != nil {
		actions.CheckForUpdates = func() { upd.CheckNow(settings.L()) }
	}

	var ui *app.App
	seams.StartBackground = func(ctx context.Context) func() {
		return app.StartBackground(ctx, app.BackgroundOptions{
			Home: home, Settings: settings, Notifier: notifier, Version: appVersion(), Host: ui,
		})
	}
	ui = app.New(app.Options{
		Version: appVersion(), Seams: seams, Report: report, Settings: settings, Actions: actions,
	})
	launch := app.LaunchActions{
		Ready: ui.Ready, OpenPanel: ui.OpenPanel, OpenSettings: ui.OpenSettings, FirstLaunch: firstLaunch,
	}
	mg.OnSecondInstance(launch.SecondInstance)
	mg.OnOpenURL(func(raw string) { mg.WhenReady(func() { ui.OpenWidgetURL(raw) }) })
	mg.OnActivate(func(bool) { app.RefreshWidgetConfigurations() })
	// A click on a notification (also the one that launched the app) opens the
	// panel.
	mg.OnNotificationClick(func(string) { ui.OpenPanel() })
	// OnQuit follows App.shutdown (settings rollback and background stop),
	// then waits only for updater writes, never for a main-thread callback.
	mg.OnQuit(func() {
		if upd != nil {
			upd.FlushAutomaticChecks()
		}
	})
	// A tray app keeps running with no window open and has no Dock icon.
	mg.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mg.OnWindowAllClosed(func() {})
	mg.WhenReady(func() { launch.InitialReady(command) })
	if tab := os.Getenv("QUOSPARR_DEV_SETTINGS_TAB"); dev && tab != "" {
		// Dev only: open the settings window on a tab at start-up.
		n, _ := strconv.Atoi(tab)
		mg.WhenReady(func() { ui.OpenSettingsTab(n) })
		if base := os.Getenv("QUOSPARR_DEV_SCREENSHOT"); base != "" {
			mg.WhenReady(func() { go devScreenshots(ui, base) })
		}
	}
	if os.Getenv(app.TierPreviewEnv) != "" && app.TierPreviewAllowed(dev) {
		// Dev only: show every subscription badge, light and dark.
		mg.WhenReady(ui.OpenTierPreview)
		if base := os.Getenv("QUOSPARR_DEV_SCREENSHOT"); base != "" {
			mg.WhenReady(func() { go devTierScreenshot(ui, base) })
		}
	}
	if err := ui.RunDesktop(); err != nil {
		log.Println("run:", err)
		return 1
	}
	return 0
}

// devScreenshots writes a PNG of each settings tab to base-<tab>.png and
// quits. Dev only (a scratch preferences domain is set).
func devScreenshots(ui *app.App, base string) {
	time.Sleep(1500 * time.Millisecond)
	for tab := 0; tab < app.SettingsTabCount(); tab++ {
		mygo.RunOnMain(func() { ui.OpenSettingsTab(tab) })
		time.Sleep(700 * time.Millisecond)
		png, err := ui.CaptureSettings()
		if err != nil {
			log.Println("screenshot:", err)
			continue
		}
		_ = os.WriteFile(fmt.Sprintf("%s-%d.png", base, tab), png, 0o600)
	}
	mygo.App.Quit()
}

// devTierScreenshot writes the badge preview to base-tiers.png and quits.
func devTierScreenshot(ui *app.App, base string) {
	time.Sleep(1500 * time.Millisecond)
	if png, err := ui.CaptureTierPreview(); err != nil {
		log.Println("screenshot:", err)
	} else {
		_ = os.WriteFile(base+"-tiers.png", png, 0o600)
	}
	mygo.App.Quit()
}
