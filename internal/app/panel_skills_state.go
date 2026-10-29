package app

import (
	"context"
	"errors"
	"time"

	"github.com/egoist/mygo/ui/markdown"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

// SkillActions read and change the global skills of the three CLIs. A nil
// func disables that control.
type SkillActions struct {
	// Query scans every provider, so a copy shared between CLIs is known.
	Query      func(context.Context) (map[store.Provider]skills.Summary, error)
	ReadBody   func(skills.Skill) (body string, truncated bool, err error)
	SetEnabled func(skills.Skill, bool) error
	// Delete and DeleteLink move to the Quosparr skills trash, or to the
	// system Trash from another volume (system is true); nothing is erased.
	Delete     func(skills.Skill, []skills.RootInfo) (system bool, err error)
	DeleteLink func(skills.Summary, string) (system bool, err error)
	Transfer   func(skills.TransferRequest) (skills.TransferResult, error)
	// The diagnostics fixes: lifting a nested Claude skill moves its outer
	// folder to the skills trash; a description is written into SKILL.md
	// after a backup.
	LiftNested         func(skills.Summary, skills.Skill) (skills.LiftResult, error)
	SuggestDescription func(skills.Skill) string
	AddDescription     func(skills.Skill, string) error
	OpenFolder         func(string) error
	Reveal             func(string)
	Copy               func(string)
	// Inspect lists a skill's folder for its page in the settings.
	Inspect func(context.Context, skills.Skill) (skills.Detail, error)
	// The trash: restoring renames an entry back, purging and emptying
	// move entries to the system Trash.
	TrashDir   string
	ListTrash  func(context.Context) ([]skills.TrashEntry, error)
	Restore    func(id string) (skills.TrashEntry, error)
	Purge      func(id string) error
	EmptyTrash func() (int, error)
	// Adding skills: a GitHub repository, folder or zip is read into
	// memory and searched for skills; the picked ones are checked against
	// each chosen folder, then written into the folders that lack them.
	// progress hears how far a GitHub download got, total 0 when unknown.
	LoadImport func(ctx context.Context, src skills.Source, progress func(done, total int64)) (*skills.Snapshot, []skills.Candidate, error)
	// PickPath asks for a folder, or for a zip file when folder is false;
	// "" is a cancelled dialog.
	PickPath       func(title string, folder bool) (string, error)
	PrepareInstall func(skills.InstallTarget, []skills.Summary, *skills.Snapshot, []skills.Candidate) (skills.InstallPlan, error)
	Install        func(context.Context, skills.InstallPlan) (skills.InstallResult, error)
	// A skill added from GitHub can be checked against and replaced by
	// its repository's latest version; the old one goes to the trash.
	CheckUpdate func(context.Context, skills.Skill) skills.UpdateCheck
	Update      func(context.Context, skills.UpdateRequest) (skills.UpdateResult, error)
}

// skillsStale is how old a scan may be before the panel scans again.
const skillsStale = 30 * time.Second

type panelSkillsState struct {
	search     string
	open       string // ID of the skill page; "" shows the list
	confirm    string // ID of the open delete confirmation
	task       Task   // the scan
	cancel     context.CancelFunc
	generation uint64
	result     map[store.Provider]skills.Summary
	at         time.Time
	work       Task // a switch, delete or folder action
	notice     string
	noticeErr  bool
	enabled    bool // the page's switch, synced from the skill
	body       panelSkillBody
	md         markdown.Options
	scrolled   string // the page detailScroll belongs to
}

type panelSkillBody struct {
	id        string
	shown     bool
	task      Task
	text      string
	truncated bool
}

// closeItem leaves the skill page for the list.
func (s *panelSkillsState) closeItem() {
	s.open, s.confirm, s.notice, s.noticeErr = "", "", "", false
	s.body = panelSkillBody{}
}

func (s *panelSkillsState) summary(p store.Provider) (skills.Summary, bool) {
	sum, ok := s.result[p]
	return sum, ok
}

func (s *panelSkillsState) find(p store.Provider, id string) (skills.Skill, bool) {
	sum, _ := s.summary(p)
	for _, sk := range sum.Skills {
		if sk.ID == id {
			return sk, true
		}
	}
	return skills.Skill{}, false
}

// syncSkills scans when nothing was scanned yet or the last scan is stale.
func (a *App) syncSkills(acts PanelActions) {
	s := &a.panelState.skills
	if s.task.Running || s.work.Running {
		return
	}
	if s.result == nil && s.task.Err != nil {
		return // wait for the refresh button after a failure
	}
	if s.result == nil || time.Since(s.at) > skillsStale {
		a.startSkillsQuery(acts)
	}
}

func (a *App) startSkillsQuery(acts PanelActions) {
	s := &a.panelState.skills
	if s.cancel != nil {
		s.cancel()
	}
	s.generation++
	gen := s.generation
	query := acts.Skills.Query
	if query == nil {
		s.task = Task{Err: errDetailUnavailable}
		return
	}
	s.task = Task{Running: true}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		res, err := query(ctx)
		runOnMain(func() {
			if ctx.Err() != nil || s.generation != gen {
				return
			}
			s.cancel = nil
			s.task = Task{Err: err}
			if err == nil {
				s.result, s.at = res, time.Now()
			}
			a.redrawPanel()
		})
	}()
}

// runSkillWork runs a change, shows its outcome and scans again.
// fn returns the text shown on success.
func (a *App) runSkillWork(acts PanelActions, fail textKey, fn func() (string, error), after func(error)) {
	s := &a.panelState.skills
	l := a.L()
	var ok string
	run := func() error {
		var err error
		ok, err = fn()
		return err
	}
	runTask(&s.work, func(f func()) { runOnMain(func() { f(); a.redrawPanel() }) }, run, func(err error) {
		switch {
		case err == nil:
			s.notice, s.noticeErr = ok, false
		case errors.Is(err, skills.ErrChanged):
			s.notice, s.noticeErr = tx(l, xSkillsChanged), true
		default:
			s.notice, s.noticeErr = tx(l, fail, err.Error()), true
		}
		if after != nil {
			after(err)
		}
		a.startSkillsQuery(acts)
	})
}

func (a *App) loadSkillBody(acts PanelActions, sk skills.Skill) {
	b := &a.panelState.skills.body
	if b.id == sk.ID && (b.task.Running || b.text != "") {
		b.shown = true
		return
	}
	*b = panelSkillBody{id: sk.ID, shown: true}
	read := acts.Skills.ReadBody
	if read == nil {
		b.task.Err = errDetailUnavailable
		return
	}
	var text string
	var cut bool
	id := sk.ID
	runTask(&b.task, func(f func()) { runOnMain(func() { f(); a.redrawPanel() }) }, func() error {
		var err error
		text, cut, err = read(sk)
		return err
	}, func(err error) {
		if b.id != id {
			return
		}
		b.text, b.truncated = text, cut
	})
}
