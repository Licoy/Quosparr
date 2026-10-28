package app

import (
	"context"
	"maps"
	"slices"

	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
)

type mcpAddSession struct {
	wizard *mcpAddWizard
	owner  *settingsUI
}

type mcpPairRow struct {
	id  int
	key string
}

type mcpDraftEntry struct {
	rows             map[string][]*mcpPairRow
	nextRow          int
	pendingTransport string
	confirmTransport bool
	draft            mcp.Draft
	selected         bool
	version          uint64
	probe            mcp.ProbeResult
	probed           bool
	task             Task
	cancel           context.CancelFunc
	search           string
	expanded         map[int]bool
}
type mcpAddTarget struct {
	provider store.Provider
	plan     mcp.AddPlan
	result   mcp.AddResult
	err      error
	done     bool
}
type mcpAddWizard struct {
	open          bool
	step          int
	entries       []*mcpDraftEntry
	edit          int
	paste         string
	importJSON    bool
	replace       bool
	showSecrets   bool
	unverified    bool
	targets       map[store.Provider]bool
	plans         []mcpAddTarget
	generation    uint64
	cancel        context.CancelFunc
	task          Task
	err           error
	writing       bool
	completed     bool
	wasUnverified bool
}

func (u *settingsUI) openMCPAdd() {
	s := u.mcpState()
	if s.add != nil && (s.add.writing || s.add.completed) {
		s.add.open = true
		return
	}
	s.add = &mcpAddWizard{open: true, replace: true, targets: map[store.Provider]bool{s.provider: true}, entries: []*mcpDraftEntry{newMCPDraft(mcp.Draft{Transport: "http"})}}
	if u.opts.mcpAddSession != nil {
		u.opts.mcpAddSession.wizard = s.add
	}
}
func newMCPDraft(d mcp.Draft) *mcpDraftEntry {
	return &mcpDraftEntry{draft: d, selected: true, expanded: map[int]bool{}}
}
func (u *settingsUI) closeMCPAdd() {
	if u.mcp == nil || u.mcp.add == nil || !u.mcp.add.open {
		return
	}
	w := u.mcp.add
	w.open = false
	w.generation++
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	for _, e := range w.entries {
		if e.cancel != nil {
			e.cancel()
		}
	}
	// Confirmed writes retain only their plans until completion; editable secrets
	// are removed as soon as the user leaves the page.
	w.entries = nil
	w.paste = ""
	if !w.writing && !w.completed {
		u.mcp.add = nil
		if u.opts.mcpAddSession != nil {
			u.opts.mcpAddSession.wizard = nil
		}
	}
}
func (w *mcpAddWizard) invalidate(e *mcpDraftEntry) {
	if e != nil {
		if e.cancel != nil {
			e.cancel()
			e.cancel = nil
		}
		e.version++
		e.probed = false
		e.probe = mcp.ProbeResult{}
		e.task = Task{}
	}
	w.generation++
	w.plans = nil
	w.unverified = false
	w.err = nil
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.task = Task{}
}
func (w *mcpAddWizard) drafts() []mcp.Draft {
	var out []mcp.Draft
	for _, e := range w.entries {
		if e.selected {
			d := e.draft
			d.Args = slices.Clone(d.Args)
			d.Env = maps.Clone(d.Env)
			d.Headers = maps.Clone(d.Headers)
			d.EnvHeaders = maps.Clone(d.EnvHeaders)
			out = append(out, d)
		}
	}
	return out
}
func (w *mcpAddWizard) needsVerification() bool {
	for _, e := range w.entries {
		if e.selected && (!e.probed || e.task.Err != nil || e.probe.Truncated) {
			return true
		}
	}
	return false
}
func (w *mcpAddWizard) importDrafts(actions MCPActions) {
	if actions.Parse == nil {
		w.err = errMCPUnavailable
		return
	}
	ds, err := actions.Parse([]byte(w.paste))
	w.err = err
	if err != nil {
		return
	}
	w.invalidate(nil)
	if !w.replace && len(w.entries) == 1 && mcpBlankDraft(w.entries[0].draft) {
		w.replace = true
	}
	if w.replace {
		for _, e := range w.entries {
			if e.cancel != nil {
				e.cancel()
			}
		}
		w.entries = nil
	}
	for _, d := range ds {
		w.entries = append(w.entries, newMCPDraft(d))
	}
	w.edit = len(w.entries) - len(ds)
	w.paste = ""
	w.importJSON = false
}
func (u *settingsUI) probeMCPDraft(actions MCPActions, e *mcpDraftEntry) {
	if actions.Probe == nil || e.task.Running {
		return
	}
	w := u.mcp.add
	w.invalidate(e)
	version := e.version
	draft := e.draft
	draft.Args = slices.Clone(draft.Args)
	draft.Env = maps.Clone(draft.Env)
	draft.Headers = maps.Clone(draft.Headers)
	draft.EnvHeaders = maps.Clone(draft.EnvHeaders)
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.task = Task{Running: true}
	go func() {
		result, err := actions.Probe(ctx, draft)
		u.post(func() {
			if ctx.Err() != nil || version != e.version || !w.open {
				return
			}
			e.cancel = nil
			e.task = Task{Err: err}
			e.probe = result
			e.probed = err == nil
		})
	}()
}
func (u *settingsUI) prepareMCPDrafts(actions MCPActions, retry bool) {
	w := u.mcp.add
	if w.task.Running || w.writing || actions.PrepareAdd == nil {
		return
	}
	ds := w.drafts()
	if !retry {
		if len(ds) == 0 {
			w.err = &mcp.DraftError{Code: "size", Field: "drafts"}
			return
		}
		for _, d := range ds {
			if err := mcp.ValidateDraft(d); err != nil {
				w.err = err
				return
			}
		}
		if w.needsVerification() && !w.unverified {
			return
		}
	}
	var targets []store.Provider
	for _, p := range store.Providers() {
		if w.targets[p] {
			targets = append(targets, p)
		}
	}
	if retry {
		targets = nil
		for _, p := range w.plans {
			if !p.done {
				targets = append(targets, p.provider)
			}
		}
	}
	if len(targets) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.task = Task{Running: true}
	w.err = nil
	gen := w.generation
	go func() {
		plans := make([]mcpAddTarget, 0, len(targets))
		for _, p := range targets {
			plan, err := actions.PrepareAdd(ctx, p, ds)
			plans = append(plans, mcpAddTarget{provider: p, plan: plan, err: err})
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
			}
			w.plans = plans
			for _, p := range plans {
				if p.err != nil {
					w.err = p.err
				}
			}
			if w.err == nil {
				w.step = 3
			}
		})
	}()
}

func mcpBlankDraft(d mcp.Draft) bool {
	return d.Name == "" && d.Command == "" && d.URL == "" && d.Cwd == "" && len(d.Args) == 0 && len(d.Env) == 0 && len(d.Headers) == 0 && len(d.EnvHeaders) == 0 && d.BearerTokenEnv == ""
}

// Called only from the main thread; a newly opened window takes ownership of
// the shared outcome without retaining an old window's controls.
func (u *settingsUI) currentMCPUI() *settingsUI {
	if s := u.opts.mcpAddSession; s != nil && s.owner != nil {
		return s.owner
	}
	return u
}
