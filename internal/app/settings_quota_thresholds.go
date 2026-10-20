package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
)

// SetQuotaAlertThresholds validates a complete edit before changing live settings.
// Persistence uses the regular settings queue, including rollback on save failure.
func (s *Settings) SetQuotaAlertThresholds(values []int) error {
	normalized, err := store.NormalizeQuotaAlertThresholds(values)
	if err != nil {
		return err
	}
	if !s.Update(func(p *store.Preferences) { p.QuotaAlertThresholds = normalized }) {
		return ErrReadOnly
	}
	return nil
}

func (u *settingsUI) quotaThresholdEditor(c *ui.Context) {
	p, l := u.s.Prefs(), u.L()
	if !p.QuotaAlertsEnabled {
		return
	}
	ro := u.s.ReadOnly()
	settingsSection(c, tx(l, xQuotaThresholds), tx(l, xQuotaThresholdHint), nil, func() {
		ui.Column(c).Gap(12).Padding(10, 0).Children(func() {
			remove := -1
			ui.Row(c).Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
				for i, value := range p.QuotaAlertThresholds {
					if u.quotaThresholdTag(c, value, ro || len(p.QuotaAlertThresholds) == 1) {
						remove = i
					}
				}
				u.quotaThresholdSelect(c)
			})
			if remove >= 0 {
				u.applyQuotaThresholds(slices.Delete(p.QuotaAlertThresholds, remove, remove+1))
			}
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tx(l, xQuotaThresholdRules)).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1).Basis(0)
				if SmallPillButton(c, nil, tx(l, xQuotaThresholdDefaults), ro).Clicked() {
					u.applyQuotaThresholds(store.DefaultQuotaAlertThresholds())
				}
			})
		})
	})
}

func (u *settingsUI) quotaThresholdTag(c *ui.Context, value int, disabled bool) bool {
	t := c.Theme()
	label := tx(u.L(), xQuotaThresholdRemove, value)
	ink := t.Accent
	if t.Dark {
		ink = t.Text
	}
	b := ui.ButtonBase(c.Key(fmt.Sprintf("quota-tier-%d", value))).Label(label).Tooltip(label).Disabled(disabled).
		Padding(6, 8, 6, 11).Gap(6).Radius(14).AlignItems(ui.Center).Background(t.Accent.Alpha(0.12))
	if !disabled {
		b.Cursor(ui.CursorPointer)
		if b.Pressed() {
			b.Background(t.Accent.Alpha(0.28))
		} else if b.Hovered() {
			b.Background(t.Accent.Alpha(0.20))
		}
	} else {
		b.Cursor(ui.CursorDefault)
	}
	b.Children(func() {
		ui.Text(c, fmt.Sprintf("%d%%", value)).FontSize(12).TextColor(ink).SingleLine()
		close := ui.Box(c).Size(16, 16).Radius(8).AlignItems(ui.Center).Justify(ui.Center)
		closeInk := ink.Alpha(0.65)
		if !disabled && close.Hovered() {
			closeInk = ink
			alpha := float32(0.12)
			if b.Pressed() {
				alpha = 0.22
			}
			close.Background(ink.Alpha(alpha))
		}
		close.Children(func() { ui.Icon(c, icClose).Size(10, 10).TextColor(closeInk) })
	})
	return b.Clicked()
}

func (u *settingsUI) quotaThresholdSelect(c *ui.Context) {
	values := u.s.Prefs().QuotaAlertThresholds
	var options []string
	for option := 0; option <= 100; option++ {
		if !slices.Contains(values, option) {
			options = append(options, fmt.Sprintf("%d%%", option))
		}
	}
	label := tx(u.L(), xQuotaThresholdAdd)
	choice := label
	// A new selection resets the native field to its add prompt; typing is
	// internal to Combobox and does not change preferences until a choice.
	key := "quota-add-" + fmt.Sprint(values)
	if ui.Combobox(c.Key(key), &choice, options).Label(label).Width(160).
		Disabled(u.s.ReadOnly() || len(options) == 0).Changed() {
		value, err := strconv.Atoi(strings.TrimSuffix(choice, "%"))
		if err != nil {
			u.showNotice(tx(u.L(), xQuotaThresholds), tx(u.L(), xQuotaThresholdInvalid, err))
			return
		}
		u.applyQuotaThresholds(append(values, value))
	}
}

func (u *settingsUI) applyQuotaThresholds(values []int) {
	if err := u.s.SetQuotaAlertThresholds(values); err != nil {
		u.showNotice(tx(u.L(), xQuotaThresholds), tx(u.L(), xQuotaThresholdInvalid, err))
	}
}
