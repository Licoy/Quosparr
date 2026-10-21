package app

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// Settings window size in DIPs.
const (
	settingsWidth        = 980
	settingsHeight       = 720
	settingsMinWidth     = 820
	settingsMinHeight    = 560
	settingsSidebarWidth = 240
)

// proxyMsgTime is how long a successful connection test stays shown; tests
// shorten it.
var proxyMsgTime = 3 * time.Second

// Actions are the operations of other packages that the settings buttons
// start. A nil one disables its button.
type Actions struct {
	Refresh func()
	// SelfCheck builds the redacted local report on a worker.
	SelfCheck func() string
	// RepairIndex rebuilds the session index (after the user confirmed).
	RepairIndex func()
	// TestNotification sends a test notification and says what happened.
	TestNotification func() string
	CheckForUpdates  func()
	// OpenFolder opens a folder in the file manager.
	OpenFolder func(path string)
}

// SettingsOptions configures the settings window.
type SettingsOptions struct {
	Version string
	Actions Actions
	// Skills returns the skills actions of the running app; nil or an
	// empty result leaves the skills tab read-only.
	Skills func() SkillActions
	// Sessions returns read and delete operations for local CLI sessions.
	Sessions func() SessionActions
	// MCP returns read and delete operations for global native MCP entries.
	MCP func() MCPActions
	// PanelDisplay returns the display used by the main panel.
	PanelDisplay func() mygo.Display
}

type settingsTab int

const (
	tabGeneral settingsTab = iota
	tabAccounts
	tabDisplay
	tabStatusBar
	tabFloating
	tabModules
	tabNotifications
	tabSessions
	tabSkills
	tabMCP
	tabAdvanced
	tabAbout
)

var settingsTabs = []settingsTab{tabGeneral, tabAccounts, tabDisplay, tabStatusBar, tabFloating, tabModules, tabNotifications, tabSessions, tabSkills, tabMCP, tabAdvanced, tabAbout}

// title is the tab's name in the sidebar and above its page.
func (t settingsTab) title(l l10n.Localizer) string {
	switch t {
	case tabStatusBar:
		return tx(l, xStatusBarTab)
	case tabFloating:
		return tx(l, xFloatingSection)
	case tabSkills:
		return tx(l, xSkillsSettingsTab)
	case tabSessions:
		return tx(l, xSessionsSettingsTab)
	case tabMCP:
		return tx(l, xMCPSettingsTab)
	}
	return l.Text([...]l10n.Key{l10n.KeyGeneral, l10n.KeyAccounts, l10n.KeyDisplay, "", "", l10n.KeyPanelModules, l10n.KeyNotificationSettings, "", "", "", l10n.KeyAdvanced, l10n.KeyAbout}[t])
}

func (t settingsTab) icon() *ui.SVG {
	return [...]*ui.SVG{icGear, icUsers, icEye, icGauge, icFloating, icLayout, icBell, icChat, icSkill, icPlug, icSliders, icInfo}[t]
}

func (t settingsTab) tint() Tint {
	return [...]Tint{TintGray, TintGreen, TintBlue, TintOrange, TintTeal, TintPurple, TintRed, TintTeal, TintPink, TintBlue, TintOrange, TintIndigo}[t]
}

//go:embed assets/app-icon.png
var appIconPNG []byte

var (
	appIconOnce sync.Once
	appIcon     *ui.Bitmap
)

func appIconBitmap() *ui.Bitmap {
	appIconOnce.Do(func() { appIcon, _ = ui.DecodeBitmap(appIconPNG) })
	return appIcon
}

const (
	websiteURL  = "https://quosparr.com"
	githubURL   = "https://github.com/Licoy/Quosparr"
	feedbackURL = "https://github.com/Licoy/Quosparr/issues/new"

	// loginItemsSettingsURL opens System Settings > General > Login Items
	// (macOS 13+), where a login item waiting for approval is allowed.
	loginItemsSettingsURL = "x-apple.systempreferences:com.apple.LoginItems-Settings.extension"
)

// settingsUI is the state of the settings window's view: which tab, the
// proxy form being edited, and the dialogs. Preferences live in Settings.
type settingsUI struct {
	s    *Settings
	opts SettingsOptions
	win  *mygo.Window // nil in tests

	tab      settingsTab
	accounts *accountForm
	sessions *settingsSessions
	skills   *settingsSkills
	mcp      *settingsMCP

	draft         ProxyDraft
	draftReady    bool
	testURL       string
	loadedCreds   ProxyCredentials
	proxyTask     Task
	proxyResult   ProxySaveResult
	testTask      Task
	selfCheckTask Task
	proxyMsg      string
	proxyMsgErr   bool
	proxyMsgGen   int
	toast         string

	pickingColor bool
	customColor  ui.Color
	floatingApps *floatingAppsDialog
	// fonts are the installed families, listed once a settings window
	// shows them, so fonts installed meanwhile show in the next window.
	fonts       []string
	fontsListed bool

	confirmRepair bool
	noticeTitle   string
	notice        string
	noticeOpen    bool
}

func newSettingsUI(s *Settings, opts SettingsOptions) *settingsUI {
	return &settingsUI{s: s, opts: opts}
}

// post runs fn on the main thread and draws a new frame.
func (u *settingsUI) post(fn func()) {
	if u.win != nil && !u.win.IsDestroyed() {
		u.win.Update(fn)
		return
	}
	runOnMain(fn)
}

func (u *settingsUI) L() l10n.Localizer { return u.s.L() }

func (u *settingsUI) text(k l10n.Key) string { return u.L().Text(k) }

// view builds one frame of the settings window.
func (u *settingsUI) view(c *ui.Context) {
	applyThemeColor(c, u.s.prefs.ThemeColor, u.s.prefs.FontFamily)
	if !u.draftReady {
		u.draft, u.draftReady = NewProxyDraft(u.s.Proxy()), true
		u.testURL = u.s.Prefs().ProxyTestTarget()
	}
	u.shell(c, u.pageContent)
	u.dialogs(c)
	if u.toast != "" {
		c.Toast(u.toast)
		u.toast = ""
	}
}

func (u *settingsUI) pageContent(c *ui.Context) {
	if split := splitPages[u.tab]; split != nil {
		// Managed lists scroll only their list, under fixed controls.
		ui.Column(c).Grow(1).Basis(0).Padding(4, 0, 0, 0).Gap(14).Children(func() {
			if u.s.ReadOnly() || u.s.SaveError() != nil {
				ui.Column(c).Padding(0, 24).Gap(14).Children(func() { u.pageNotices(c) })
			}
			split(u, c)
		})
		return
	}
	ui.Scroll(c.Key("page-"+fmt.Sprint(int(u.tab)))).Grow(1).Basis(0).Padding(4, 24, 24, 24).Gap(22).Children(func() {
		u.pageNotices(c)
		switch u.tab {
		case tabGeneral:
			u.generalPage(c)
		case tabAccounts:
			// The toolbar titles the box of accounts under it.
			ui.Column(c).Gap(10).Children(func() { u.accountsPage(c) })
		case tabDisplay:
			u.displayPage(c)
		case tabStatusBar:
			u.statusBarPage(c)
		case tabFloating:
			u.floatingPage(c)
		case tabModules:
			u.modulesGroup(c)
		case tabNotifications:
			u.notificationsGroup(c)
		case tabAdvanced:
			u.advancedPage(c)
		case tabAbout:
			u.aboutPage(c)
		}
	})
}

// splitPages are the tabs whose page lays out its own scrolling, under
// fixed controls.
var splitPages = map[settingsTab]func(*settingsUI, *ui.Context){
	tabSessions: (*settingsUI).sessionsPage,
	tabSkills:   (*settingsUI).skillsPage,
	tabMCP:      (*settingsUI).mcpPage,
}

// pageNotices tops a page with why settings cannot be changed or saved.
func (u *settingsUI) pageNotices(c *ui.Context) {
	if u.s.ReadOnly() {
		u.readOnlyBanner(c)
	}
	if err := u.s.SaveError(); err != nil {
		ui.Text(c, tx(u.L(), xSettingsSaveFailed, err.Error())).FontSize(12).TextColor(c.Theme().Danger)
	}
}

// readOnlyBanner explains why nothing can be changed.
func (u *settingsUI) readOnlyBanner(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(4).Padding(12).Radius(12).Background(t.Danger.Alpha(0.10)).Children(func() {
		ui.Text(c, tx(u.L(), xSettingsUnreadable)).FontSize(13).FontWeight(600).TextColor(t.Danger)
		ui.Text(c, u.s.LoadError().Error()).FontSize(11).TextColor(t.Danger)
		ui.Text(c, tx(u.L(), xSettingsReadOnly)).FontSize(11).TextColor(t.TextMuted)
	})
}

// dialogs draws the alerts that are open.
func (u *settingsUI) dialogs(c *ui.Context) {
	u.sessionDeleteDialog(c)
	u.mcpDeleteDialog(c)
	if u.confirmRepair {
		l := u.L()
		if ui.AlertDialog(c.Key("repair-confirm"), &u.confirmRepair,
			l.Text(l10n.KeyRepairConfirmTitle), l.Text(l10n.KeyRepairConfirmMessage),
			l.Text(l10n.KeyCancel), l.Text(l10n.KeyRepair)) == 1 && u.opts.Actions.RepairIndex != nil {
			u.opts.Actions.RepairIndex()
		}
	}
	if u.noticeOpen {
		ui.AlertDialog(c.Key("notice"), &u.noticeOpen, u.noticeTitle, u.notice, u.text(l10n.KeyOk))
	}
}

func (u *settingsUI) showNotice(title, message string) {
	u.noticeTitle, u.notice, u.noticeOpen = title, message, true
}

// ---- Labels of intervals ----

func (u *settingsUI) minutes(seconds int) string {
	return fmt.Sprintf("%d %s", seconds/60, u.text(l10n.KeyMinutes))
}

func (u *settingsUI) widgetInterval(seconds int) string {
	if seconds == 60 {
		return "60 " + u.text(l10n.KeySeconds)
	}
	return u.minutes(seconds)
}

func (u *settingsUI) resetTodayInterval(seconds int) string {
	if seconds < 3_600 {
		return u.minutes(seconds)
	}
	return fmt.Sprintf("%d %s", seconds/3_600, u.text(l10n.KeyHours))
}

// ---- General ----

func (u *settingsUI) generalPage(c *ui.Context) {
	s, l := u.s, u.L()
	p := s.Prefs()
	ro := s.ReadOnly()
	sys := s.System()
	settingsGroup(c, "", "", nil, func(row rowFunc) {
		row(l.Text(l10n.KeyLanguage), "", func() {
			langs := store.Languages()
			if v, ok := choose(c, "language", l.Text(l10n.KeyLanguage), p.Language, langs, func(v store.Language) string {
				return l10n.MenuTitle(string(v), l.Language())
			}, ro); ok {
				s.SetLanguage(v)
			}
		})
		detail := l.Text(l10n.KeyLaunchAtLoginDescription)
		switch {
		case s.LoginError() != nil:
			detail = l.Text(l10n.KeyLaunchAtLoginFailed)
		case !s.LoginAvailable():
			detail = l.Text(l10n.KeyLaunchAtLoginUnavailable)
		case s.LoginStatus() == LoginRequiresApproval:
			detail = l.Text(l10n.KeyLaunchAtLoginRequiresApproval)
		}
		row(l.Text(l10n.KeyLaunchAtLogin), detail, func() {
			if s.LoginPending() {
				ui.Spinner(c).Size(14, 14)
			}
			if s.LoginStatus() == LoginRequiresApproval && PillButton(c, nil, l.Text(l10n.KeyOpenLoginItemsSettings), false).Clicked() {
				c.OpenURL(loginItemsSettingsURL)
			}
			if v, ok := toggle(c, "login", l.Text(l10n.KeyLaunchAtLogin), s.LoginSwitch(), ro || !s.LoginAvailable() || s.LoginPending()); ok {
				s.ChangeLaunchAtLogin(v)
			}
		})
		row(l.Text(l10n.KeyRefreshInterval), "", func() {
			opts := []int{60, 300, 600, 900, 1_800}
			if v, ok := choose(c, "refresh", l.Text(l10n.KeyRefreshInterval), p.RefreshIntervalSeconds, opts, u.minutes, ro); ok {
				s.SetRefreshInterval(v)
			}
		})
		detail = l.Text(l10n.KeyWidgetRefreshIntervalDescription)
		if !sys.Widgets {
			detail = tx(l, xWidgetMac14)
		}
		row(l.Text(l10n.KeyWidgetRefreshInterval), detail, func() {
			if v, ok := choose(c, "widget-refresh", l.Text(l10n.KeyWidgetRefreshInterval), p.WidgetRefreshIntervalSeconds,
				store.WidgetRefreshIntervalOptions(), u.widgetInterval, ro || !sys.Widgets); ok {
				s.SetWidgetRefreshInterval(v)
			}
		})
	})
	u.proxyGroup(c)
}

// openFolder opens a folder in the file manager.
func openFolder(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		return
	}
	_ = mygo.Shell.OpenPath(path)
}

// ---- Network proxy ----

func (u *settingsUI) proxyGroup(c *ui.Context) {
	s, l := u.s, u.L()
	t := c.Theme()
	busy := u.proxyTask.Running || u.testTask.Running
	settingsGroup(c, l.Text(l10n.KeyNetworkProxy), "", nil, func(row rowFunc) {
		if err := s.ProxyError(); err != nil {
			row("", "", func() {
				ui.Column(c).Grow(1).Basis(0).Gap(3).Children(func() {
					ui.Text(c, l.Text(proxyErrorKey(err))).FontSize(12).TextColor(t.Danger)
					ui.Text(c, tx(l, xProxyBlocked)).FontSize(11).TextColor(t.TextMuted)
				})
			})
		}
		row(l.Text(l10n.KeyNetworkProxyMode), "", func() {
			modes := store.ProxyModes()
			if v, ok := pills(c, "px-mode", u.draft.Mode, modes, func(m store.ProxyMode) string {
				switch m {
				case store.ProxyModeHTTP:
					return l.Text(l10n.KeyNetworkProxyHTTP)
				case store.ProxyModeSOCKS5:
					return l.Text(l10n.KeyNetworkProxySOCKS5)
				}
				return l.Text(l10n.KeyNetworkProxySystem)
			}, busy); ok {
				u.draft.Mode = v
				u.setProxyMsg("", false)
			}
		})
		if u.draft.Mode != store.ProxyModeSystem {
			u.proxyFields(c, row, busy)
		}
		if s.proxyHooks.Test != nil {
			row(tx(l, xProxyTestURL), "", func() {
				ui.TextInput(c.Key("px-test-url"), &u.testURL).Width(230).Placeholder(store.DefaultProxyTestURL).
					Label(tx(l, xProxyTestURL)).Disabled(busy).OnSubmit(u.testProxy)
			})
		}
		u.proxyActions(c, busy)
	})
}

// proxyFields draws the manual proxy form, including optional authentication.
func (u *settingsUI) proxyFields(c *ui.Context, row rowFunc, busy bool) {
	l := u.L()
	row(l.Text(l10n.KeyNetworkProxyHost), "", func() {
		ui.TextInput(c.Key("px-host"), &u.draft.Host).Width(230).Placeholder("127.0.0.1").Label(l.Text(l10n.KeyNetworkProxyHost)).Disabled(busy)
	})
	row(l.Text(l10n.KeyNetworkProxyPort), "", func() {
		ui.TextInput(c.Key("px-port"), &u.draft.Port).Width(230).Placeholder("8080").Label(l.Text(l10n.KeyNetworkProxyPort)).Disabled(busy)
	})
	row(l.Text(l10n.KeyNetworkProxyAuthentication), "", func() {
		if v, ok := toggle(c, "px-auth", l.Text(l10n.KeyNetworkProxyAuthentication), u.draft.UsesAuth, busy); ok {
			u.draft.UsesAuth = v
		}
	})
	if !u.draft.UsesAuth {
		return
	}
	row(l.Text(l10n.KeyNetworkProxyUsername), "", func() {
		ui.TextInput(c.Key("px-user"), &u.draft.Username).Width(230).Label(l.Text(l10n.KeyNetworkProxyUsername)).Disabled(busy)
	})
	detail := ""
	if u.draft.CredentialID != "" {
		detail = l.Text(l10n.KeyNetworkProxySavedCredentials)
	}
	row(l.Text(l10n.KeyNetworkProxyPassword), detail, func() {
		ui.TextInput(c.Key("px-pass"), &u.draft.Password).Password().Width(230).Label(l.Text(l10n.KeyNetworkProxyPassword)).Disabled(busy)
	})
	if _, ok := u.s.creds.(UnavailableCredentials); ok {
		row("", tx(l, xProxyLocalSaveWarning), nil)
	}
	if u.draft.CredentialID != "" {
		row("", "", func() {
			if PillButton(c, nil, l.Text(l10n.KeyNetworkProxyLoadCredentials), busy).Clicked() {
				u.loadCredentials()
			}
		})
	}
}

// proxyActions is the final row inside the network card. The save button
// shows unsaved changes in the warning color and is disabled without them.
func (u *settingsUI) proxyActions(c *ui.Context, busy bool) {
	l := u.L()
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(2, 0, 8, 0).Children(func() {
		col := t.Success
		if u.proxyMsgErr {
			col = t.Danger
		}
		ui.Text(c, u.proxyMsg).FontSize(12).TextColor(col).Grow(1).Basis(0)
		if u.s.proxyHooks.Test != nil {
			label := l.Text(l10n.KeyNetworkProxyTest)
			if u.testTask.Running {
				label = l.Text(l10n.KeyNetworkProxyTesting)
			}
			if PillButton(c, nil, label, busy).Clicked() {
				u.testProxy()
			}
		}
		save := l.Text(l10n.KeyNetworkProxySave)
		if !busy && u.proxyUnsaved() {
			if WarningPillButton(c, save).Clicked() {
				u.saveProxy()
			}
		} else {
			PillButton(c, nil, save, true)
		}
	})
}

// proxyUnsaved reports whether saving the draft would change anything: it
// differs from the saved configuration and the credentials loaded into it,
// or the saved configuration is unusable.
func (u *settingsUI) proxyUnsaved() bool {
	if u.s.ProxyError() != nil {
		return true
	}
	base := NewProxyDraft(u.s.Proxy())
	base.Username, base.Password = u.loadedCreds.Username, u.loadedCreds.Password
	return proxyDraftKey(u.draft) != proxyDraftKey(base)
}

// proxyDraftKey drops the fields of a draft its mode ignores.
func proxyDraftKey(d ProxyDraft) ProxyDraft {
	if d.Mode == store.ProxyModeSystem {
		return ProxyDraft{Mode: d.Mode}
	}
	d.Host, d.Port = strings.TrimSpace(d.Host), strings.TrimSpace(d.Port)
	if !d.UsesAuth {
		d.Username, d.Password, d.CredentialID = "", "", ""
	}
	return d
}

// setProxyMsg replaces the message beside the network buttons.
func (u *settingsUI) setProxyMsg(msg string, isErr bool) {
	u.proxyMsg, u.proxyMsgErr = msg, isErr
	u.proxyMsgGen++
}

// flashProxyMsg shows a success message that clears itself after a while.
func (u *settingsUI) flashProxyMsg(msg string) {
	u.setProxyMsg(msg, false)
	gen := u.proxyMsgGen
	time.AfterFunc(proxyMsgTime, func() {
		u.post(func() {
			if u.proxyMsgGen == gen {
				u.setProxyMsg("", false)
			}
		})
	})
}

func (u *settingsUI) saveProxy() {
	d, prev := u.draft, u.s.Proxy()
	u.setProxyMsg("", false)
	runTask(&u.proxyTask, u.post, func() error {
		r, err := u.s.SaveProxy(d, prev)
		u.proxyResult = r
		return err
	}, func(err error) {
		l := u.L()
		if err != nil {
			u.setProxyMsg(l.Text(proxyErrorKey(err)), true)
			return
		}
		r := u.proxyResult
		u.s.ProxySaved(r)
		u.draft, u.loadedCreds = NewProxyDraft(r.Config), ProxyCredentials{}
		if r.CleanupFailed {
			u.setProxyMsg(l.Text(l10n.KeyNetworkProxyCleanupFailed), true)
			return
		}
		u.toast = l.Text(l10n.KeyNetworkProxySaved)
	})
}

func (u *settingsUI) loadCredentials() {
	id := u.draft.CredentialID
	var got ProxyCredentials
	runTask(&u.proxyTask, u.post, func() error {
		c, err := u.s.LoadProxyCredentials(id)
		got = c
		return err
	}, func(err error) {
		if err != nil {
			u.setProxyMsg(u.L().Text(proxyErrorKey(err)), true)
			return
		}
		u.draft.Username, u.draft.Password = got.Username, got.Password
		u.loadedCreds = got
		u.setProxyMsg("", false)
	})
}

func (u *settingsUI) testProxy() {
	if u.proxyTask.Running || u.testTask.Running {
		return
	}
	site, err := store.ParseProxyTestURL(u.testURL)
	if err != nil {
		u.setProxyMsg(tx(u.L(), xProxyTestURLInvalid), true)
		return
	}
	u.testURL = site
	u.s.SetProxyTestURL(site)
	d := u.draft
	u.setProxyMsg("", false)
	runTask(&u.testTask, u.post, func() error {
		cfg, err := d.Configuration()
		if err != nil {
			return err
		}
		creds, err := credentialsFor(cfg, d.entered(), u.s.creds)
		if err != nil {
			return err
		}
		return u.s.proxyHooks.Test(context.Background(), cfg, creds, site)
	}, func(err error) {
		if err != nil {
			u.setProxyMsg(u.L().Text(proxyErrorKey(err)), true)
			return
		}
		u.flashProxyMsg(u.L().Text(l10n.KeyNetworkProxyTestSucceeded))
	})
}

// ---- Accounts ----

func (u *settingsUI) accountsPage(c *ui.Context) {
	renderAccountsPage(c, u)
}

// ---- Advanced ----

func (u *settingsUI) advancedPage(c *ui.Context) {
	s, l := u.s, u.L()
	p := s.Prefs()
	sys := s.System()
	a := u.opts.Actions
	settingsGroup(c, "", "", nil, func(row rowFunc) {
		detail := tx(l, xDisablePanelAnimationHint)
		if !sys.PanelAnimation {
			detail = tx(l, xMacOnly)
		}
		row(tx(l, xDisablePanelAnimation), detail, func() {
			if v, ok := toggle(c, "disable-panel-animation", tx(l, xDisablePanelAnimation), p.DisableMainPanelAnimation, s.ReadOnly() || !sys.PanelAnimation); ok {
				s.SetDisableMainPanelAnimation(v)
			}
		})
		row(l.Text(l10n.KeyRefresh), l.Text(l10n.KeyQuota), func() {
			if PillButton(c, nil, l.Text(l10n.KeyRefresh), a.Refresh == nil).Clicked() {
				a.Refresh()
			}
		})
		row(l.Text(l10n.KeySelfCheck), l.Text(l10n.KeySessionRepair), func() {
			if u.selfCheckTask.Running {
				ui.Spinner(c).Size(14, 14)
			}
			if PillButton(c, nil, l.Text(l10n.KeySelfCheck), a.SelfCheck == nil || u.selfCheckTask.Running).Clicked() {
				var report string
				runTask(&u.selfCheckTask, u.post, func() error {
					report = a.SelfCheck()
					return nil
				}, func(error) { u.showNotice(u.text(l10n.KeySelfCheck), report) })
			}
		})
		row(l.Text(l10n.KeyExportStatusJSON), "~/.quosparr/status.json", func() {
			if v, ok := toggle(c, "export-json", l.Text(l10n.KeyExportStatusJSON), p.ExportsStatusJSON, s.ReadOnly()); ok {
				s.SetExportsStatusJSON(v)
			}
		})
		row(l.Text(l10n.KeyRepairIndex), l.Text(l10n.KeyBackup), func() {
			if PillButton(c, nil, l.Text(l10n.KeyRepair), a.RepairIndex == nil).Clicked() {
				u.confirmRepair = true
			}
		})
	})
}

// ---- About ----

func (u *settingsUI) aboutPage(c *ui.Context) {
	s, l := u.s, u.L()
	p := s.Prefs()
	t := c.Theme()
	a := u.opts.Actions
	Panel(c).Padding(20, 12).Gap(4).AlignItems(ui.Center).Children(func() {
		if bm := appIconBitmap(); bm != nil {
			ui.Image(c, bm).Size(72, 72).Label("Quosparr")
		}
		ui.Text(c, "Quosparr").FontSize(20).FontWeight(700).SingleLine()
		ui.Text(c, l.Text(l10n.KeyVersion)+" "+valueOr(u.opts.Version, "dev")).FontSize(12).TextColor(t.TextMuted).SingleLine()
	})
	settingsGroup(c, "", "", nil, func(row rowFunc) {
		row(l.Text(l10n.KeyAutomaticallyCheckForUpdates), "", func() {
			if v, ok := toggle(c, "auto-update", l.Text(l10n.KeyAutomaticallyCheckForUpdates), p.AutomaticallyChecksForUpdates, s.ReadOnly()); ok {
				s.SetAutomaticUpdateChecks(v)
			}
		})
		row(l.Text(l10n.KeyCheckForUpdates), "", func() {
			if PillButton(c, nil, l.Text(l10n.KeyCheckForUpdates), a.CheckForUpdates == nil).Clicked() {
				a.CheckForUpdates()
			}
		})
	})
	settingsGroup(c, "", "", nil, func(row rowFunc) {
		row(l.Text(l10n.KeyWebsite), "quosparr.com", func() {
			if PillButton(c, nil, l.Text(l10n.KeyOpenWebsite), false).Clicked() {
				c.OpenURL(websiteURL)
			}
		})
		row("GitHub", "github.com/Licoy/Quosparr", func() {
			if PillButton(c, nil, l.Text(l10n.KeyOpenGithub), false).Clicked() {
				c.OpenURL(githubURL)
			}
		})
		row(l.Text(l10n.KeyFeedbackIssue), "GitHub Issues", func() {
			if PillButton(c, nil, l.Text(l10n.KeyFeedbackIssue), false).Clicked() {
				c.OpenURL(feedbackURL)
			}
		})
	})
}
