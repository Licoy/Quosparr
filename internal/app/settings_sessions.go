package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
)

func (u *settingsUI) sessionsPage(c *ui.Context) {
	s, actions := u.sessionsState(), u.sessionActions()
	if !s.loaded && !s.task.Running && s.task.Err == nil {
		u.startSessionsScan(actions)
	}
	// The tabs, filters and selection bar stay put; the list or the open
	// session scrolls under them, its bar at the window's edge. An open
	// session hides the tabs, as a page of its own.
	item, open := s.findOpen()
	if s.open != "" && !open {
		s.open = ""
	}
	key := "session-list-" + string(s.provider)
	if open {
		key = "session-detail-" + s.open
	}
	splitPage(c, key, func() {
		if open {
			u.sessionDetailHeader(c, item, actions)
			return
		}
		u.sessionProviderTabs(c, actions)
		u.sessionFilterBar(c)
		u.sessionNotice(c)
		u.sessionListHeader(c, actions)
	}, func() {
		if open {
			u.sessionDetailPage(c, item, actions)
			return
		}
		u.sessionList(c, actions)
	})
}

// sessionListHeader reports a failed refresh and holds the selection bar
// once the list has loaded.
func (u *settingsUI) sessionListHeader(c *ui.Context, actions SessionActions) {
	s, l, t := u.sessionsState(), u.L(), c.Theme()
	if !s.loaded {
		return
	}
	if s.task.Err != nil {
		ui.Text(c, tx(l, xSessionsRefreshFailed, s.task.Err.Error())).FontSize(12).TextColor(t.Danger)
	}
	u.sessionSelectionBar(c, s.filtered(), actions)
}

func (u *settingsUI) sessionProviderTabs(c *ui.Context, actions SessionActions) {
	s, l, t := u.sessionsState(), u.L(), c.Theme()
	providers := store.CLIProviders()
	selected := 0
	for i, provider := range providers {
		if provider == s.provider {
			selected = i
		}
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		seg := ui.SegmentedBase(c.Key("session-providers"), &selected, len(providers))
		seg.Track.Padding(3).Gap(2).Radius(7).Background(t.Text.Alpha(0.06)).Children(func() {
			for i, provider := range providers {
				segment := seg.Segment(i).Padding(5, 10).Gap(6).Radius(5).Label(l.Text(providerKey(provider))).Disabled(s.work.Running)
				color := t.TextMuted
				if i == selected {
					color = t.Text
					segment.Background(pal(t).Inset).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.12))
				}
				segment.Children(func() {
					ProviderIcon(c, provider, 15)
					ui.Text(c, l.Text(providerKey(provider))).FontSize(13).FontWeight(500).TextColor(color).SingleLine()
				})
			}
		})
		if seg.Track.Changed() {
			s.switchProvider(providers[selected])
			u.startSessionsScan(actions)
		}
		ui.Spacer(c).Grow(1)
		if s.task.Running || s.work.Running {
			ui.Spinner(c).Size(14, 14)
		}
		if IconButton(c, icRefresh, l.Text(l10n.KeyRefresh), s.task.Running || s.work.Running || actions.Query == nil).Clicked() {
			u.startSessionsScan(actions)
		}
	})
}

func (u *settingsUI) sessionList(c *ui.Context, actions SessionActions) {
	s, l, t := u.sessionsState(), u.L(), c.Theme()
	if !s.loaded {
		if s.task.Err != nil {
			ui.Text(c, tx(l, xSessionsLoadFailed, s.task.Err.Error())).FontSize(12).TextColor(t.Danger)
		} else {
			skillsLoading(c)
		}
		return
	}
	list := s.filtered()
	if len(list) == 0 {
		message := tx(l, xSessionsEmpty)
		if len(s.items) > 0 {
			message = tx(l, xSessionsNoMatch)
		}
		skillsEmptyBox(c, message)
		return
	}
	shown := min(s.shown, len(list))
	listPanel(c).Children(func() {
		for _, item := range list[:shown] {
			u.sessionRow(c, item, actions)
		}
	})
	if shown < len(list) {
		ui.Row(c).Justify(ui.Center).Children(func() {
			if PillButton(c, nil, tx(l, xSessionsShowMore, min(sessionPageSize, len(list)-shown)), false).Clicked() {
				s.shown += sessionPageSize
			}
		})
	}
}

// sessionSelectionBar tops the list: a box selecting every filtered
// session, mixed while only some are, the counts, and on the right the
// batch actions.
func (u *settingsUI) sessionSelectionBar(c *ui.Context, list []sessions.ManagedSession, actions SessionActions) {
	s, l, t := u.sessionsState(), u.L(), c.Theme()
	selected := len(s.selectedItems())
	all := len(list) > 0
	for _, item := range list {
		if !s.selected[sessionKey(item)] {
			all = false
			break
		}
	}
	listBar(c).Children(func() {
		label := tx(l, xSessionsSelectFiltered, len(list))
		if all {
			label = tx(l, xSessionsClearSelection)
		}
		if mixedCheckbox(c, "session-select-all", all, selected > 0, label, s.work.Running || len(list) == 0) {
			if all {
				s.clearSelection()
			} else {
				s.selected = make(map[string]bool, len(list))
				for _, item := range list {
					s.selected[sessionKey(item)] = true
				}
			}
		}
		counts := tx(l, xSessionsShowing, min(s.shown, len(list)), len(list))
		if selected > 0 {
			counts += " · " + tx(l, xSessionsSelected, selected)
		}
		ui.Text(c, counts).FontSize(12).FontFeatures("tnum").TextColor(t.TextMuted).SingleLine().Grow(1).Basis(0)
		if SmallPillButton(c, icTrash, tx(l, xSessionsDeleteShort), s.work.Running || selected == 0 || actions.Delete == nil).
			Label(tx(l, xSessionsDeleteSelected)).Tooltip(tx(l, xSessionsDeleteSelected)).Cursor(ui.CursorPointer).Clicked() {
			s.confirmDelete("")
		}
	})
}

// sessionRow is one session: its box, then its title and details, which
// open it, then its actions. The whole row lights up under the pointer.
func (u *settingsUI) sessionRow(c *ui.Context, item sessions.ManagedSession, actions SessionActions) {
	s, l, t := u.sessionsState(), u.L(), c.Theme()
	key := sessionKey(item)
	checked := s.selected[key]
	title := item.Title
	if strings.TrimSpace(title) == "" {
		title = l.Text(l10n.KeyUnknown)
	}
	listRow(c, "session-row-"+key, checked).Children(func() {
		if ui.Checkbox(c.Key("session-check-"+key), &checked, "").Label(tx(l, xSessionsSelectOne, title)).Size(18, 18).Disabled(s.work.Running).Cursor(ui.CursorPointer).Changed() {
			if checked {
				s.selected[key] = true
			} else {
				delete(s.selected, key)
			}
		}
		button := ui.ButtonBase(c.Key("session-open-" + key)).Label(tx(l, xSessionsOpenDetails, title)).Grow(1).Basis(0).Cursor(ui.CursorPointer)
		button.Children(func() {
			ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, title).FontSize(13).FontWeight(550).SingleLine().Ellipsis("…").Shrink(1)
					if item.Archived {
						ui.Text(c, tx(l, xSessionsArchived)).FontSize(10).TextColor(t.TextMuted)
					}
					if item.Problem != "" || item.State == sessions.StateNeedsAttention {
						ui.Text(c, tx(l, xSessionsNeedsAttention)).FontSize(10).TextColor(t.Warning)
					}
				})
				meta := []string{item.ProjectName, item.UpdatedAt.Local().Format("2006-01-02 15:04"), tokenCompact(item.Totals.TotalTokens) + " " + l.Text(l10n.KeyTokens)}
				ui.Text(c, strings.Join(nonEmpty(meta), " · ")).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			})
		})
		if button.Clicked() {
			s.open = key
			u.loadSessionDetail(actions, item)
		}
		u.sessionActionButtons(c, item, actions)
	})
}

// sessionActionButtons show a session's files and move it to the Trash,
// after the same confirmation as the selection.
func (u *settingsUI) sessionActionButtons(c *ui.Context, item sessions.ManagedSession, actions SessionActions) {
	s, l := u.sessionsState(), u.L()
	if actions.Reveal != nil && IconButton(c, icFolder, tx(l, xSessionsReveal), false).Cursor(ui.CursorPointer).Clicked() {
		if err := actions.Reveal(item.Path); err != nil {
			s.notice, s.noticeErr = err.Error(), true
		}
	}
	if actions.Delete != nil && IconButton(c, icTrash, tx(l, xSessionsDeleteOne), s.work.Running).Cursor(ui.CursorPointer).Clicked() {
		s.confirmDelete(sessionKey(item))
	}
}

func nonEmpty(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}
