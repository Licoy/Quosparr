package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

// The steps of the add page; results follow the confirmed write.
const (
	skillAddStepSource = iota
	skillAddStepPick
	skillAddStepReview
	skillAddStepResults
)

// skillAddKinds are the source tabs, in their order.
var skillAddKinds = []skills.SourceKind{skills.SourceGitHub, skills.SourceFolder, skills.SourceZip}

var errSkillAddSource = errors.New("skills: no source given")

type skillAddSession struct {
	wizard *skillAddWizard
	owner  *settingsUI
}

type skillAddTarget struct {
	target skills.InstallTarget
	plan   skills.InstallPlan
	result skills.InstallResult
	err    error
	done   bool
}

type skillAddWizard struct {
	open bool
	step int
	kind int // index into skillAddKinds
	// What was typed or chosen for each kind of source.
	github, folder, zip string
	// snap is the source read last, found its skills; picked and
	// expanded are keyed by a skill's folder in the source.
	snap     *skills.Snapshot
	found    []skills.Candidate
	picked   map[string]bool
	expanded map[string]bool
	load     Task
	loadStop context.CancelFunc
	loadGen  uint64
	// loadDone of loadTotal bytes are unpacked from a GitHub download.
	loadDone, loadTotal int64

	targets    map[skills.InstallTarget]bool
	plans      []skillAddTarget
	generation uint64
	cancel     context.CancelFunc
	task       Task
	err        error
	writing    bool
	completed  bool
}

func (u *settingsUI) openSkillAdd() {
	s := u.skillsState()
	if s.add != nil && (s.add.writing || s.add.completed) {
		s.add.open = true
		return
	}
	target := skills.InstallTarget(s.provider())
	s.add = &skillAddWizard{open: true, targets: map[skills.InstallTarget]bool{target: true}}
	if u.opts.skillAddSession != nil {
		u.opts.skillAddSession.wizard = s.add
	}
}

// closeSkillAdd leaves the page. A write that was confirmed keeps its
// results for the next visit; anything else read is dropped.
func (u *settingsUI) closeSkillAdd() {
	if u.skills == nil || u.skills.add == nil || !u.skills.add.open {
		return
	}
	w := u.skills.add
	w.open = false
	w.stopLoad()
	w.stopPrepare()
	if w.writing || w.completed {
		return
	}
	u.skills.add = nil
	if u.opts.skillAddSession != nil {
		u.opts.skillAddSession.wizard = nil
	}
}

func (w *skillAddWizard) stopLoad() {
	if w.loadStop != nil {
		w.loadStop()
		w.loadStop = nil
	}
	w.loadGen++
	w.load = Task{}
	w.loadDone, w.loadTotal = 0, 0
}

func (w *skillAddWizard) stopPrepare() {
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.generation++
	w.task = Task{}
}

func (w *skillAddWizard) sourceKind() skills.SourceKind { return skillAddKinds[w.kind] }

// input is what the current tab holds.
func (w *skillAddWizard) input() *string {
	switch w.sourceKind() {
	case skills.SourceFolder:
		return &w.folder
	case skills.SourceZip:
		return &w.zip
	}
	return &w.github
}

// source is the current tab's source, or why it is not one.
func (w *skillAddWizard) source() (skills.Source, error) {
	raw := strings.TrimSpace(*w.input())
	if raw == "" {
		return skills.Source{}, errSkillAddSource
	}
	if w.sourceKind() == skills.SourceGitHub {
		return skills.ParseGitHubSource(raw)
	}
	return skills.Source{Kind: w.sourceKind(), Path: raw}, nil
}

// sourceChanged drops what was read once the source differs from it.
func (w *skillAddWizard) sourceChanged() {
	w.stopLoad()
	w.snap, w.found, w.picked, w.expanded = nil, nil, nil, nil
	w.invalidate()
}

// invalidate drops the checks of the targets after the picks changed.
func (w *skillAddWizard) invalidate() {
	w.stopPrepare()
	w.plans, w.err = nil, nil
}

// loadSource reads the source and moves on to its skills; a source read
// before moves on at once.
func (u *settingsUI) loadSkillSource(acts SkillActions) {
	w := u.skills.add
	if w.load.Running {
		return
	}
	if w.snap != nil {
		w.step = skillAddStepPick
		return
	}
	src, err := w.source()
	if err != nil {
		w.load = Task{Err: err}
		return
	}
	if acts.LoadImport == nil {
		w.load = Task{Err: errSkillsUnavailable}
		return
	}
	w.stopLoad()
	ctx, cancel := context.WithCancel(context.Background())
	w.loadStop = cancel
	w.load = Task{Running: true}
	gen, load := w.loadGen, acts.LoadImport
	progress := func(done, total int64) {
		u.post(func() {
			if gen == w.loadGen && w.load.Running {
				w.loadDone, w.loadTotal = done, total
			}
		})
	}
	go func() {
		snap, found, err := load(ctx, src, progress)
		u.post(func() {
			if ctx.Err() != nil || gen != w.loadGen || !w.open {
				return
			}
			w.loadStop = nil
			w.load = Task{Err: err}
			if err != nil {
				return
			}
			w.snap, w.found = snap, found
			w.picked, w.expanded = map[string]bool{}, map[string]bool{}
			if usable := skillAddUsable(found); len(usable) == 1 {
				w.picked[usable[0].Folder] = true
			}
			w.step = skillAddStepPick
		})
	}()
}

func skillAddUsable(found []skills.Candidate) []skills.Candidate {
	var out []skills.Candidate
	for _, c := range found {
		if c.Problem == skills.ProblemNone {
			out = append(out, c)
		}
	}
	return out
}

// pickedSkills are the checked skills in the order they were found.
func (w *skillAddWizard) pickedSkills() []skills.Candidate {
	var out []skills.Candidate
	for _, c := range w.found {
		if w.picked[c.Folder] && c.Problem == skills.ProblemNone {
			out = append(out, c)
		}
	}
	return out
}

func (w *skillAddWizard) setPicked(c skills.Candidate, on bool) {
	if c.Problem != skills.ProblemNone {
		return
	}
	w.picked[c.Folder] = on
	w.invalidate()
}

// prepareSkillInstall checks the chosen folders for the picked skills,
// without writing; a retry checks again only the folders whose write
// failed and keeps the others' results.
func (u *settingsUI) prepareSkillInstall(acts SkillActions, retry bool) {
	w := u.skills.add
	if w.writing || acts.PrepareInstall == nil || w.snap == nil {
		return
	}
	w.stopPrepare()
	picked := w.pickedSkills()
	var targets []skills.InstallTarget
	for _, t := range skills.InstallTargets() {
		if retry && w.plan(t) != nil && !w.plan(t).done || !retry && w.targets[t] {
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 || len(picked) == 0 {
		w.plans, w.err = nil, nil
		return
	}
	var sums []skills.Summary
	for _, p := range store.Providers() {
		if sum, ok := u.skills.result[p]; ok {
			sums = append(sums, sum)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.task, w.err = Task{Running: true}, nil
	gen, snap, prepare := w.generation, w.snap, acts.PrepareInstall
	go func() {
		plans := make([]skillAddTarget, 0, len(targets))
		for _, t := range targets {
			plan, err := prepare(t, sums, snap, picked)
			plans = append(plans, skillAddTarget{target: t, plan: plan, err: err})
		}
		u.post(func() {
			if ctx.Err() != nil || gen != w.generation || !w.open {
				return
			}
			w.cancel = nil
			w.task = Task{}
			if retry {
				for _, old := range w.plans {
					if old.done {
						plans = append(plans, old)
					}
				}
				slices.SortFunc(plans, func(a, b skillAddTarget) int {
					return slices.Index(skills.InstallTargets(), a.target) - slices.Index(skills.InstallTargets(), b.target)
				})
			}
			w.plans = plans
		})
	}()
}

// plan is the target's prepared or finished check, if any.
func (w *skillAddWizard) plan(t skills.InstallTarget) *skillAddTarget {
	for i := range w.plans {
		if w.plans[i].target == t {
			return &w.plans[i]
		}
	}
	return nil
}

// pendingAdds counts the skills the confirmed write would add.
func (w *skillAddWizard) pendingAdds() int {
	n := 0
	for _, p := range w.plans {
		if p.done || p.err != nil {
			continue
		}
		for _, item := range p.plan.Items {
			if item.Status == skills.InstallAdd {
				n++
			}
		}
	}
	return n
}

func (w *skillAddWizard) blocked() bool {
	return w.task.Running || w.writing || w.pendingAdds() == 0 ||
		slices.ContainsFunc(w.plans, func(p skillAddTarget) bool { return p.err != nil && !p.done })
}

// failed reports a target that could not be written, wholly or in part.
func (p skillAddTarget) failed() bool {
	return p.err != nil || slices.ContainsFunc(p.result.Items, func(o skills.InstallOutcome) bool { return o.Err != nil })
}

func skillAddFailed(plans []skillAddTarget) bool {
	return slices.ContainsFunc(plans, skillAddTarget.failed)
}

func (u *settingsUI) submitSkillInstall(acts SkillActions) {
	w := u.skills.add
	if w.blocked() || acts.Install == nil {
		return
	}
	w.writing = true
	w.step = skillAddStepResults
	plans := slices.Clone(w.plans)
	install := acts.Install
	go func() {
		for i := range plans {
			if plans[i].done || plans[i].err != nil {
				continue
			}
			plans[i].result, plans[i].err = install(context.Background(), plans[i].plan)
			plans[i].done = !plans[i].failed()
		}
		u.post(func() {
			w.plans = plans
			w.writing, w.completed = false, true
			owner := u.currentSkillsUI()
			owner.startSkillsScan(acts)
			if owner.win != nil && !owner.win.IsDestroyed() {
				owner.win.Update(func() {})
			}
		})
	}()
}

// finishSkillAdd closes the results and drops what was read.
func (u *settingsUI) finishSkillAdd() {
	w := u.skills.add
	w.completed = false
	u.closeSkillAdd()
}

// Called only from the main thread; a newly opened window takes over the
// results of a write confirmed in a closed one.
func (u *settingsUI) currentSkillsUI() *settingsUI {
	if s := u.opts.skillAddSession; s != nil && s.owner != nil {
		return s.owner
	}
	return u
}
