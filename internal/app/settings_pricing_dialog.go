package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
	"strconv"
)

func (u *settingsUI) pricingProviderTabs(c *ui.Context, snap PricingSnapshot, a PricingActions) {
	s, l, t := u.pricingState(), u.L(), c.Theme()
	providers := store.Providers()
	selected := slicesIndexProvider(providers, s.provider)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		seg := ui.SegmentedBase(c.Key("pricing-providers"), &selected, len(providers))
		seg.Track.Padding(3).Gap(2).Radius(7).Background(t.Text.Alpha(0.06)).Children(func() {
			for i, provider := range providers {
				part := seg.Segment(i).Padding(5, 10).Gap(6).Radius(5).Label(l.Text(providerKey(provider)))
				color := t.TextMuted
				if i == selected {
					color = t.Text
					part.Background(pal(t).Inset).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.12))
				}
				part.Children(func() {
					ProviderIcon(c, provider, 15)
					ui.Text(c, l.Text(providerKey(provider))).FontSize(13).FontWeight(500).TextColor(color).SingleLine()
					if n := s.providerCounts[pricingProviderID(provider)]; n > 0 {
						ui.Text(c, strconv.Itoa(n)).FontSize(11).FontFeatures("tnum").TextColor(t.TextMuted).SingleLine()
					}
				})
			}
		})
		if seg.Track.Changed() {
			s.switchProvider(providers[selected])
		}
		ui.Spacer(c).Grow(1)
		if SmallPillButton(c, icSliders, l.Pricing("syncSettings"), false).Clicked() {
			s.syncSettings = true
		}
		u.pricingSyncButton(c, snap, a, true)
	})
}
func (u *settingsUI) pricingNotice(c *ui.Context, snap PricingSnapshot) {
	u.pricingStatus(c, snap, false)
	u.pricingSyncError(c, snap)
}

// pricingStatus is the sync state behind a dot, with the time of the last
// successful sync when at is set.
func (u *settingsUI) pricingStatus(c *ui.Context, snap PricingSnapshot, at bool) {
	s, l, t := u.pricingState(), u.L(), c.Theme()
	busy := s.task.Running || snap.Syncing
	message, dot := l.Pricing("synced"), t.Success
	switch {
	case busy:
		message = l.Pricing("syncing")
	case snap.LastSuccess.IsZero():
		message, dot = l.Pricing("usingBuiltin"), t.TextMuted
	case at:
		message += " · " + pricingTime(snap.LastSuccess)
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if busy {
			ui.Spinner(c).Size(14, 14)
		} else {
			ui.Box(c).Size(6, 6).Radius(3).Background(dot)
		}
		ui.Text(c, message).FontSize(11).FontFeatures("tnum").TextColor(t.TextMuted)
	})
}
func (u *settingsUI) pricingSyncError(c *ui.Context, snap PricingSnapshot) {
	if err := u.pricingState().syncError(snap); err != "" {
		ui.Text(c, u.L().Pricing("error")+": "+err).FontSize(11).TextColor(c.Theme().Danger)
	}
}
func (u *settingsUI) pricingDialog(c *ui.Context) {
	s := u.pricing
	if s == nil {
		return
	}
	if u.tab != tabModelPricing {
		s.syncSettings = false
		return
	}
	if !s.syncSettings {
		return
	}
	a := u.pricingActions()
	snap := PricingSnapshot{}
	if a.Snapshot != nil {
		snap = a.Snapshot()
	}
	l := u.L()
	ui.Modal(c.Key("pricing-sync-settings"), &s.syncSettings, func() {
		ui.Text(c, l.Pricing("syncSettings")).FontSize(17).FontWeight(600)
		ui.Text(c, l.Pricing("globalSync")).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Scroll(c.Key("pricing-sync-body")).Height(320).Gap(12).Children(func() {
			u.pageNotices(c)
			u.pricingSyncControls(c)
			u.pricingSyncStatus(c, snap)
			ui.Text(c, l.Pricing("unit")).FontSize(11).TextColor(c.Theme().TextMuted)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			u.pricingSyncButton(c, snap, a, false)
			ui.Spacer(c).Grow(1)
			if PillButton(c, nil, l.Pricing("done"), false).Clicked() {
				s.syncSettings = false
			}
		})
	}).Width(480)
}
func (u *settingsUI) pricingSyncControls(c *ui.Context) {
	s, l := u.pricingState(), u.L()
	p := u.s.Prefs().ModelPricing
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		on := p.AutoSync
		if ui.Switch(c.Key("pricing-auto"), &on).Label(l.Pricing("auto")).Disabled(u.s.ReadOnly()).Changed() {
			u.s.Update(func(p *store.Preferences) { p.ModelPricing.AutoSync = on })
		}
		ui.Text(c, l.Pricing("auto")).FontSize(12)
	})
	listMenuButton(c, "pricing-interval", nil, l.Pricing("interval")+": "+pricingIntervalLabel(u, p.SyncIntervalSeconds), l.Pricing("interval"), u.s.ReadOnly(), func(m *ui.Menu) {
		for _, seconds := range []int{900, 1800, 3600, 10800, 21600, 43200, 86400} {
			pricingMenuItem(m, pricingIntervalLabel(u, seconds), seconds == p.SyncIntervalSeconds, func() {
				u.s.Update(func(p *store.Preferences) { p.ModelPricing.SyncIntervalSeconds = seconds })
				s.custom = ""
				s.customOpen = false
				s.validation = ""
			})
		}
		pricingMenuItem(m, l.Pricing("custom"), false, func() { s.custom = strconv.Itoa(p.SyncIntervalSeconds / 60); s.customOpen = true; s.validation = "" })
	})
	if s.customOpen {
		ui.Column(c).Gap(5).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, l.Text(l10n.KeyMinutes)).FontSize(12)
				if ui.TextInput(c.Key("pricing-custom"), &s.custom).Width(110).Label(l.Pricing("custom")).Disabled(u.s.ReadOnly()).Changed() {
					u.savePricingInterval(s.custom)
				}
			})
			if s.validation != "" {
				ui.Text(c, s.validation).FontSize(11).TextColor(c.Theme().Danger)
			}
		})
	}
}
func (u *settingsUI) pricingSyncStatus(c *ui.Context, snap PricingSnapshot) {
	l := u.L()
	u.pricingNotice(c, snap)
	for _, field := range []struct{ key, value string }{{"source", "models.dev"}, {"version", pricingShortVersion(snap.Version)}, {"lastAttempt", pricingTime(snap.LastAttempt)}, {"lastSuccess", pricingTime(snap.LastSuccess)}, {"lastChanged", pricingTime(snap.LastChanged)}, {"nextSync", pricingTime(snap.NextSync)}} {
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, l.Pricing(field.key)).Width(150).FontSize(11).TextColor(c.Theme().TextMuted)
			ui.Text(c, field.value).Tooltip(snap.Version).FontSize(11).Grow(1).Basis(0)
		})
	}
}

// The spinner lives inside the disabled button and schedules its native paints.
func (u *settingsUI) pricingSyncButton(c *ui.Context, snap PricingSnapshot, a PricingActions, iconOnly bool) {
	s, l, t := u.pricingState(), u.L(), c.Theme()
	busy := s.task.Running || snap.Syncing
	label := l.Pricing("refresh")
	if iconOnly {
		label = l.Text(l10n.KeyRefresh)
	}
	b := ui.ButtonBase(c.Key("pricing-refresh")).Label(label).Tooltip(label).Disabled(busy || a.Refresh == nil).Radius(6).Gap(6).AlignItems(ui.Center).Justify(ui.Center)
	if iconOnly {
		b.Size(26, 26)
	} else {
		b.Padding(6, 10).Background(t.Text.Alpha(0.06))
	}
	if b.Hovered() && !busy {
		b.Background(pal(t).Hover)
	}
	color := t.TextMuted
	if busy || (!iconOnly && a.Refresh != nil) {
		color = t.Text
	}
	b.Children(func() {
		if busy {
			ui.Spinner(c).Size(14, 14).TextColor(t.Text)
		} else {
			ui.Icon(c, icRefresh).Size(14, 14).TextColor(color)
		}
		if !iconOnly {
			text := l.Pricing("refresh")
			if busy {
				text = l.Pricing("syncing")
			}
			ui.Text(c, text).FontSize(12).TextColor(color).SingleLine()
		}
	})
	if b.Clicked() {
		u.refreshPricing(a)
	}
}
