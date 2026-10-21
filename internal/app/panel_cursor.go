package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

const cursorUsageURL = "https://cursor.com/dashboard?tab=usage"

func cursorIdentityRow(c *ui.Context, l l10n.Localizer, p store.Preferences, s runway.CursorState) {
	ui.Row(c).Gap(8).Padding(8, 0, 0, 0).Children(func() {
		name := s.Email
		if name == "" {
			name = l.Text(l10n.KeyProviderCursor)
		}
		ui.Text(c, accountPrivate(p.AccountPrivacyModeEnabled, name)).FontSize(fsCallout).Grow(1).Basis(0).SingleLine().Ellipsis("…")
		if s.Plan != "" {
			runwayTagPadded(c, s.Plan, toneNeutral, 1, 5)
		}
	})
}

func panelCursorQuota(c *ui.Context, l l10n.Localizer, p store.Preferences, s runway.CursorState, acts PanelActions, first *bool, st *panelState) {
	sectionBlock(c, first, func() {
		spec := sectionSpec{title: l.Text(l10n.KeyProviderCursor), refreshing: s.Refreshing}
		if p.CursorMonitoringEnabled {
			spec.refresh = sectionRefresh(acts, runway.SectionQuota)
		}
		drawSectionHead(c, l, st, spec)
		if !p.CursorMonitoringEnabled {
			dimText(c, l.Text(l10n.KeyCursorReadOnlyHint))
			if accentButton(c, icPlug, l.Text(l10n.KeyCursorEnable), acts.SetCursorMonitoring == nil).Clicked() && acts.SetCursorMonitoring != nil {
				acts.SetCursorMonitoring(true)
			}
			return
		}
		if s.Stale {
			dimText(c, l.Text(l10n.KeyCursorStale))
		}
		ui.Column(c).Gap(12).Children(func() {
			for _, m := range s.Meters {
				grokMeterRow(c.Key("cursor-pool-"+m.Title), l, m, true, time.Now(), st.resetSwitch(p.QuotaResetTimeFormat))
			}
			for _, line := range s.Lines {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, line.Title).FontSize(fsCaption).Grow(1).Basis(0)
					ui.Text(c, line.Value).FontSize(fsCaption).FontFeatures("tnum")
				})
			}
		})
		if len(s.Meters) == 0 && len(s.Lines) == 0 && s.LastError == "" {
			sectionEmpty(c, l, s.Refreshing)
		}
		if !s.UpdatedAt.IsZero() {
			dimText(c, l.Text(l10n.KeyLastUpdated)+" · "+s.UpdatedAt.In(time.Local).Format("2006-01-02 15:04:05"))
		}
		if linkButton(c, l.Text(l10n.KeyCursorOpenUsage)).Clicked() {
			c.OpenURL(cursorUsageURL)
		}
	})
}

func (u *settingsUI) cursorGroup(c *ui.Context) {
	s, l := u.s, u.L()
	settingsGroup(c, l.Text(l10n.KeyProviderCursor), "", nil, func(row rowFunc) {
		row(l.Text(l10n.KeyCursorEnable), l.Text(l10n.KeyCursorReadOnlyHint), func() {
			if v, ok := toggle(c, "cursor-monitor", l.Text(l10n.KeyCursorEnable), s.Prefs().CursorMonitoringEnabled, s.ReadOnly()); ok {
				s.SetCursorMonitoringEnabled(v)
			}
		})
	})
}

func renderCursorAccount(c *ui.Context, u *settingsUI, s runway.CursorState, refresh func()) {
	u.cursorGroup(c)
	p, l := u.s.Prefs(), u.L()
	if !p.CursorMonitoringEnabled {
		return
	}
	settingsSection(c, l.Text(l10n.KeyAccountsCurrent), "", func() {
		if ghostButton(c, icRefresh, l.Text(l10n.KeyRefresh), s.Refreshing, refresh == nil).Clicked() {
			refresh()
		}
	}, func() {
		cursorIdentityRow(c, l, p, s)
		summary := runway.CursorStatusText(l, s)
		if len(s.Meters) > 0 && !s.Stale {
			summary = s.Meters[0].Title + " · " + summary
		}
		if s.LastError != "" {
			summary = s.LastError
		}
		dimText(c, accountUpdatedSummary(l, summary, s.UpdatedAt))
		if linkButton(c, l.Text(l10n.KeyCursorOpenUsage)).Clicked() {
			c.OpenURL(cursorUsageURL)
		}
	})
}
