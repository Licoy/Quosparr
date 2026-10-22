package app

import (
	"maps"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// floatingAppIconPixels is the size of the icons the list asks for, enough
// for 20 DIPs on a 3x display.
const floatingAppIconPixels = 64

// floatingAppsDialog chooses the apps the floating window follows: the
// installed ones, and programs picked from a file.
type floatingAppsDialog struct {
	query  string
	picked []store.FloatingApp
	// listed are the apps offered, in a steady order: those chosen when
	// it opened or picked from a file, then the installed ones.
	listed  []store.FloatingApp
	loading bool
	failed  bool
	// icons are by the app's path; a nil one could not be drawn.
	icons map[string]*ui.Bitmap
	busy  bool
	note  string
	done  chan struct{}
}

// floatingAppRows choose how the app in front shows the window, and the apps.
func (u *settingsUI) floatingAppRows(c *ui.Context, row rowFunc, f store.FloatingWindow, ro bool) {
	l := u.L()
	supported := foregroundSupported()
	hint := floatingAppFilterHint(l, f.AppFilter)
	if !supported {
		hint = tx(l, xFloatingAppFilterUnsupported)
	}
	row(tx(l, xFloatingAppFilter), hint, func() {
		if v, ok := choose(c, "fw-app-filter", tx(l, xFloatingAppFilter), f.AppFilter, store.FloatingAppFilters(),
			func(v store.FloatingAppFilter) string { return floatingAppFilterName(l, v) }, ro || !supported); ok {
			u.s.SetFloatingWindow(func(f *store.FloatingWindow) { f.AppFilter = v })
		}
	})
	if f.AppFilter == store.FloatingAppFilterOff || !supported {
		return
	}
	row(tx(l, xFloatingApps), floatingAppsSummary(l, f.Apps), func() {
		if PillButton(c, nil, tx(l, xFloatingChooseApps), ro).Clicked() {
			u.openFloatingApps(f.Apps)
		}
	})
	if f.AppFilter == store.FloatingAppFilterAllow {
		row(tx(l, xFloatingOffList), floatingOffListHint(l, f.HideOffList), func() {
			if v, ok := choose(c, "fw-off-list", tx(l, xFloatingOffList), f.HideOffList, []bool{false, true},
				func(hide bool) string { return floatingOffListName(l, hide) }, ro); ok {
				u.s.SetFloatingWindow(func(f *store.FloatingWindow) { f.HideOffList = v })
			}
		})
	}
	u.floatingAppsEditor(c, f.AppFilter)
}

func floatingOffListName(l l10n.Localizer, hide bool) string {
	if hide {
		return tx(l, xFloatingOffListHide)
	}
	return tx(l, xFloatingOffListLower)
}

func floatingOffListHint(l l10n.Localizer, hide bool) string {
	if hide {
		return tx(l, xFloatingOffListHideHint)
	}
	return tx(l, xFloatingOffListLowerHint)
}

func floatingAppFilterName(l l10n.Localizer, v store.FloatingAppFilter) string {
	switch v {
	case store.FloatingAppFilterAllow:
		return tx(l, xFloatingAppFilterAllow)
	case store.FloatingAppFilterBlock:
		return tx(l, xFloatingAppFilterBlock)
	}
	return tx(l, xFloatingAppFilterOff)
}

func floatingAppFilterHint(l l10n.Localizer, v store.FloatingAppFilter) string {
	switch v {
	case store.FloatingAppFilterAllow:
		return tx(l, xFloatingAppFilterAllowHint)
	case store.FloatingAppFilterBlock:
		return tx(l, xFloatingAppFilterBlockHint)
	}
	return tx(l, xFloatingAppFilterHint)
}

// floatingAppsSummary names the chosen apps, or counts them past two.
func floatingAppsSummary(l l10n.Localizer, apps []store.FloatingApp) string {
	switch len(apps) {
	case 0:
		return tx(l, xFloatingAppsNone)
	case 1, 2:
		names := make([]string, len(apps))
		for i, a := range apps {
			names[i] = floatingAppName(a)
		}
		return strings.Join(names, ", ")
	}
	return tx(l, xFloatingAppsCount, strconv.Itoa(len(apps)))
}

func floatingAppName(a store.FloatingApp) string {
	if a.Name != "" {
		return a.Name
	}
	return a.ID
}

// openFloatingApps opens the dialog with the apps chosen, and lists the
// installed ones and their icons in the background.
func (u *settingsUI) openFloatingApps(apps []store.FloatingApp) {
	d := &floatingAppsDialog{picked: slices.Clone(apps), listed: slices.Clone(apps), loading: true, icons: map[string]*ui.Bitmap{}, done: make(chan struct{})}
	u.floatingApps = d
	go func() {
		found, err := mygo.Shell.InstalledApps()
		installed := make([]store.FloatingApp, 0, len(found))
		for _, a := range found {
			installed = append(installed, store.FloatingApp{ID: a.ID, Name: a.Name, Path: a.Path})
		}
		u.post(func() {
			d.loading, d.failed = false, err != nil
			d.list(false, installed...)
		})
		u.loadFloatingAppIcons(d, append(slices.Clone(apps), installed...))
	}()
}

// loadFloatingAppIcons draws the icons one by one, showing them in batches,
// until the dialog closes.
func (u *settingsUI) loadFloatingAppIcons(d *floatingAppsDialog, apps []store.FloatingApp) {
	const batch = 24
	pending := map[string]*ui.Bitmap{}
	flush := func() {
		got := pending
		pending = map[string]*ui.Bitmap{}
		u.post(func() { maps.Copy(d.icons, got) })
	}
	seen := map[string]bool{}
	for _, a := range apps {
		select {
		case <-d.done:
			return
		default:
		}
		if a.Path == "" || seen[a.Path] {
			continue
		}
		seen[a.Path] = true
		var bm *ui.Bitmap
		if png, err := mygo.Shell.FileIcon(a.Path, floatingAppIconPixels); err == nil {
			bm, _ = ui.DecodeBitmap(png)
		}
		pending[a.Path] = bm
		if len(pending) == batch {
			flush()
		}
	}
	flush()
}

func (u *settingsUI) closeFloatingApps() {
	if d := u.floatingApps; d != nil {
		close(d.done)
		u.floatingApps = nil
	}
}

// floatingAppsEditor is the dialog: the chosen apps first, then the other
// installed ones, both narrowed by the search.
func (u *settingsUI) floatingAppsEditor(c *ui.Context, filter store.FloatingAppFilter) {
	d := u.floatingApps
	if d == nil {
		return
	}
	l, t, open := u.L(), c.Theme(), true
	ui.Modal(c.Key("floating-apps"), &open, func() {
		ui.Text(c, tx(l, xFloatingAppsTitle)).FontSize(17).FontWeight(600)
		ui.Text(c, floatingAppFilterHint(l, filter)).FontSize(11).TextColor(t.TextMuted)
		ui.TextInput(c.Key("fw-apps-search"), &d.query).Label(tx(l, xFloatingAppsSearch)).Placeholder(tx(l, xFloatingAppsSearch))
		rows := d.rows()
		ui.Scroll(c.Key("fw-apps-rows")).Height(320).Gap(2).Children(func() {
			for _, a := range rows {
				u.floatingAppRow(c, d, a)
			}
			switch {
			case d.loading:
				ui.Row(c).Gap(8).Padding(8, 4).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(14, 14)
					ui.Text(c, tx(l, xPanelLoading)).FontSize(12).TextColor(t.TextMuted)
				})
			case d.failed:
				ui.Text(c, tx(l, xFloatingAppsLoadFailed)).FontSize(12).TextColor(t.Danger)
			case len(rows) == 0:
				ui.Text(c, tx(l, xFloatingAppsEmpty)).FontSize(12).TextColor(t.TextMuted)
			}
		})
		if d.note != "" {
			ui.Text(c, d.note).FontSize(12).TextColor(t.Danger)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if PillButton(c, nil, tx(l, xFloatingAppsAddFile), d.busy).Clicked() {
				u.addFloatingAppFiles(d)
			}
			ui.Spacer(c).Grow(1)
			if PillButton(c, nil, l.Text(l10n.KeyCancel), false).Clicked() {
				u.closeFloatingApps()
			}
			if PillButton(c, nil, l.Text(l10n.KeyOk), d.busy).Clicked() {
				picked := d.picked
				u.s.SetFloatingWindow(func(f *store.FloatingWindow) { f.Apps = picked })
				u.closeFloatingApps()
			}
		})
	}).Width(480)
	if !open && u.floatingApps == d {
		u.closeFloatingApps()
	}
}

// list offers apps not offered yet, first or last.
func (d *floatingAppsDialog) list(first bool, apps ...store.FloatingApp) {
	var add []store.FloatingApp
	for _, a := range apps {
		listed := func(l store.FloatingApp) bool { return l.ID == a.ID }
		if !slices.ContainsFunc(d.listed, listed) && !slices.ContainsFunc(add, listed) {
			add = append(add, a)
		}
	}
	if first {
		d.listed = append(add, d.listed...)
	} else {
		d.listed = append(d.listed, add...)
	}
}

// rows are the apps offered that match the search.
func (d *floatingAppsDialog) rows() []store.FloatingApp {
	q := strings.ToLower(strings.TrimSpace(d.query))
	if q == "" {
		return d.listed
	}
	var rows []store.FloatingApp
	for _, a := range d.listed {
		if strings.Contains(strings.ToLower(floatingAppName(a)), q) || strings.Contains(strings.ToLower(a.ID), q) {
			rows = append(rows, a)
		}
	}
	return rows
}

// toggle chooses an app or drops it.
func (d *floatingAppsDialog) toggle(a store.FloatingApp, on bool) {
	i := slices.IndexFunc(d.picked, func(p store.FloatingApp) bool { return p.ID == a.ID })
	switch {
	case on && i < 0 && len(d.picked) < store.MaxFloatingApps:
		d.picked = append(d.picked, a)
	case !on && i >= 0:
		d.picked = slices.Delete(d.picked, i, i+1)
	}
}

func (u *settingsUI) floatingAppRow(c *ui.Context, d *floatingAppsDialog, a store.FloatingApp) {
	on := slices.ContainsFunc(d.picked, func(p store.FloatingApp) bool { return p.ID == a.ID })
	ui.Row(c).Gap(8).Padding(3, 4).AlignItems(ui.Center).Tooltip(a.ID).Children(func() {
		if bm := d.icons[a.Path]; bm != nil {
			ui.Image(c, bm).Size(20, 20).Shrink(0)
		} else {
			ui.Box(c).Size(20, 20).Shrink(0)
		}
		was := on
		box := ui.Checkbox(c.Key("fw-app-"+a.ID), &on, floatingAppName(a)).Grow(1).MinWidth(0)
		if box.Changed() && on != was {
			d.toggle(a, on)
		}
	})
}

// addFloatingAppFiles adds the apps picked from files: .app bundles on
// macOS, programs on Windows, desktop entries or programs on Linux.
func (u *settingsUI) addFloatingAppFiles(d *floatingAppsDialog) {
	d.busy, d.note = true, ""
	l := u.L()
	opts := mygo.OpenDialogOptions{Title: tx(l, xFloatingAppsAddFile), Multiple: true, DefaultPath: floatingAppsFolder()}
	switch runtime.GOOS {
	case "darwin":
		opts.Filters = []mygo.FileFilter{{Name: ".app", Extensions: []string{"app"}}}
	case "windows":
		opts.Filters = []mygo.FileFilter{{Name: ".exe", Extensions: []string{"exe"}}}
	}
	go func() {
		paths, _ := mygo.Dialog.Open(opts)
		var added []store.FloatingApp
		failed := false
		for _, p := range paths {
			if a, ok := floatingAppFromFile(p); ok {
				added = append(added, a)
			} else {
				failed = true
			}
		}
		u.post(func() {
			d.busy = false
			d.list(true, added...)
			for _, a := range added {
				d.toggle(a, true)
			}
			if failed {
				d.note = tx(l, xFloatingAppsNotApp)
			}
		})
		u.loadFloatingAppIcons(d, added)
	}()
}

// floatingAppsFolder is where the file dialog starts.
func floatingAppsFolder() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Applications"
	case "windows":
		return os.Getenv("ProgramFiles")
	}
	return "/usr/share/applications"
}
