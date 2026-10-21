// Package app is the Quosparr host: tray icon, panel and the startup
// sequence. Views run on the main thread.
package app

import (
	"context"
	"errors"
	"log"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/cachehit"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/resettoday"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// panelWidth matches the Swift main panel (MainPanelLayout.width).
const panelWidth = 400

// PanelActions are the panel and accounts controls. A nil func disables
// that control. The live service installs them; tests leave them nil.
type PanelActions struct {
	Refresh             func()
	RefreshSection      func(runway.Section)
	SwitchAccount       func(id string)
	SelectProvider      func(store.Provider)
	SetCursorMonitoring func(bool)
	// PanelShown reports the panel opening and closing; the panel provider's
	// local data loads only while it shows.
	PanelShown    func(open bool)
	QueryCost     func(context.Context, store.Provider, CostQuery) (usage.Summary, error)
	QueryCache    func(context.Context, store.Provider, CostQuery) (cachehit.Report, error)
	QueryReaction func(context.Context) (resettoday.ReactionSnapshot, error)
	React         func(context.Context) (resettoday.ReactionPostResult, error)
	// RepairSessions rebuilds the session index after a backup; the panel
	// asks for confirmation first.
	RepairSessions func()
	// Accounts are the multi-account page controls.
	Accounts AccountActions
	// Skills are the skills page controls.
	Skills SkillActions
	// Sessions are the local session-management controls.
	Sessions SessionActions
	// MCP manages user-scoped MCP declarations without starting servers.
	MCP MCPActions
}

// Options configures App.
type Options struct {
	// Version is shown in the panel and the tray tooltip.
	Version string
	// Seams are the startup steps other packages provide.
	Seams Seams
	// Report is the outcome of Seams.RunSteps, run before the app starts.
	Report StartupReport
	// Settings holds the loaded preferences (see Config). Nil gives
	// throwaway in-memory defaults.
	Settings *Settings
	// Actions are what the settings buttons start; nil ones are disabled.
	Actions Actions
}

// App owns the tray and the panel.
type App struct {
	opts     Options
	tray     *mygo.Tray
	panel    *mygo.Window
	settings *Settings
	setWin   *mygo.Window
	tierWin  *mygo.Window
	setUI    *settingsUI

	popover     panelPopoverState
	panelState  panelState
	panelResize panelResizeState
	// panelClosedAt tells the floating window's click apart from the press
	// that took the panel's focus away.
	panelClosedAt time.Time
	floating      floatingState

	snapMu    sync.Mutex
	snap      runway.Snapshot
	panelActs PanelActions

	backgroundReady bool
	stopBackground  func()
	cancel          context.CancelFunc
	startupErr      error
}

// panelSize is the flyout size in DIPs. Height follows the saved preference.
func (a *App) panelSize() (int, int) {
	display := a.panelDisplay()
	return a.panelSizeWithin(display.Bounds.Height, display.WorkArea.Height)
}

func (a *App) panelDisplay() mygo.Display {
	if !mygo.App.IsReady() {
		return mygo.Display{} // No native screen before startup or in UI tests.
	}
	if a.panel != nil {
		return mygo.Screen.DisplayMatching(a.panel.Bounds())
	}
	if a.tray != nil {
		return mygo.Screen.DisplayMatching(a.tray.Bounds())
	}
	return mygo.Screen.PrimaryDisplay()
}

func (a *App) panelSizeWithin(screenHeight, available int) (int, int) {
	h := store.DefaultMainPanelHeight
	if a.settings != nil {
		h = a.settings.Prefs().MainPanelHeight
	}
	return panelWidth, int(clampPanelHeight(h, screenHeight, available))
}

// Snapshot is the latest coordinator view. The zero value is what the
// panel shows before the first publish.
func (a *App) Snapshot() runway.Snapshot {
	a.snapMu.Lock()
	defer a.snapMu.Unlock()
	return a.snap
}

// PublishSnapshot stores s and redraws the panel. Safe from any goroutine.
func (a *App) PublishSnapshot(s runway.Snapshot) {
	a.snapMu.Lock()
	a.snap = s
	a.snapMu.Unlock()
	mygo.RunOnMain(func() {
		if a.panel != nil {
			a.panel.Invalidate()
		}
		if a.setWin != nil && !a.setWin.IsDestroyed() {
			a.setWin.Invalidate()
		}
		if a.floating.win != nil {
			a.syncFloating()
		}
	})
}

// PanelActions returns the controls installed for the panel.
func (a *App) PanelActions() PanelActions {
	a.snapMu.Lock()
	defer a.snapMu.Unlock()
	return a.panelActs
}

// SetPanelActions installs the panel controls. Call it before the panel opens.
func (a *App) SetPanelActions(p PanelActions) {
	a.cancelPanelDetails()
	a.snapMu.Lock()
	a.panelActs = p
	a.snapMu.Unlock()
}

// New builds the host. Call Ready from mygo's WhenReady.
func New(opts Options) *App {
	a := &App{opts: opts, settings: opts.Settings}
	if a.settings == nil {
		a.settings = NewSettings(store.New(store.NewMemoryDomain(), nil), System{}, UnavailableCredentials{})
		_ = a.settings.Load()
		_ = a.settings.LoadProxy()
	}
	a.settings.OnChange(a.settingsChanged)
	a.settings.OnLoginChange(func() {
		if a.setWin != nil && !a.setWin.IsDestroyed() {
			a.setWin.Invalidate()
		}
	})
	return a
}

// L is the localizer of the language in effect.
func (a *App) L() l10n.Localizer { return a.settings.L() }

// settingsChanged follows a change of the settings: the windows redraw, and
// what carries text (the tray, the settings window title) is translated.
func (a *App) settingsChanged() {
	a.refreshStartupReport()
	if a.backgroundReady {
		a.startBackground()
	}
	if a.panel != nil {
		w, h := a.panelSize()
		a.panel.SetContentSize(w, h)
		a.matchPanelAppearance()
		a.panel.Invalidate()
	}
	if a.setWin != nil && !a.setWin.IsDestroyed() {
		a.setWin.SetTitle(a.L().Text(l10n.KeySettings))
		a.setWin.Invalidate()
	}
	if a.tray != nil && runtime.GOOS == "linux" {
		a.tray.SetMenu(a.trayMenu())
	}
	a.syncFloating()
}

// Ready creates the tray and starts the background tasks. It runs on the
// main thread once mygo is ready.
func (a *App) Ready() {
	// The login item can only be read once the app runs.
	_ = a.settings.ReconcileLogin()
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.shutdown() })
	if err := a.readyWithTray(a.createTray); err != nil {
		log.Println("tray:", err)
		mygo.Dialog.Error("Quosparr", err.Error())
		mygo.App.Quit()
		return
	}
	mygo.App.OnDidBecomeActive(func() {
		if a.setWin != nil && a.setWin.IsVisible() {
			a.settings.RefreshLogin()
		}
	})
}

// A failed tray is terminal: no background work or requested settings
// window may start while Quit is waiting for the event loop to return.
func (a *App) readyWithTray(create func() error) error {
	if err := create(); err != nil {
		a.startupErr = err
		return err
	}
	a.backgroundReady = true
	a.startBackground()
	a.syncFloating()
	return nil
}

// createTray adds the tray icon. On Linux without AppIndicator the app has
// no entry point at all, so it says so and quits instead of idling invisibly.
func (a *App) createTray() error {
	icon, template := trayIcon()
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           icon,
		IconIsTemplate: template,
		ToolTip:        "Quosparr",
	})
	if err != nil {
		return trayStartupError(a.L(), err)
	}
	a.tray = tray
	a.wireTray()
	// A mono icon follows the taskbar, which may turn light or dark; the
	// popover follows the app's appearance.
	mygo.Theme.OnUpdated(func() {
		a.matchPanelAppearance()
		ApplyTray(a.tray, TrayFromStatus(a.Snapshot().StatusBar))
		if a.floating.win != nil {
			a.floating.win.Invalidate()
		}
	})
	return nil
}

func (a *App) wireTray() {
	if runtime.GOOS == "linux" {
		// AppIndicator shows menus only and reports no clicks: the menu
		// carries "Open" so the panel is always reachable.
		a.tray.SetMenu(a.trayMenu())
		return
	}
	a.tray.OnClick(a.TogglePanel)
	a.tray.OnRightClick(func() {
		a.ClosePanel()
		a.tray.SetMenu(a.trayMenu())
		defer a.tray.SetMenu(nil)
		a.tray.PopUpMenu()
	})
}

func (a *App) trayMenu() *mygo.Menu {
	l := a.L()
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: l.Text(l10n.KeyOpenControlPanel), Click: func(*mygo.MenuItem, *mygo.Window) { a.OpenPanel() }},
		{Label: l.Text(l10n.KeySettings), Click: func(*mygo.MenuItem, *mygo.Window) { a.OpenSettings() }},
		{Label: tx(l, xFloatingEnabled), Type: mygo.MenuItemCheckbox, Checked: a.settings.Prefs().FloatingWindow.Enabled,
			Disabled: a.settings.ReadOnly(), Click: func(item *mygo.MenuItem, _ *mygo.Window) {
				a.settings.SetFloatingWindow(func(f *store.FloatingWindow) { f.Enabled = item.Checked })
			}},
		mygo.Separator(),
		{Label: l.Text(l10n.KeyQuit), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

// TogglePanel opens the panel, or closes it when open.
func (a *App) TogglePanel() {
	if a.panel != nil {
		a.ClosePanel()
		return
	}
	a.OpenPanel()
}

// OpenPanel shows the panel next to the tray icon, or focuses it.
func (a *App) OpenPanel() {
	if a.tray == nil {
		return // not ready yet
	}
	if a.panel != nil {
		a.panel.Focus()
		return
	}
	w, h := a.panelSize()
	opts := mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Gap:       4,
		Width:     w,
		Height:    h,
		Focusable: true,
		Shadow:    true,
		Popover:   runtime.GOOS == "darwin",
		Content:   ui.View(a.panelView),
	}
	stop := observePanelPopover(a)
	func() {
		defer stop()
		a.panel = mygo.NewFlyout(opts)
	}()
	panel := a.panel
	a.syncPanelResizeEdge()
	panel.OnShow(a.matchPanelAppearance)
	panel.OnFocus(a.matchPanelAppearance)
	a.panelOpened()
	panel.OnClosed(a.panelClosed)
	if err := a.watchPanelDismiss(); err != nil {
		log.Printf("open main panel: %v", err)
		a.ClosePanel()
	}
}

// panelOpened runs once per new flyout, separately from repaint/focus events.
func (a *App) panelOpened() {
	a.matchPanelAppearance()
	if a.panel != nil {
		a.panel.Invalidate()
	}
	prefs, actions := a.settings.Prefs(), a.PanelActions()
	if actions.PanelShown != nil {
		actions.PanelShown(true)
	}
	if prefs.SelectedProvider == store.ProviderCodex && prefs.ShowsRateLimitResetToday && actions.RefreshSection != nil {
		// The queued section refresh bypasses the timer interval and coalesces
		// in-flight requests, keeping the last snapshot visible while fetching.
		actions.RefreshSection(runway.SectionResetToday)
	}
}

func (a *App) panelClosed() {
	if shown := a.PanelActions().PanelShown; shown != nil {
		shown(false)
	}
	a.cancelPanelDetails()
	a.panelState.detail = ""
	a.panelState.chartMode = ""
	a.panelState.scrollDetail = ""
	a.panelState.mainScroll = ui.ScrollState{}
	a.panelState.detailScroll = ui.ScrollState{}
	a.panelResize = panelResizeState{}
	releasePanelPopover(a)
	a.panel = nil
	a.panelClosedAt = time.Now()
}

// ClosePanel closes the panel.
func (a *App) ClosePanel() {
	if a.panel != nil {
		a.panel.Close()
	}
}

// startBackground runs the background seam unless startup found the config
// or proxy unusable.
func (a *App) refreshStartupReport() {
	problems := make([]Problem, 0, len(a.opts.Report.Problems))
	blocked := !a.settings.BackgroundAllowed()
	for _, problem := range a.opts.Report.Problems {
		if problem.Step == "config" || problem.Step == "proxy credentials" {
			continue
		}
		problems = append(problems, problem)
		if problem.Step == "legacy migration" {
			blocked = true
		}
	}
	if err := a.settings.LoadError(); err != nil {
		problems = append(problems, Problem{Step: "config", Err: err})
	}
	if err := a.settings.ProxyError(); err != nil {
		problems = append(problems, Problem{Step: "proxy credentials", Err: err})
	}
	a.opts.Report = StartupReport{Problems: problems, BackgroundAllowed: !blocked}
}

func (a *App) startBackground() {
	a.refreshStartupReport()
	if !a.opts.Report.BackgroundAllowed {
		if a.cancel != nil {
			a.cancel()
			a.cancel = nil
		}
		if a.stopBackground != nil {
			a.stopBackground()
			a.stopBackground = nil
			a.SetPanelActions(PanelActions{})
		}
		return
	}
	if a.cancel != nil || a.opts.Seams.StartBackground == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.stopBackground = a.opts.Seams.StartBackground(ctx)
}

// OpenSettingsTab opens the settings window on tab 0..10 (general,
// accounts, appearance & menu, floating window, panel modules,
// notifications, sessions, skills, MCP, advanced, about); for dev runs and screenshots.
func (a *App) OpenSettingsTab(tab int) {
	a.OpenSettings()
	if a.setUI != nil && tab >= 0 && tab < len(settingsTabs) {
		a.setUI.tab = settingsTabs[tab]
		a.setWin.Invalidate()
	}
}

// openAccountsSettings opens the accounts tab on one provider kind
// (accountKindCodex and so on). A busy form keeps its provider.
func (a *App) openAccountsSettings(kind int) {
	a.OpenSettingsTab(1)
	if a.setUI == nil {
		return
	}
	if f := accountFormOf(a.setUI); !f.busy && f.kind != kind {
		f.kind, f.mode, f.paste, f.api, f.selection = kind, "", "", apiDraft{}, nil
	}
}

// openSkillsSettings opens the skills tab on one CLI's list.
func (a *App) openSkillsSettings(p store.Provider) {
	a.OpenSettingsTab(slices.Index(settingsTabs, tabSkills))
	if a.setUI == nil {
		return
	}
	a.setUI.skillsState().selectProvider(p)
}

// CaptureSettings returns a PNG of the settings window, for dev runs.
func (a *App) CaptureSettings() ([]byte, error) {
	var w *mygo.Window
	mygo.RunOnMain(func() { w = a.setWin })
	if w == nil {
		return nil, errors.New("settings window is not open")
	}
	return w.CapturePage()
}

// OpenSettings shows the settings window, or focuses it.
func (a *App) OpenSettings() {
	if a.startupErr != nil {
		return
	}
	a.ClosePanel()
	if a.setWin != nil {
		a.settings.RefreshLogin()
		a.setWin.Show()
		a.setWin.Focus()
		w, h := a.setWin.Size()
		log.Printf("settings window: visible=%t size=%dx%d", a.setWin.IsVisible(), w, h)
		return
	}
	actions := a.opts.Actions
	if actions.OpenFolder == nil {
		actions.OpenFolder = openFolder
	}
	a.setUI = newSettingsUI(a.settings, SettingsOptions{
		Version:      a.opts.Version,
		Actions:      actions,
		Skills:       func() SkillActions { return a.PanelActions().Skills },
		Sessions:     func() SessionActions { return a.PanelActions().Sessions },
		MCP:          func() MCPActions { return a.PanelActions().MCP },
		PanelDisplay: a.panelDisplay,
	})
	a.setWin = mygo.NewWindow(mygo.WindowOptions{
		StateKey:             "settings",
		Title:                a.L().Text(l10n.KeySettings),
		Width:                settingsWidth,
		Height:               settingsHeight,
		MinWidth:             settingsMinWidth,
		MinHeight:            settingsMinHeight,
		TitleBarStyle:        mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: 18, Y: 20},
		Vibrancy:             settingsVibrancy(),
		Content:              ui.View(a.setUI.view),
	})
	// Restore keeps the saved dimensions, so enlarge older small settings windows.
	if w, h := a.setWin.Size(); w < settingsMinWidth || h < settingsMinHeight {
		a.setWin.SetSize(max(w, settingsMinWidth), max(h, settingsMinHeight))
	}
	a.setUI.win = a.setWin
	a.setWin.OnFocus(a.settings.RefreshLogin)
	a.settings.RefreshLogin()
	a.setWin.OnClosed(func() {
		if a.setUI != nil {
			a.setUI.cancelAccountTask()
			a.setUI.cancelSessionTasks()
			a.setUI.cancelMCPTasks()
		}
		a.setWin = nil
		a.setUI = nil
	})
	w, h := a.setWin.Size()
	log.Printf("settings window: visible=%t size=%dx%d", a.setWin.IsVisible(), w, h)
}

func (a *App) shutdown() {
	a.cancelPanelDetails()
	if a.setUI != nil {
		a.setUI.cancelAccountTask()
		a.setUI.cancelSessionTasks()
		a.setUI.cancelMCPTasks()
	}
	// A change made just before quitting still reaches the store.
	a.settings.Flush()
	if a.cancel != nil {
		a.cancel()
	}
	if a.stopBackground != nil {
		a.stopBackground()
	}
	if a.tray != nil {
		a.tray.Destroy()
	}
}
