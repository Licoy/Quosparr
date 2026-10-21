package app

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// ---- Appearance & menu ----

func (u *settingsUI) displayPage(c *ui.Context) {
	u.appearanceGroup(c)
	u.privacyGroup(c)
	u.cursorGroup(c)
}

func (u *settingsUI) privacyGroup(c *ui.Context) {
	s, l := u.s, u.L()
	settingsGroup(c, tx(l, xPrivacyDisplaySection), "", nil, func(row rowFunc) {
		row(l.Text(l10n.KeyAccountPrivacyMode), tx(l, xPrivacyDisplayDescription), func() {
			if v, ok := toggle(c, "account-privacy", l.Text(l10n.KeyAccountPrivacyMode), s.Prefs().AccountPrivacyModeEnabled, s.ReadOnly()); ok {
				s.SetAccountPrivacyMode(v)
			}
		})
	})
}

func (u *settingsUI) appearanceGroup(c *ui.Context) {
	s, l := u.s, u.L()
	p := s.Prefs()
	ro := s.ReadOnly()
	sys := s.System()
	macNote := func(supported bool) string {
		if supported {
			return ""
		}
		return tx(l, xMacOnly)
	}
	settingsGroup(c, l.Text(l10n.KeyDisplayAppearanceSection), "", nil, func(row rowFunc) {
		u.panelProviderRows(c, row, p, ro)
		if providers := p.PanelSelection(); len(providers) > 1 {
			row(tx(l, xProvider), tx(l, xProviderDescription), func() {
				selected := max(0, slices.Index(providers, p.SelectedProvider))
				if providerTabsOf(c.Key("provider"), providers, &selected, ro, providerLabels(l, providers)...) {
					s.SetSelectedProvider(providers[selected])
				}
			})
		}
		row(l.Text(l10n.KeyAppearance), "", func() {
			if v, ok := pills(c, "appearance", p.Appearance, store.Appearances(),
				func(v store.Appearance) string { return l.Text(appearanceKey(v)) }, ro); ok {
				s.SetAppearance(v)
			}
		})
		row(tx(l, xThemeColor), u.themeColorName(p.ThemeColor), func() { u.themeColorControl(c, ro) })
		row(tx(l, xDisplayFont), tx(l, xDisplayFontHint), func() { u.fontControl(c, p.FontFamily, ro) })
		detail := l.Text(l10n.KeyMainPanelBackgroundDescription)
		if !sys.PanelBackground {
			detail = macNote(false)
		}
		row(l.Text(l10n.KeyMainPanelBackground), detail, func() {
			if v, ok := pills(c, "panel-bg", p.MainPanelBackgroundStyle, store.PanelBackgrounds(),
				func(v store.PanelBackground) string { return l.Text(panelBackgroundKey(v)) }, ro || !sys.PanelBackground); ok {
				s.SetPanelBackground(v)
			}
		})
		row(tx(l, xPanelHeight), tx(l, xPanelHeightHint), func() {
			h := p.MainPanelHeight
			var display mygo.Display
			if u.opts.PanelDisplay != nil {
				display = u.opts.PanelDisplay()
			}
			limit := panelHeightLimit(display.Bounds.Height, display.WorkArea.Height)
			if h == store.DefaultMainPanelHeight {
				h = clampPanelHeight(h, display.Bounds.Height, display.WorkArea.Height)
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, strconv.Itoa(int(h+0.5))).FontSize(13).FontFeatures("tnum").SingleLine()
				if ui.Stepper(c.Key("panel-height"), &h, store.MinMainPanelHeight, limit, 1).
					Label(tx(l, xPanelHeight)).Disabled(ro).Changed() {
					s.SetMainPanelHeight(h)
				}
			})
		})
		u.statusBarRows(c, row, p, ro, sys.StatusBarStyles, macNote(sys.StatusBarStyles))
	})
}

// fontControl chooses the display font: the system's, listed first as
// Default, or an installed family, which typing filters.
func (u *settingsUI) fontControl(c *ui.Context, cur string, ro bool) {
	l := u.L()
	if !u.fontsListed {
		u.fontsListed = true
		if f := u.s.System().FontFamilies; f != nil {
			u.fonts = f()
		}
	}
	def := tx(l, xDisplayFontDefault)
	options := append([]string{def}, u.fonts...)
	sel := cur
	if sel == "" {
		sel = def
	}
	if ui.Combobox(c.Key("font-family"), &sel, options).Label(tx(l, xDisplayFont)).Width(230).Disabled(ro).Changed() {
		if sel == def {
			sel = ""
		}
		u.s.SetFontFamily(sel)
	}
}

// panelProviderRows choose the providers of the main panel: all of them, or
// a custom set of at least one. Turning custom on keeps every provider
// checked, so the panel does not change until one is unchecked.
func (u *settingsUI) panelProviderRows(c *ui.Context, row rowFunc, p store.Preferences, ro bool) {
	s, l := u.s, u.L()
	custom := len(p.PanelProviders) > 0
	row(tx(l, xPanelProviders), tx(l, xPanelProvidersHint), func() {
		if v, ok := choose(c, "panel-scope", tx(l, xPanelProviders), custom, []bool{false, true},
			func(v bool) string {
				if v {
					return l.Text(l10n.KeyStatusBarProviderScopeCustom)
				}
				return tx(l, xPanelProvidersAll)
			}, ro); ok {
			var next []store.Provider
			if v {
				next = store.Providers()
			}
			s.SetPanelProviders(next)
		}
	})
	if !custom {
		return
	}
	row(l.Text(l10n.KeyStatusBarProviders), "", func() {
		selected := map[store.Provider]bool{}
		for _, item := range p.PanelProviders {
			selected[item] = true
		}
		ui.Row(c).Gap(12).Wrap().Children(func() {
			for _, prov := range store.Providers() {
				on := selected[prov]
				// Dropping the last one is not offered.
				box := ui.Checkbox(c.Key("panel-prov-"+string(prov)), &on, l.Text(providerKey(prov))).
					Disabled(ro || (on && len(selected) <= 1))
				if !box.Changed() || ro {
					continue
				}
				selected[prov] = on
				next := make([]store.Provider, 0, len(selected))
				for _, item := range store.Providers() {
					if selected[item] {
						next = append(next, item)
					}
				}
				if len(next) > 0 {
					s.SetPanelProviders(next)
				}
			}
		})
	})
}

// statusBarRows are the settings of the status item. Where the OS has no
// such status item they stay, disabled and explained, with their values.
func (u *settingsUI) statusBarRows(c *ui.Context, row rowFunc, p store.Preferences, ro, supported bool, note string) {
	s, l := u.s, u.L()
	off := ro || !supported
	row(l.Text(l10n.KeyStatusBarStyle), note, func() {
		if v, ok := choose(c, "sb-style", l.Text(l10n.KeyStatusBarStyle), p.StatusBarDisplayStyle, store.StatusBarStyles(),
			func(v store.StatusBarStyle) string { return l.Text(statusBarStyleKey(v)) }, off); ok {
			s.SetStatusBarStyle(v)
		}
	})
	row(l.Text(l10n.KeyStatusBarProviderScope), note, func() {
		cur := p.StatusBarProviderScope
		if cur != store.ProviderScopeSelected {
			cur = store.ProviderScopeCustom
		}
		scopes := []store.StatusBarProviderScope{store.ProviderScopeSelected, store.ProviderScopeCustom}
		if v, ok := choose(c, "sb-scope", l.Text(l10n.KeyStatusBarProviderScope), cur, scopes,
			func(v store.StatusBarProviderScope) string { return l.Text(providerScopeKey(v)) }, off); ok {
			s.SetStatusBarProviderScope(v)
		}
	})
	if p.StatusBarProviderScope != store.ProviderScopeSelected {
		hint := note
		if hint == "" {
			hint = l.Text(l10n.KeyStatusBarProvidersHint)
		}
		row(l.Text(l10n.KeyStatusBarProviders), hint, func() {
			statusBarProviderChecks(c, l, s, p, off)
		})
	}
	if p.StatusBarDisplayStyle == store.StatusBarStyleMeters {
		row(l.Text(l10n.KeyStatusBarMetersDetailStyle), note, func() {
			if v, ok := choose(c, "sb-meters", l.Text(l10n.KeyStatusBarMetersDetailStyle), p.StatusBarMetersDetailStyle, store.MetersDetails(),
				func(v store.MetersDetail) string { return l.Text(metersDetailKey(v)) }, off); ok {
				s.SetStatusBarMetersDetail(v)
			}
		})
	}
	if p.StatusBarDisplayStyle == store.StatusBarStyleBattery {
		row(l.Text(l10n.KeyStatusBarBatteryScope), note, func() {
			if v, ok := choose(c, "sb-bscope", l.Text(l10n.KeyStatusBarBatteryScope), p.StatusBarBatteryScope, store.BatteryScopes(),
				func(v store.BatteryScope) string { return l.Text(batteryScopeKey(v)) }, off); ok {
				s.SetStatusBarBatteryScope(v)
			}
		})
		row(l.Text(l10n.KeyStatusBarBatteryDetailStyle), note, func() {
			if v, ok := choose(c, "sb-bdetail", l.Text(l10n.KeyStatusBarBatteryDetailStyle), p.StatusBarBatteryDetailStyle, store.BatteryDetails(),
				func(v store.BatteryDetail) string { return l.Text(batteryDetailKey(v)) }, off); ok {
				s.SetStatusBarBatteryDetail(v)
			}
		})
	}
}

func statusBarProviderChecks(c *ui.Context, l l10n.Localizer, s *Settings, p store.Preferences, off bool) {
	selected := map[store.Provider]bool{}
	for _, item := range p.StatusBarSelection() {
		selected[item] = true
	}
	if p.StatusBarProviderScope == store.ProviderScopeBoth {
		selected[store.ProviderCodex] = true
		selected[store.ProviderGrok] = true
	}
	n := 0
	for _, on := range selected {
		if on {
			n++
		}
	}
	ui.Row(c).Gap(12).Wrap().Children(func() {
		for _, prov := range store.Providers() {
			on := selected[prov]
			// A third pick and dropping the last one are not offered.
			box := ui.Checkbox(c.Key("sb-prov-"+string(prov)), &on, l.Text(providerKey(prov))).
				Disabled(off || (!on && n >= 2) || (on && n <= 1))
			if !box.Changed() || off {
				continue
			}
			next := make([]store.Provider, 0, 2)
			for _, item := range store.Providers() {
				want := selected[item]
				if item == prov {
					want = on
				}
				if want {
					next = append(next, item)
				}
			}
			if len(next) >= 1 && len(next) <= 2 {
				s.SetStatusBarProviders(next)
			}
		}
	})
}

// ---- Panel modules ----

// modulesGroup is the panel modules page. The page header names it, so the
// section is untitled.
func (u *settingsUI) modulesGroup(c *ui.Context) {
	s, l := u.s, u.L()
	p := s.Prefs()
	ro := s.ReadOnly()
	atDefault := slices.Equal(p.MainPanelModuleOrder, store.Modules())
	settingsSection(c, "", l.Text(l10n.KeyMainPanelModulesDescription), func() {
		if IconButton(c, icUndo, l.Text(l10n.KeyRestoreDefaultOrder), ro || atDefault).Clicked() {
			s.ResetMainPanelModuleOrder()
		}
	}, func() {
		for i, m := range p.MainPanelModuleOrder {
			u.moduleCard(c, p, m, i, len(p.MainPanelModuleOrder))
		}
	})
}

func (u *settingsUI) moduleCard(c *ui.Context, p store.Preferences, m store.Module, index, count int) {
	s, l := u.s, u.L()
	t := c.Theme()
	ro := s.ReadOnly()
	info := moduleInfoOf(m)
	title, subtitle := moduleTexts(l, m)
	shown := ModuleShown(p, m)
	key := string(m)
	applies := l.Text(l10n.KeyModuleAppliesCodex)
	if info.both {
		applies = l.Text(l10n.KeyModuleAppliesBoth)
	}
	ui.Column(c).Children(func() {
		if index > 0 {
			ui.Box(c).Height(1).Background(pal(t).Line)
		}
		ui.Column(c).Gap(10).Padding(10, 0).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
				ui.Row(c).Size(30, 26).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					ui.Icon(c, info.icon).Size(16, 16).TextColor(t.TextMuted)
				})
				ui.Column(c).Grow(1).Basis(0).Gap(3).Children(func() {
					col := t.Text
					if !shown {
						col = t.TextMuted
					}
					ui.Row(c).Height(26).AlignItems(ui.Center).Children(func() {
						ui.Text(c, title).FontSize(13).TextColor(col).Grow(1).Basis(0).SingleLine().Tooltip(title)
					})
					ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted)
					ui.Text(c, applies).FontSize(10).TextColor(t.TextMuted).SingleLine()
				})
				ui.Row(c).Width(110).Shrink(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
					if IconButton(c.Key("up-"+key), icChevUp, l.Text(l10n.KeyAccountsMoveUp), ro || index == 0).Clicked() {
						s.MoveMainPanelModule(m, -1)
					}
					if IconButton(c.Key("down-"+key), icChevDown, l.Text(l10n.KeyAccountsMoveDown), ro || index+1 >= count).Clicked() {
						s.MoveMainPanelModule(m, 1)
					}
					name := l.Text(l10n.KeyModuleHidden)
					if shown {
						name = l.Text(l10n.KeyModuleVisible)
					}
					if v, ok := toggle(c, "show-"+key, name, shown, ro); ok {
						s.SetModuleShown(m, v)
					}
				})
			})
			if info.config && (shown || info.configWhenHidden) {
				ui.Column(c).Gap(12).Padding(0, 0, 0, 40).Children(func() { u.moduleConfig(c, m, p) })
			}
		})
	})
}

// moduleConfig is the setting a module has of its own.
func (u *settingsUI) moduleConfig(c *ui.Context, m store.Module, p store.Preferences) {
	s, l := u.s, u.L()
	t := c.Theme()
	ro := s.ReadOnly()
	line := func(title, detail string, control func()) {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Basis(0).Gap(1).Children(func() {
				ui.Text(c, title).FontSize(12)
				if detail != "" {
					ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted)
				}
			})
			control()
		})
	}
	switch m {
	case store.ModuleQuota:
		line(l.Text(l10n.KeyShowModelSpecificQuotaUsage), l.Text(l10n.KeyModelSpecificQuotaUsageDescription), func() {
			if v, ok := toggle(c, "model-quota", l.Text(l10n.KeyShowModelSpecificQuotaUsage), p.ShowsModelSpecificQuotaUsage, ro); ok {
				s.SetShowsModelSpecificQuotaUsage(v)
			}
		})
		line(tx(l, xShowCreditsBalance), tx(l, xShowCreditsBalanceHint), func() {
			if v, ok := toggle(c, "credits-balance", tx(l, xShowCreditsBalance), p.ShowsCreditsBalance, ro); ok {
				s.SetShowsCreditsBalance(v)
			}
		})
		line(tx(l, xResetTimeFormat), tx(l, xResetTimeFormatHint), func() {
			if v, ok := choose(c, "reset-time-format", tx(l, xResetTimeFormat), p.QuotaResetTimeFormat, store.ResetTimeFormats(),
				func(v store.ResetTimeFormat) string { return resetTimeFormatName(l, v) }, ro); ok {
				s.SetQuotaResetTimeFormat(v)
			}
		})
	case store.ModuleTokenUsage:
		line(l.Text(l10n.KeyTokenUsageChartStyle), "", func() {
			if v, ok := choose(c, "chart-style", l.Text(l10n.KeyTokenUsageChartStyle), p.TokenUsageChartStyle, store.ChartStyles(),
				func(v store.ChartStyle) string { return l.Text(chartStyleKey(v)) }, ro); ok {
				s.SetTokenUsageChartStyle(v)
			}
		})
	case store.ModuleRateLimitResetToday:
		line(l.Text(l10n.KeyRateLimitResetTodayRefreshInterval), "", func() {
			if v, ok := choose(c, "reset-today-interval", l.Text(l10n.KeyRateLimitResetTodayRefreshInterval),
				p.RateLimitResetTodayRefreshIntervalSeconds, store.RateLimitResetTodayRefreshIntervalOptions(), u.resetTodayInterval, ro); ok {
				s.SetRateLimitResetTodayRefreshInterval(v)
			}
		})
	case store.ModuleAPICost:
		line(l.Text(l10n.KeyApiCostSummaryRange), "", func() {
			if v, ok := choose(c, "cost-range", l.Text(l10n.KeyApiCostSummaryRange), p.APICostSummaryRange, store.CostRanges(),
				func(v store.CostRange) string { return l.Text(costRangeKey(v)) }, ro); ok {
				s.SetAPICostRange(v)
			}
		})
	case store.ModuleQuotaEstimate:
		line(l.Text(l10n.KeyQuotaEstimateWindowMode), "", func() {
			if v, ok := choose(c, "estimate-window", l.Text(l10n.KeyQuotaEstimateWindowMode), p.QuotaEstimateWindowMode, store.WindowModes(),
				func(v store.WindowMode) string { return l.Text(windowModeKey(v)) }, ro); ok {
				s.SetQuotaEstimateWindowMode(v)
			}
		})
	}
}

// ---- Notifications ----

// notificationsGroup is the notifications page; the page header names it.
func (u *settingsUI) notificationsGroup(c *ui.Context) {
	s, l := u.s, u.L()
	p := s.Prefs()
	ro := s.ReadOnly()
	settingsGroup(c, "", "", nil, func(row rowFunc) {
		row(l.Text(l10n.KeyQuotaAlerts), l.Text(l10n.KeyQuotaAlertsDescription), func() {
			if v, ok := toggle(c, "alert-quota", l.Text(l10n.KeyQuotaAlerts), p.QuotaAlertsEnabled, ro); ok {
				s.SetQuotaAlerts(v)
			}
		})
		row(l.Text(l10n.KeyResetCreditAlerts), l.Text(l10n.KeyResetCreditAlertsDescription), func() {
			if v, ok := toggle(c, "alert-credits", l.Text(l10n.KeyResetCreditAlerts), p.ResetCreditAlertsEnabled, ro); ok {
				s.SetResetCreditAlerts(v)
			}
		})
		row(l.Text(l10n.KeyRateLimitResetTodayAlerts), l.Text(l10n.KeyRateLimitResetTodayAlertsDescription), func() {
			if v, ok := toggle(c, "alert-reset-today", l.Text(l10n.KeyRateLimitResetTodayAlerts), p.RateLimitResetTodayAlertsEnabled, ro); ok {
				s.SetRateLimitResetTodayAlerts(v)
			}
		})
		row(l.Text(l10n.KeyTestNotification), l.Text(l10n.KeyTestNotificationSubtitle), func() {
			test := u.opts.Actions.TestNotification
			if PillButton(c, nil, l.Text(l10n.KeyTestNotification), test == nil).Clicked() {
				u.showNotice(l.Text(l10n.KeyTestNotification), test())
			}
		})
	})
	u.quotaThresholdEditor(c)
}
