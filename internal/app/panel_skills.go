package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

// panelSkills is the skills summary of the main panel: how many global
// skills the selected CLI has, how many are off, and a row to the page.
func panelSkills(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState) {
	s := &st.skills
	if st.syncSkills != nil {
		st.syncSkills(acts)
	}
	title := tx(l, xSkillsTitle)
	var refresh func()
	if st.refreshSkills != nil && acts.Skills.Query != nil {
		refresh = func() { st.refreshSkills(acts) }
	}
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, sectionSpec{
			title: title, refreshing: s.task.Running, refresh: refresh,
			info: &sectionInfo{id: "skills-info", help: title, body: popoverBody(c, l, st, title, tx(l, xSkillsInfo))},
		})
		sum, ok := s.summary(snap.Provider)
		switch {
		case ok:
			skillsSummaryLines(c, l, sum)
		case s.task.Err != nil:
			ui.Text(c, tx(l, xSkillsScanFailed)).FontSize(fsCallout).TextColor(sys(c.Theme(), sysRed))
		default:
			sectionEmpty(c, l, s.task.Running)
		}
		if disclosureRow(c, icSkill, tx(l, xSkillsManage), true).Clicked() {
			st.detail = store.ModuleSkills
		}
	})
}

func skillsSummaryLines(c *ui.Context, l l10n.Localizer, sum skills.Summary) {
	t := c.Theme()
	if len(sum.Skills) == 0 {
		calloutDim(c, tx(l, xSkillsEmpty))
	} else {
		ui.Text(c, tx(l, xSkillsSummary, len(sum.Skills), skillsOff(sum))).FontSize(fsCallout).FontFeatures("tnum")
	}
	if n := len(sum.Issues); n > 0 {
		ui.Text(c, tx(l, xSkillsIssueCount, n)).FontSize(fsCaption).TextColor(sys(t, sysOrange))
	}
}

func skillsOff(sum skills.Summary) int {
	n := 0
	for _, sk := range sum.Skills {
		if sk.State == skills.StateOff {
			n++
		}
	}
	return n
}

// skillsPage is the skills detail page: the list, or one skill's page.
func (a *App) skillsPage(c *ui.Context, l l10n.Localizer, prefs store.Preferences, acts PanelActions) {
	s := &a.panelState.skills
	a.syncSkills(acts)
	if s.scrolled != s.open {
		a.panelState.detailScroll = ui.ScrollState{}
		s.scrolled = s.open
	}
	sum, ok := s.summary(prefs.SelectedProvider)
	if s.open != "" {
		if sk, found := s.find(prefs.SelectedProvider, s.open); found {
			a.skillItemPage(c, l, sum, sk, acts)
			return
		}
		if ok && !s.task.Running {
			s.closeItem() // gone after a rescan
		}
	}
	ui.Column(c).Gap(10).Grow(1).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c.Key("skills-search"), &s.search).Placeholder(tx(l, xSkillsSearch)).Grow(1).Basis(0)
			refreshButton(c, l, s.task.Running || s.work.Running, func() { a.startSkillsQuery(acts) })
			if accountIconButton(c, icSliders, tx(l, xSkillsManageInSettings), false, false, false).Clicked() {
				a.openSkillsSettings(prefs.SelectedProvider)
			}
		})
		skillsNotice(c, s)
		switch {
		case !ok && s.task.Err != nil:
			ui.Text(c, tx(l, xSkillsScanFailed)).FontSize(fsCallout).TextColor(sys(c.Theme(), sysRed))
			return
		case !ok:
			sectionEmpty(c, l, true)
			return
		}
		skillsSummaryLines(c, l, sum)
		ui.Scroll(c.Key("skills-scroll")).Grow(1).TrackScroll(&a.panelState.detailScroll).Children(func() {
			ui.Column(c).Gap(14).Padding(2, 0, 8, 0).Children(func() {
				skillGroups(c, l, sum, s)
				a.skillIssues(c, l, sum, acts)
			})
		})
	})
}

func skillsNotice(c *ui.Context, s *panelSkillsState) {
	if s.notice == "" {
		return
	}
	t := c.Theme()
	col := inkSecondary(t)
	if s.noticeErr {
		col = sys(t, sysRed)
	}
	ui.Text(c, s.notice).FontSize(fsCaption).TextColor(col)
}

// skillGroup is a run of skills from one location, in the CLI's order.
type skillGroup struct {
	key, title string
	items      []skills.Skill
}

func groupSkills(l l10n.Localizer, list []skills.Skill, search string) []skillGroup {
	var groups []skillGroup
	at := map[string]int{}
	for _, sk := range list {
		if !skillMatches(sk, search) {
			continue
		}
		key, title := skillGroupOf(l, sk)
		i, ok := at[key]
		if !ok {
			i = len(groups)
			at[key] = i
			groups = append(groups, skillGroup{key: key, title: title})
		}
		groups[i].items = append(groups[i].items, sk)
	}
	return groups
}

func skillGroupOf(l l10n.Localizer, sk skills.Skill) (key, title string) {
	if sk.Scope == skills.ScopeUser {
		return "user\x00" + sk.Root, sk.RootLabel
	}
	return string(sk.Scope), skillScopeTitle(l, sk.Scope)
}

func skillScopeTitle(l l10n.Localizer, scope skills.Scope) string {
	switch scope {
	case skills.ScopeSystem:
		return tx(l, xSkillsScopeSystem)
	case skills.ScopeBundled:
		return tx(l, xSkillsScopeBundled)
	case skills.ScopeAdmin:
		return tx(l, xSkillsScopeAdmin)
	case skills.ScopePlugin:
		return tx(l, xSkillsScopePlugin)
	case skills.ScopeSynced:
		return tx(l, xSkillsScopeSynced)
	}
	return string(scope)
}

func skillMatches(sk skills.Skill, search string) bool {
	q := strings.ToLower(strings.TrimSpace(search))
	if q == "" {
		return true
	}
	for _, v := range []string{sk.InvokeName, sk.DisplayName, sk.Description, sk.ShortDescription, sk.Plugin} {
		if strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

func skillGroups(c *ui.Context, l l10n.Localizer, sum skills.Summary, s *panelSkillsState) {
	groups := groupSkills(l, sum.Skills, s.search)
	if len(groups) == 0 {
		if len(sum.Skills) > 0 {
			calloutDim(c, tx(l, xSkillsNoMatch))
		}
		return
	}
	t := c.Theme()
	for _, g := range groups {
		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, g.title).FontSize(fsCaption).FontWeight(600).TextColor(inkSecondary(t)).Grow(1).Basis(0).SingleLine().Ellipsis("…")
				ui.Text(c, fmt.Sprint(len(g.items))).FontSize(fsCaption).FontFeatures("tnum").TextColor(inkTertiary(t))
			})
			cardColumn(c, surfaceSunkenKind, radiusCard).Clip().Children(func() {
				for i, sk := range g.items {
					tableRowRule(c, i == 0)
					if skillRow(c, l, sk).Clicked() {
						s.closeItem()
						s.open = sk.ID
					}
				}
			})
		})
	}
}

func skillTitle(sk skills.Skill) string {
	if sk.DisplayName != "" {
		return sk.DisplayName
	}
	return sk.InvokeName
}

func skillSubtitle(sk skills.Skill) string {
	if sk.ShortDescription != "" {
		return sk.ShortDescription
	}
	return sk.Description
}

// skillRow is one list row: the name with its state badges, one line of
// description, and a chevron to the skill's page.
func skillRow(c *ui.Context, l l10n.Localizer, sk skills.Skill) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(skillTitle(sk)).Padding(7, 10).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(hoverNeutral(t))
	}
	return b.Children(func() {
		ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				name := ui.Text(c, skillTitle(sk)).FontSize(fsCallout).FontWeight(500).SingleLine().Ellipsis("…").Shrink(1)
				if sk.State != skills.StateOn {
					name.TextColor(inkSecondary(t))
				}
				if sk.LinkPath != "" {
					ui.Icon(c, icLink).Size(11, 11).TextColor(inkTertiary(t))
				}
				skillStateBadge(c, l, sk.State)
			})
			if sub := skillSubtitle(sk); sub != "" {
				ui.Text(c, sub).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine().Ellipsis("…")
			}
			if len(sk.SharedWith) > 0 {
				ui.Text(c, tx(l, xSkillsSharedWith)+" "+skillProviders(l, sk.SharedWith)).FontSize(fsCaption).TextColor(inkTertiary(t)).SingleLine().Ellipsis("…")
			}
		})
		ui.Icon(c, icChevRight).Size(12, 12).TextColor(inkSecondary(t))
	})
}

// skillStateBadge is the small tag of a skill that is not in use.
func skillStateBadge(c *ui.Context, l l10n.Localizer, state skills.State) {
	t := c.Theme()
	var text string
	col := sys(t, sysGray)
	switch state {
	case skills.StateOff:
		text, col = tx(l, xSkillsStateOff), sys(t, sysOrange)
	case skills.StateOverridden:
		text = tx(l, xSkillsStateOverridden)
	case skills.StateNotLoaded:
		text = tx(l, xSkillsStateNotLoaded)
	default:
		return
	}
	ui.Row(c).Padding(1, 5).Radius(radiusControl).Background(col.Alpha(0.14)).Children(func() {
		ui.Text(c, text).FontSize(fsCaption).FontWeight(500).TextColor(col).SingleLine()
	})
}

func skillProviders(l l10n.Localizer, ps []skills.Provider) string {
	names := make([]string, 0, len(ps))
	for _, p := range ps {
		names = append(names, l.Text(providerKey(store.Provider(p))))
	}
	return strings.Join(names, ", ")
}

// skillPath shows path under the summary's roots the way their labels do
// ("~/.agents/skills/…").
func skillPath(sum skills.Summary, path string) string {
	best := -1
	for i, r := range sum.Roots {
		if r.Path == "" || r.Label == "" || r.Label == r.Path {
			continue
		}
		if rel, err := filepath.Rel(r.Path, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if best < 0 || len(r.Path) > len(sum.Roots[best].Path) {
				best = i
			}
		}
	}
	if best < 0 {
		return path
	}
	r := sum.Roots[best]
	return r.Label + strings.TrimPrefix(path, r.Path)
}

// skillIssues is the diagnostics list under the skills.
func (a *App) skillIssues(c *ui.Context, l l10n.Localizer, sum skills.Summary, acts PanelActions) {
	if len(sum.Issues) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, tx(l, xSkillsIssuesTitle)).FontSize(fsCaption).FontWeight(600).TextColor(inkSecondary(t))
		cardColumn(c, surfaceSunkenKind, radiusCard).Clip().Children(func() {
			for i, is := range sum.Issues {
				tableRowRule(c, i == 0)
				ui.Row(c).Gap(8).Padding(7, 10).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icWarnCircle).Size(12, 12).TextColor(sys(t, sysOrange))
					ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
						ui.Text(c, skillIssueText(l, is.Kind)).FontSize(fsCallout).FontWeight(500)
						if is.Name != "" {
							dimText(c, is.Name).SingleLine().Ellipsis("…")
						}
						ui.Text(c, skillPath(sum, is.Path)).FontSize(fsCaption).TextColor(inkTertiary(t)).Selectable()
					})
					if is.Kind == skills.IssueBrokenLink && is.Removable {
						a.removeLinkButton(c, l, sum, is, acts)
					}
				})
			}
		})
	})
}

func (a *App) removeLinkButton(c *ui.Context, l l10n.Localizer, sum skills.Summary, is skills.Issue, acts PanelActions) {
	s := &a.panelState.skills
	id := "link\x00" + is.Path
	del := acts.Skills.DeleteLink
	b := accountIconButton(c, icTrash, tx(l, xSkillsRemoveLink), del == nil || s.work.Running, false, false)
	open := s.confirm == id
	if b.Clicked() {
		s.confirm, open = id, true
	}
	confirmPopover(c, l, b, &open, tx(l, xSkillsRemoveLinkTitle), tx(l, xSkillsRemoveLinkMsg, skillPath(sum, is.Path)), tx(l, xSkillsDelete), nil, func() {
		a.runSkillWork(acts, xSkillsDeleteFailed, func() (string, error) {
			system, err := del(sum, is.Path)
			return skillMovedText(l, system), err
		}, nil)
	})
	if !open && s.confirm == id {
		s.confirm = ""
	}
}

func skillIssueText(l l10n.Localizer, k skills.IssueKind) string {
	switch k {
	case skills.IssueBrokenLink:
		return tx(l, xSkillsIssueBrokenLink)
	case skills.IssueNested:
		return tx(l, xSkillsIssueNested)
	case skills.IssueMissingDesc:
		return tx(l, xSkillsIssueNoDesc)
	case skills.IssueInvalidFront:
		return tx(l, xSkillsIssueBadFront)
	case skills.IssueNameTooLong:
		return tx(l, xSkillsIssueLongName)
	case skills.IssueDuplicateName:
		return tx(l, xSkillsIssueDuplicate)
	case skills.IssueTruncated:
		return tx(l, xSkillsIssueTruncated)
	}
	return string(k)
}

// skillItemPage is one skill: what it is, whether the CLI uses it, where it
// lives, the switch, the folder and trash actions, and its SKILL.md.
func (a *App) skillItemPage(c *ui.Context, l l10n.Localizer, sum skills.Summary, sk skills.Skill, acts PanelActions) {
	s := &a.panelState.skills
	t := c.Theme()
	ui.Scroll(c.Key("skill-item-scroll")).Grow(1).TrackScroll(&a.panelState.detailScroll).Children(func() {
		ui.Column(c).Gap(14).Padding(2, 0, 8, 0).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, skillTitle(sk)).FontSize(fsTitle3).FontWeight(600).Shrink(1).Selectable()
					skillStateBadge(c, l, sk.State)
				})
				if sk.Description != "" {
					ui.Text(c, sk.Description).FontSize(fsCallout).TextColor(inkSecondary(t)).Selectable()
				}
				if why := skillReasonText(l, sk.Reason); why != "" && sk.State != skills.StateOn {
					ui.Text(c, why).FontSize(fsCaption).TextColor(sys(t, sysOrange))
				}
			})
			a.skillSwitch(c, l, sk, acts)
			skillFacts(c, l, sum, sk)
			a.skillActions(c, l, sum, sk, acts)
			skillsNotice(c, s)
			a.skillContent(c, l, sk, acts)
		})
	})
}

// skillSwitchable says whether the switch can change what the CLI does: a
// skill turned off by something else than the setting the switch writes
// would stay off.
func skillSwitchable(sk skills.Skill) bool {
	if !sk.Toggleable {
		return false
	}
	if sk.State != skills.StateOff {
		return true
	}
	switch sk.Reason {
	case skills.ReasonConfigPath, skills.ReasonConfigName, skills.ReasonOverride, skills.ReasonDisabledList:
		return true
	}
	return false
}

func (a *App) skillSwitch(c *ui.Context, l l10n.Localizer, sk skills.Skill, acts PanelActions) {
	s := &a.panelState.skills
	t := c.Theme()
	set := acts.Skills.SetEnabled
	cardColumn(c, surfaceRaisedKind, radiusCard).Gap(6).Padding(10).Children(func() {
		if !skillSwitchable(sk) {
			calloutDim(c, skillToggleBlockText(l, sk))
			return
		}
		if !s.work.Running {
			s.enabled = sk.State != skills.StateOff
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tx(l, xSkillsEnabled)).FontSize(fsCallout).FontWeight(500).Grow(1).Basis(0)
			if s.work.Running {
				ui.Spinner(c).Size(14, 14)
			}
			if ui.Switch(c.Key("skill-enabled"), &s.enabled).Label(tx(l, xSkillsEnabled)).Disabled(set == nil || s.work.Running).Changed() && set != nil {
				on := s.enabled
				a.runSkillWork(acts, xSkillsToggleFailed, func() (string, error) { return "", set(sk, on) }, nil)
			}
		})
		ui.Text(c, skillToggleHint(l, sk.Provider)).FontSize(fsCaption).TextColor(inkSecondary(t))
	})
}

func skillToggleHint(l l10n.Localizer, p skills.Provider) string {
	switch p {
	case skills.ProviderClaude:
		return tx(l, xSkillsToggleClaude)
	case skills.ProviderGrok:
		return tx(l, xSkillsToggleGrok)
	}
	return tx(l, xSkillsToggleCodex)
}

func skillToggleBlockText(l l10n.Localizer, sk skills.Skill) string {
	switch {
	case sk.Reason == skills.ReasonPermissionDeny || sk.ToggleBlock == skills.ReasonPermissionDeny:
		return tx(l, xSkillsBlockDeny)
	case sk.Reason == skills.ReasonIgnored || sk.ToggleBlock == skills.ReasonIgnored:
		return tx(l, xSkillsBlockIgnored)
	case sk.ToggleBlock == skills.ReasonPluginCollection || sk.Scope == skills.ScopePlugin:
		return tx(l, xSkillsBlockPlugin)
	case sk.ToggleBlock == skills.ReasonNoToggle:
		return tx(l, xSkillsBlockNoToggle)
	case sk.State == skills.StateOff:
		if why := skillReasonText(l, sk.Reason); why != "" {
			return why
		}
	case sk.State == skills.StateNotLoaded:
		return tx(l, xSkillsBlockOther)
	}
	return tx(l, xSkillsBlockReadOnly)
}

func skillReasonText(l l10n.Localizer, r skills.Reason) string {
	key, ok := map[skills.Reason]textKey{
		skills.ReasonConfigPath:     xSkillsReasonConfigPath,
		skills.ReasonConfigName:     xSkillsReasonConfigName,
		skills.ReasonBundledOff:     xSkillsReasonBundledOff,
		skills.ReasonOverride:       xSkillsReasonOverride,
		skills.ReasonPermissionDeny: xSkillsReasonDeny,
		skills.ReasonDisabledList:   xSkillsReasonDisabled,
		skills.ReasonIgnored:        xSkillsReasonIgnored,
		skills.ReasonCompatOff:      xSkillsReasonCompatOff,
		skills.ReasonPluginOff:      xSkillsReasonPluginOff,
		skills.ReasonShadowed:       xSkillsReasonShadowed,
		skills.ReasonNested:         xSkillsReasonNested,
		skills.ReasonTooDeep:        xSkillsReasonTooDeep,
		skills.ReasonMissingDesc:    xSkillsReasonNoDesc,
		skills.ReasonInvalidFront:   xSkillsReasonBadFront,
		skills.ReasonHiddenDir:      xSkillsReasonHidden,
	}[r]
	if !ok {
		return ""
	}
	return tx(l, key)
}

// skillFacts is the label / value table of a skill page.
func skillFacts(c *ui.Context, l l10n.Localizer, sum skills.Summary, sk skills.Skill) {
	type fact struct{ label, value string }
	source := sk.RootLabel
	if sk.Scope != skills.ScopeUser {
		source = skillScopeTitle(l, sk.Scope) + " · " + sk.RootLabel
	}
	state := tx(l, xSkillsStateOn)
	switch sk.State {
	case skills.StateOff:
		state = tx(l, xSkillsStateOff)
	case skills.StateOverridden:
		state = tx(l, xSkillsStateOverridden)
	case skills.StateNotLoaded:
		state = tx(l, xSkillsStateNotLoaded)
	}
	facts := []fact{
		{tx(l, xSkillsState), state},
		{tx(l, xSkillsInvoke), sk.InvokeName},
		{tx(l, xSkillsLocation), skillPath(sum, sk.File)},
		{tx(l, xSkillsSource), source},
	}
	if sk.Plugin != "" {
		facts = append(facts, fact{tx(l, xSkillsPlugin), sk.Plugin})
	}
	if sk.LinkTarget != "" {
		facts = append(facts, fact{tx(l, xSkillsLinkTarget), sk.LinkTarget})
	}
	if sk.OverriddenBy != "" {
		facts = append(facts, fact{tx(l, xSkillsOverridden), skillPath(sum, sk.OverriddenBy)})
	}
	if len(sk.SharedWith) > 0 {
		facts = append(facts, fact{tx(l, xSkillsSharedWith), skillProviders(l, sk.SharedWith)})
	}
	if !sk.ModTime.IsZero() {
		facts = append(facts, fact{tx(l, xSkillsModified), sk.ModTime.Local().Format("2006-01-02 15:04")})
	}
	for _, f := range sk.Fields {
		facts = append(facts, fact{f.Key, f.Value})
	}
	t := c.Theme()
	cardColumn(c, surfaceSunkenKind, radiusCard).Clip().Children(func() {
		for i, f := range facts {
			tableRowRule(c, i == 0)
			ui.Row(c).Gap(10).Padding(6, 10).AlignItems(ui.Start).Children(func() {
				ui.Text(c, f.label).FontSize(fsCaption).TextColor(inkSecondary(t)).Width(92).Shrink(0)
				ui.Text(c, f.value).FontSize(fsCaption).Grow(1).Basis(0).Selectable()
			})
		}
	})
}

// skillActions are the folder buttons and the trash button with its
// confirmation, or why the skill cannot be deleted here.
func (a *App) skillActions(c *ui.Context, l l10n.Localizer, sum skills.Summary, sk skills.Skill, acts PanelActions) {
	s := &a.panelState.skills
	ac := acts.Skills
	ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		if IconButton(c, icFolder, tx(l, xSkillsOpenFolder), ac.OpenFolder == nil).Clicked() && ac.OpenFolder != nil {
			if err := ac.OpenFolder(sk.Dir); err != nil {
				s.notice, s.noticeErr = tx(l, xSkillsOpenFailed, err.Error()), true
			}
		}
		if IconButton(c, icSearch, tx(l, xSkillsReveal), ac.Reveal == nil).Clicked() && ac.Reveal != nil {
			ac.Reveal(sk.File)
		}
		if IconButton(c, icLink, tx(l, xSkillsCopyPath), ac.Copy == nil).Clicked() && ac.Copy != nil {
			ac.Copy(sk.File)
			s.notice, s.noticeErr = tx(l, xSkillsCopied), false
		}
		if !sk.Deletable {
			return
		}
		del := ac.Delete
		b := IconButton(c, icTrash, tx(l, xSkillsDelete), del == nil || s.work.Running)
		open := s.confirm == sk.ID
		if b.Clicked() && del != nil && !s.work.Running {
			s.confirm, open = sk.ID, true
		}
		msg := tx(l, xSkillsDeleteMessage, skillPath(sum, sk.Removal.Path))
		confirmPopover(c, l, b, &open, tx(l, xSkillsDeleteTitle), msg, tx(l, xSkillsDelete), func() { skillDeleteNotes(c, l, sk) }, func() {
			a.runSkillWork(acts, xSkillsDeleteFailed, func() (string, error) {
				system, err := del(sk, sum.Roots)
				return skillMovedText(l, system), err
			}, func(err error) {
				if err == nil {
					s.open, s.confirm, s.body = "", "", panelSkillBody{}
				}
			})
		})
		if !open && s.confirm == sk.ID {
			s.confirm = ""
		}
	})
	if !sk.Deletable {
		dimText(c, skillReadOnlyText(l, sk))
	}
}

// skillDeleteNotes are the consequences listed in the confirmation.
func skillDeleteNotes(c *ui.Context, l l10n.Localizer, sk skills.Skill) {
	if sk.Removal.IsLink {
		calloutDim(c, tx(l, xSkillsDeleteLink))
	}
	if n := len(sk.Removal.Covers); n > 1 {
		names := sk.Removal.Covers
		more := ""
		if n > 8 {
			names, more = names[:8], " …"
		}
		calloutDim(c, tx(l, xSkillsDeleteCovers, strings.Join(names, ", ")+more))
	}
	if len(sk.SharedWith) > 0 {
		ui.Text(c, tx(l, xSkillsDeleteShared, skillProviders(l, sk.SharedWith))).FontSize(fsCallout).TextColor(sys(c.Theme(), sysOrange))
	}
	calloutDim(c, tx(l, xSkillsDeleteRestart))
}

func skillReadOnlyText(l l10n.Localizer, sk skills.Skill) string {
	switch sk.Scope {
	case skills.ScopeSystem:
		return tx(l, xSkillsReadOnlySystem)
	case skills.ScopeBundled:
		return tx(l, xSkillsReadOnlyBundled)
	case skills.ScopeAdmin:
		return tx(l, xSkillsReadOnlyAdmin)
	case skills.ScopeSynced:
		return tx(l, xSkillsReadOnlySynced)
	case skills.ScopePlugin:
		switch sk.Provider {
		case skills.ProviderClaude:
			return tx(l, xSkillsReadOnlyPluginCL, sk.Plugin)
		case skills.ProviderGrok:
			return tx(l, xSkillsReadOnlyPluginGK)
		case skills.ProviderGemini:
			return tx(l, xSkillsReadOnlyPluginGM)
		}
		return tx(l, xSkillsReadOnlyPluginCX)
	}
	return tx(l, xSkillsBlockReadOnly)
}

// skillContent shows SKILL.md after the frontmatter on demand.
func (a *App) skillContent(c *ui.Context, l l10n.Localizer, sk skills.Skill, acts PanelActions) {
	s := &a.panelState.skills
	b := &s.body
	shown := b.id == sk.ID && b.shown
	label := tx(l, xSkillsShowContent)
	if shown {
		label = tx(l, xSkillsHideContent)
	}
	if disclosureRow(c, nil, label, false).Clicked() {
		if shown {
			b.shown = false
		} else {
			a.loadSkillBody(acts, sk)
		}
		return
	}
	if !shown {
		return
	}
	t := c.Theme()
	switch {
	case b.task.Running:
		sectionEmpty(c, l, true)
	case b.task.Err != nil:
		ui.Text(c, tx(l, xSkillsReadFailed, b.task.Err.Error())).FontSize(fsCaption).TextColor(sys(t, sysRed))
	case strings.TrimSpace(b.text) == "":
		calloutDim(c, tx(l, xSkillsNoContent))
	default:
		if b.truncated {
			dimText(c, tx(l, xSkillsTruncated))
		}
		s.md.WrapCode = true
		cardColumn(c, surfaceSunkenKind, radiusCard).Padding(10).Children(func() {
			markdown.View(c, b.text, &s.md)
		})
	}
}
