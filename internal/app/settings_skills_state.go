package app

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/markdown"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

// skillsTrashTab is the tab after the three providers on the skills page.
const skillsTrashTab = 3

// skillFlag narrows the list beyond the state; set flags combine.
type skillFlag int

const (
	skillFlagShared skillFlag = 1 << iota
	skillFlagLink
	skillFlagGitHub
	skillFlagIssues
	skillFlagDeletable
	skillFlagSwitchable
)

var skillFlags = []skillFlag{skillFlagShared, skillFlagLink, skillFlagGitHub, skillFlagIssues, skillFlagDeletable, skillFlagSwitchable}

type skillSort int

const (
	skillSortCLI skillSort = iota // grouped by location, in the CLI's order
	skillSortName
	skillSortModified
)

var skillSorts = []skillSort{skillSortCLI, skillSortName, skillSortModified}

// skillFilter is what the toolbar of a provider tab selects.
type skillFilter struct {
	search string
	state  skills.State // "" is every state
	source string       // a skillGroupOf key; "" is every location
	flags  skillFlag
	sort   skillSort
}

func (f skillFilter) narrowed() bool { return f.source != "" || f.flags != 0 }

// match reports whether sk passes everything but the state.
func (f skillFilter) match(sum skills.Summary, sk skills.Skill) bool {
	if !skillSearchMatches(sk, f.search) {
		return false
	}
	if f.source != "" {
		if key, _ := skillGroupKey(sk); key != f.source {
			return false
		}
	}
	for _, flag := range skillFlags {
		if f.flags&flag != 0 && !skillHasFlag(sum, sk, flag) {
			return false
		}
	}
	return true
}

func skillHasFlag(sum skills.Summary, sk skills.Skill, flag skillFlag) bool {
	switch flag {
	case skillFlagShared:
		return len(sk.SharedWith) > 0
	case skillFlagLink:
		return sk.LinkPath != ""
	case skillFlagGitHub:
		return sk.Origin.GitHub != ""
	case skillFlagIssues:
		return len(skillIssuesOf(sum, sk)) > 0
	case skillFlagDeletable:
		return sk.Deletable
	case skillFlagSwitchable:
		return skillSwitchable(sk)
	}
	return false
}

// skillSearchMatches is skillMatches plus the folder and the repository.
func skillSearchMatches(sk skills.Skill, search string) bool {
	if skillMatches(sk, search) {
		return true
	}
	q := strings.ToLower(strings.TrimSpace(search))
	for _, v := range []string{sk.Name, sk.File, sk.Origin.GitHub, sk.Origin.Repository} {
		if v != "" && strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

func skillGroupKey(sk skills.Skill) (string, bool) {
	if sk.Scope == skills.ScopeUser {
		return "user\x00" + sk.Root, true
	}
	return string(sk.Scope), false
}

// skillStateCounts counts the skills passing f in each state; "" is all.
func skillStateCounts(sum skills.Summary, f skillFilter) map[skills.State]int {
	n := map[skills.State]int{}
	for _, sk := range sum.Skills {
		if f.match(sum, sk) {
			n[""]++
			n[sk.State]++
		}
	}
	return n
}

// filterSkills applies f and its order to a summary.
func filterSkills(sum skills.Summary, f skillFilter) []skills.Skill {
	var out []skills.Skill
	for _, sk := range sum.Skills {
		if (f.state == "" || sk.State == f.state) && f.match(sum, sk) {
			out = append(out, sk)
		}
	}
	switch f.sort {
	case skillSortName:
		slices.SortStableFunc(out, func(a, b skills.Skill) int {
			return cmp.Compare(strings.ToLower(skillTitle(a)), strings.ToLower(skillTitle(b)))
		})
	case skillSortModified:
		slices.SortStableFunc(out, func(a, b skills.Skill) int { return b.ModTime.Compare(a.ModTime) })
	}
	return out
}

// skillSource is one location of the source filter.
type skillSource struct {
	key, title string
	count      int
}

func skillSources(l l10n.Localizer, sum skills.Summary) []skillSource {
	var out []skillSource
	at := map[string]int{}
	for _, sk := range sum.Skills {
		key, title := skillGroupOf(l, sk)
		i, ok := at[key]
		if !ok {
			i = len(out)
			at[key] = i
			out = append(out, skillSource{key: key, title: title})
		}
		out[i].count++
	}
	return out
}

// skillIssuesOf are the diagnostics that name this skill's files.
func skillIssuesOf(sum skills.Summary, sk skills.Skill) []skills.Issue {
	var out []skills.Issue
	for _, is := range sum.Issues {
		switch {
		case is.Path != "" && (is.Path == sk.File || is.Path == sk.LinkPath || is.Path == sk.Dir):
		case is.Kind == skills.IssueDuplicateName && is.Name != "" && is.Name == sk.InvokeName:
		default:
			continue
		}
		out = append(out, is)
	}
	return out
}

// trashMatches filters the trash by provider (0 is all) and text.
func trashMatches(e skills.TrashEntry, provider int, search string) bool {
	if provider > 0 && provider <= len(store.Providers()) {
		p := skills.Provider(store.Providers()[provider-1])
		if e.Provider != p && !slices.Contains(e.SharedWith, p) {
			return false
		}
	}
	q := strings.ToLower(strings.TrimSpace(search))
	if q == "" {
		return true
	}
	for _, v := range []string{e.Name, e.InvokeName, e.DisplayName, e.Description, e.Original, e.Origin.GitHub} {
		if strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

type skillDetailState struct {
	id      string
	task    Task
	detail  skills.Detail
	allRows bool
}

type skillTrashState struct {
	task     Task
	cancel   context.CancelFunc
	gen      uint64
	list     []skills.TrashEntry
	loaded   bool
	search   string
	provider int
	confirm  string
}

// shown is the trash under its provider and search filters.
func (tr *skillTrashState) shown() []skills.TrashEntry {
	var list []skills.TrashEntry
	for _, e := range tr.list {
		if trashMatches(e, tr.provider, tr.search) {
			list = append(list, e)
		}
	}
	return list
}

// settingsSkills is the state of the settings skills page.
type settingsSkills struct {
	tab    int // index into store.Providers(), or skillsTrashTab
	filter [skillsTrashTab]skillFilter
	open   string // ID of the skill page; "" shows the list
	// confirm is the delete confirmation that is open.
	confirm string
	// transfer is the move confirmation opened from an action menu.
	transferID     string
	transferTarget skills.Provider

	task       Task
	cancel     context.CancelFunc
	generation uint64
	result     map[store.Provider]skills.Summary
	at         time.Time

	work      Task
	notice    string
	noticeErr bool
	// pending holds a switch's new value until the next scan shows it.
	pending    map[string]bool
	showIssues bool
	// fixChoice and fixDesc are the open fix confirmation's choices: which
	// of the issue's fixes, and the description to write.
	fixChoice int
	fixDesc   string
	// selected are the IDs of the skills checked in the list.
	selected map[string]bool

	body   panelSkillBody
	md     markdown.Options
	detail skillDetailState
	trash  skillTrashState
	// add is the add page while it is open, or its confirmed write.
	add    *skillAddWizard
	update skillUpdateState
}

func (u *settingsUI) skillsState() *settingsSkills {
	if u.skills == nil {
		u.skills = &settingsSkills{}
		if u.opts.skillAddSession != nil {
			u.skills.add = u.opts.skillAddSession.wizard
		}
	}
	return u.skills
}

func (u *settingsUI) skillActions() SkillActions {
	if u.opts.Skills == nil {
		return SkillActions{}
	}
	return u.opts.Skills()
}

func (s *settingsSkills) provider() store.Provider {
	ps := store.Providers()
	if s.tab >= 0 && s.tab < len(ps) {
		return ps[s.tab]
	}
	return ps[0]
}

// selectProvider shows a provider's tab with its list.
func (s *settingsSkills) selectProvider(p store.Provider) {
	if i := slices.Index(store.Providers(), p); i >= 0 {
		s.tab = i
	}
	s.closeItem()
}

func (s *settingsSkills) closeItem() {
	s.open, s.confirm, s.notice, s.noticeErr = "", "", "", false
	s.transferID, s.transferTarget = "", ""
	s.body, s.detail = panelSkillBody{}, skillDetailState{}
	s.update.stop()
}

func (s *settingsSkills) find(id string) (skills.Summary, skills.Skill, bool) {
	sum := s.result[s.provider()]
	for _, sk := range sum.Skills {
		if sk.ID == id {
			return sum, sk, true
		}
	}
	return sum, skills.Skill{}, false
}

func (s *settingsSkills) cancelTasks() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.trash.cancel != nil {
		s.trash.cancel()
		s.trash.cancel = nil
	}
	s.generation++
	s.trash.gen++
}

// syncSkills scans when nothing was scanned yet or the scan is stale, and
// reads the trash once.
func (u *settingsUI) syncSkills(acts SkillActions) {
	s := u.skillsState()
	if !s.trash.loaded && !s.trash.task.Running && s.trash.task.Err == nil {
		u.loadSkillTrash(acts)
	}
	if s.task.Running || s.work.Running || s.result == nil && s.task.Err != nil {
		return
	}
	if s.result == nil || time.Since(s.at) > skillsStale {
		u.startSkillsScan(acts)
	}
}

func (u *settingsUI) startSkillsScan(acts SkillActions) {
	s := u.skillsState()
	if s.cancel != nil {
		s.cancel()
	}
	s.generation++
	gen := s.generation
	if acts.Query == nil {
		s.task = Task{Err: errDetailUnavailable}
		return
	}
	s.task = Task{Running: true}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	query := acts.Query
	go func() {
		res, err := query(ctx)
		u.post(func() {
			if ctx.Err() != nil || s.generation != gen {
				return
			}
			s.cancel = nil
			s.task = Task{Err: err}
			if err == nil {
				s.result, s.at, s.pending = res, time.Now(), nil
			}
		})
	}()
}

func (u *settingsUI) loadSkillTrash(acts SkillActions) {
	tr := &u.skillsState().trash
	if tr.cancel != nil {
		tr.cancel()
	}
	tr.gen++
	gen := tr.gen
	if acts.ListTrash == nil {
		tr.task, tr.loaded = Task{}, true
		return
	}
	tr.task = Task{Running: true}
	ctx, cancel := context.WithCancel(context.Background())
	tr.cancel = cancel
	list := acts.ListTrash
	go func() {
		got, err := list(ctx)
		u.post(func() {
			if ctx.Err() != nil || tr.gen != gen {
				return
			}
			tr.cancel = nil
			tr.task = Task{Err: err}
			if err == nil {
				tr.list, tr.loaded = got, true
			}
		})
	}()
}

// skillWork runs a change, shows its outcome, then reads the skills and
// the trash again.
func (u *settingsUI) skillWork(acts SkillActions, fn func() (string, error), fail textKey, after func(error)) {
	s := u.skillsState()
	var ok string
	runTask(&s.work, u.post, func() error {
		var err error
		ok, err = fn()
		return err
	}, func(err error) {
		l := u.L()
		switch {
		case err == nil:
			s.notice, s.noticeErr = ok, false
		case errors.Is(err, skills.ErrChanged) && !errors.Is(err, skills.ErrTransferPartial):
			s.notice, s.noticeErr = tx(l, xSkillsChanged), true
		default:
			s.notice, s.noticeErr = tx(l, fail, skillErrorText(l, err)), true
		}
		if after != nil {
			after(err)
		}
		u.startSkillsScan(acts)
		u.loadSkillTrash(acts)
	})
}

// skillErrorText names the trash errors a user can act on.
func skillErrorText(l l10n.Localizer, err error) string {
	var partial *skillTransferPartialError
	var imported *skills.ImportError
	switch {
	case errors.As(err, &imported):
		return skillAddErrorText(l, err)
	case errors.As(err, &partial):
		return tx(l, xSkillsTransferPartial, partial.path)
	case errors.Is(err, skills.ErrNotFixable):
		return tx(l, xSkillsFixStale)
	case errors.Is(err, skills.ErrTrashConflict):
		return tx(l, xSkillsRestoreConflict)
	case errors.Is(err, skills.ErrTrashParentGone):
		return tx(l, xSkillsRestoreParent)
	case errors.Is(err, skills.ErrTrashEntry):
		return tx(l, xSkillsTrashDamaged)
	case errors.Is(err, skills.ErrTransferProvider):
		return tx(l, xSkillsTransferProviderError)
	case errors.Is(err, skills.ErrTransferConflict):
		return tx(l, xSkillsTransferConflict)
	case errors.Is(err, skills.ErrTransferSource):
		return tx(l, xSkillsTransferSource)
	case errors.Is(err, skills.ErrTransferOverlap):
		return tx(l, xSkillsTransferOverlap)
	case errors.Is(err, skills.ErrTransferUnsafeLink):
		return tx(l, xSkillsTransferUnsafeLink)
	case errors.Is(err, skills.ErrTransferNotMovable):
		return tx(l, xSkillsTransferNotMovable)
	case errors.Is(err, skills.ErrTransferPartial):
		return tx(l, xSkillsTransferPartialUnknown)
	}
	return err.Error()
}

func (u *settingsUI) loadSkillPageDetail(acts SkillActions, sk skills.Skill) {
	d := &u.skillsState().detail
	if d.id == sk.ID {
		return
	}
	*d = skillDetailState{id: sk.ID}
	if acts.Inspect == nil {
		d.task.Err = errDetailUnavailable
		return
	}
	var got skills.Detail
	id, inspect := sk.ID, acts.Inspect
	runTask(&d.task, u.post, func() error {
		var err error
		got, err = inspect(context.Background(), sk)
		return err
	}, func(error) {
		if d.id == id {
			d.detail = got
		}
	})
}

func (u *settingsUI) loadSettingsSkillBody(acts SkillActions, sk skills.Skill) {
	b := &u.skillsState().body
	if b.id == sk.ID && (b.task.Running || b.text != "") {
		b.shown = true
		return
	}
	*b = panelSkillBody{id: sk.ID, shown: true}
	if acts.ReadBody == nil {
		b.task.Err = errDetailUnavailable
		return
	}
	var text string
	var cut bool
	id, read := sk.ID, acts.ReadBody
	runTask(&b.task, u.post, func() error {
		var err error
		text, cut, err = read(sk)
		return err
	}, func(error) {
		if b.id == id {
			b.text, b.truncated = text, cut
		}
	})
}
