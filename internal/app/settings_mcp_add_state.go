package app

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
)

// The steps of the add page; results follow the confirmed write.
const (
	mcpStepConfigure = iota
	mcpStepTools
	mcpStepReview
	mcpStepResults
)

// mcpTransports are the transports the form offers, in its order.
var mcpTransports = []string{"stdio", "http", "sse"}

type mcpAddSession struct {
	wizard *mcpAddWizard
	owner  *settingsUI
}

// mcpPair is one row of a name and value table. Rows keep their order and
// may be blank or repeat a name while typed; mcpDraftEntry.draft checks
// them. id keys the row's inputs, so removing a row above keeps them.
type mcpPair struct {
	id         int
	key, value string
}

type mcpArg struct {
	id    int
	value string
}

// mcpDraftEntry is the form of one server. It keeps the fields of both
// kinds of transport, so switching back restores what was typed; only the
// chosen transport's fields reach the draft.
type mcpDraftEntry struct {
	id                  int
	name, transport     string
	command, cwd        string
	args                []mcpArg
	env                 []mcpPair
	url, bearerEnv      string
	headers, envHeaders []mcpPair
	nextID              int
	advanced            bool
	selected            bool
	version             uint64
	probe               mcp.ProbeResult
	probed              bool
	task                Task
	cancel              context.CancelFunc
	search              string
	expanded            map[int]bool
}
type mcpAddTarget struct {
	provider store.Provider
	plan     mcp.AddPlan
	result   mcp.AddResult
	err      error
	done     bool
}
type mcpAddWizard struct {
	open    bool
	step    int
	entries []*mcpDraftEntry
	nextID  int
	// edit is the entry the form shows; jsonMode shows the JSON box in
	// its place, and imported counts the servers its last parse added.
	edit        int
	jsonMode    bool
	paste       string
	jsonErr     error
	imported    int
	replace     bool
	showSecrets bool
	// attempted turns on the field errors once Continue was pressed, so
	// they follow the typing from then on.
	attempted     bool
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
	s.add = &mcpAddWizard{open: true, targets: map[store.Provider]bool{s.provider: true}}
	s.add.addEntry(mcp.Draft{Transport: "stdio"})
	if u.opts.mcpAddSession != nil {
		u.opts.mcpAddSession.wizard = s.add
	}
}

// addEntry appends a server to the form and shows it.
func (w *mcpAddWizard) addEntry(d mcp.Draft) *mcpDraftEntry {
	w.nextID++
	e := newMCPDraft(d)
	e.id = w.nextID
	w.entries = append(w.entries, e)
	w.edit = len(w.entries) - 1
	return e
}

// removeEntry drops a server; the last one gives way to an empty form.
func (w *mcpAddWizard) removeEntry(i int) {
	if e := w.entries[i]; e.cancel != nil {
		e.cancel()
	}
	w.entries = slices.Delete(w.entries, i, i+1)
	w.invalidate(nil)
	if len(w.entries) == 0 {
		w.addEntry(mcp.Draft{Transport: "stdio"})
	}
	if w.edit >= i && w.edit > 0 {
		w.edit--
	}
}

func newMCPDraft(d mcp.Draft) *mcpDraftEntry {
	e := &mcpDraftEntry{name: d.Name, transport: d.Transport, command: d.Command, cwd: d.Cwd, url: d.URL, bearerEnv: d.BearerTokenEnv,
		selected: true, expanded: map[int]bool{}}
	if !slices.Contains(mcpTransports, e.transport) {
		e.transport = "stdio"
	}
	for _, arg := range d.Args {
		e.args = append(e.args, mcpArg{id: e.newID(), value: arg})
	}
	e.env, e.headers, e.envHeaders = e.pairs(d.Env), e.pairs(d.Headers), e.pairs(d.EnvHeaders)
	e.advanced = d.Cwd != "" || d.BearerTokenEnv != "" || len(d.EnvHeaders) > 0
	return e
}

func (e *mcpDraftEntry) newID() int {
	e.nextID++
	return e.nextID
}

func (e *mcpDraftEntry) pairs(m map[string]string) []mcpPair {
	var rows []mcpPair
	for _, k := range slices.Sorted(maps.Keys(m)) {
		rows = append(rows, mcpPair{id: e.newID(), key: k, value: m[k]})
	}
	return rows
}

// draft is the server the form describes, or the first table row that
// cannot be part of one: a value without a name, or a name given twice.
// Names, the command and the URL lose surrounding spaces; values and
// arguments are kept as typed.
func (e *mcpDraftEntry) draft() (mcp.Draft, error) {
	d := mcp.Draft{Name: strings.TrimSpace(e.name), Transport: e.transport}
	var err error
	if e.transport == "stdio" {
		d.Command, d.Cwd = strings.TrimSpace(e.command), e.cwd
		for _, arg := range e.args {
			d.Args = append(d.Args, arg.value)
		}
		d.Env, err = mcpPairMap(e.env, "env")
		return d, err
	}
	d.URL, d.BearerTokenEnv = strings.TrimSpace(e.url), strings.TrimSpace(e.bearerEnv)
	if d.Headers, err = mcpPairMap(e.headers, "headers"); err != nil {
		return d, err
	}
	d.EnvHeaders, err = mcpPairMap(e.envHeaders, "env_headers")
	return d, err
}

func mcpPairMap(rows []mcpPair, field string) (map[string]string, error) {
	var m map[string]string
	for _, row := range rows {
		key := strings.TrimSpace(row.key)
		if key == "" {
			if row.value == "" {
				continue
			}
			return nil, &mcp.DraftError{Code: "invalid", Field: field}
		}
		if _, dup := m[key]; dup {
			return nil, &mcp.DraftError{Code: "duplicate", Field: field}
		}
		if m == nil {
			m = map[string]string{}
		}
		m[key] = row.value
	}
	return m, nil
}

func (e *mcpDraftEntry) blank() bool {
	d, err := e.draft()
	return err == nil && d.Name == "" && d.Command == "" && d.URL == "" && d.Cwd == "" && d.BearerTokenEnv == "" &&
		!slices.ContainsFunc(d.Args, func(a string) bool { return a != "" }) && len(d.Env) == 0 && len(d.Headers) == 0 && len(d.EnvHeaders) == 0
}

// entryError keeps an included server from being added: its own fields,
// or a name an earlier included server already has.
func (w *mcpAddWizard) entryError(e *mcpDraftEntry) error {
	d, err := e.draft()
	if err == nil {
		err = mcp.ValidateDraft(d)
	}
	if err != nil {
		return err
	}
	for _, other := range w.entries {
		if other == e {
			break
		}
		if other.selected && strings.TrimSpace(other.name) == d.Name {
			return &mcp.DraftError{Code: "duplicate", Field: "name"}
		}
	}
	return nil
}

// continueToTools leaves the form when every included server is valid, or
// shows the first one that is not, with its errors.
func (w *mcpAddWizard) continueToTools() {
	w.attempted = true
	for i, e := range w.entries {
		if !e.selected {
			continue
		}
		if err := w.entryError(e); err != nil {
			w.edit, w.jsonMode = i, false
			if d, ok := err.(*mcp.DraftError); ok && slices.Contains([]string{"cwd", "env_headers", "bearer_token_env"}, d.Field) {
				e.advanced = true
			}
			return
		}
	}
	if len(w.drafts()) > 0 {
		w.step, w.imported = mcpStepTools, 0
	}
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

// drafts are the included servers; Continue checked them.
func (w *mcpAddWizard) drafts() []mcp.Draft {
	var out []mcp.Draft
	for _, e := range w.entries {
		if e.selected {
			d, _ := e.draft()
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

// allBlank reports a form nobody has typed into yet, which a parse replaces.
func (w *mcpAddWizard) allBlank() bool {
	for _, e := range w.entries {
		if !e.blank() {
			return false
		}
	}
	return true
}

func (w *mcpAddWizard) importDrafts(actions MCPActions) {
	if actions.Parse == nil {
		w.jsonErr = errMCPUnavailable
		return
	}
	ds, err := actions.Parse([]byte(w.paste))
	for i := 0; err == nil && i < len(ds); i++ {
		err = mcpImportable(ds[i])
	}
	w.jsonErr = err
	if err != nil {
		return
	}
	w.invalidate(nil)
	if w.replace || w.allBlank() {
		for _, e := range w.entries {
			if e.cancel != nil {
				e.cancel()
			}
		}
		w.entries = nil
	}
	for _, d := range ds {
		w.addEntry(d)
	}
	w.edit = len(w.entries) - len(ds)
	w.paste, w.jsonMode, w.imported = "", false, len(ds)
}

// mcpImportable rejects what the form could not show unchanged: a
// transport it does not offer, or the fields of both kinds of transport.
func mcpImportable(d mcp.Draft) error {
	if !slices.Contains(mcpTransports, d.Transport) {
		return &mcp.DraftError{Code: "unsupported", Field: "transport"}
	}
	remote := d.URL != "" || len(d.Headers) > 0 || len(d.EnvHeaders) > 0 || d.BearerTokenEnv != ""
	local := d.Command != "" || len(d.Args) > 0 || len(d.Env) > 0 || d.Cwd != ""
	if d.Transport == "stdio" && remote || d.Transport != "stdio" && local {
		return &mcp.DraftError{Code: "conflict", Field: "transport"}
	}
	return nil
}

func (u *settingsUI) probeMCPDraft(actions MCPActions, e *mcpDraftEntry) {
	if actions.Probe == nil || e.task.Running {
		return
	}
	w := u.mcp.add
	w.invalidate(e)
	draft, err := e.draft()
	if err != nil {
		e.task = Task{Err: err}
		return
	}
	version := e.version
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

// prepareMCPDrafts checks every chosen provider's configuration for the
// servers, without writing; a retry checks again only the providers whose
// write failed and keeps the others' results.
func (u *settingsUI) prepareMCPDrafts(actions MCPActions, retry bool) {
	w := u.mcp.add
	if w.writing || actions.PrepareAdd == nil {
		return
	}
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	ds := w.drafts()
	if !retry {
		if len(ds) == 0 {
			w.err = &mcp.DraftError{Code: "size", Field: "drafts"}
			return
		}
		for _, e := range w.entries {
			if e.selected {
				if err := w.entryError(e); err != nil {
					w.err = err
					return
				}
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
		w.plans, w.task = nil, Task{}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.task = Task{Running: true}
	w.err = nil
	w.generation++
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
		})
	}()
}

// plan is the provider's prepared or finished target, if any.
func (w *mcpAddWizard) plan(p store.Provider) *mcpAddTarget {
	for i := range w.plans {
		if w.plans[i].provider == p {
			return &w.plans[i]
		}
	}
	return nil
}

// Called only from the main thread; a newly opened window takes ownership of
// the shared outcome without retaining an old window's controls.
func (u *settingsUI) currentMCPUI() *settingsUI {
	if s := u.opts.mcpAddSession; s != nil && s.owner != nil {
		return s.owner
	}
	return u
}
