package app

import (
	"context"
	"errors"
	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
	"sync/atomic"
	"testing"
	"time"
)

// takeMCPCallback captures the completion of a controlled, sole pending action.
// Cancellation tests must execute that callback even after cancellation clears
// its Running flag; waiting for an idle flag would skip the stale result.
func takeMCPCallback(t *testing.T, queue <-chan func()) func() {
	t.Helper()
	select {
	case callback := <-queue:
		return callback
	case <-time.After(3 * time.Second):
		t.Fatal("MCP completion callback was not posted")
		return nil
	}
}

func mcpAddTestActions() MCPActions {
	return MCPActions{Parse: mcp.ParseDrafts, PrepareAdd: func(_ context.Context, p store.Provider, ds []mcp.Draft) (mcp.AddPlan, error) {
		plan := mcp.AddPlan{Provider: mcp.Provider(p)}
		for _, d := range ds {
			plan.Items = append(plan.Items, mcp.AddItem{Name: d.Name, Status: "add", Details: "[redacted]"})
		}
		return plan, nil
	}, Add: func(context.Context, store.Provider, mcp.AddPlan) (mcp.AddResult, error) {
		return mcp.AddResult{Added: []string{"fixture"}}, nil
	}}
}
func mcpAddFixture(u *settingsUI) *mcpDraftEntry {
	u.openMCPAdd()
	w := u.mcp.add
	w.entries = nil
	return w.addEntry(mcp.Draft{Name: "fixture", Transport: "http", URL: "https://mcp.example.test"})
}
func TestMCPAddJSONBatchSharesEditedDraft(t *testing.T) {
	actions := mcpAddTestActions()
	u, tt, _ := settingsMCPView(t, actions)
	u.openMCPAdd()
	w := u.mcp.add
	w.paste = `{"mcpServers":{"alpha":{"url":"https://a.test"},"beta":{"command":"fixture","args":["one","two"],"env":{"FIXTURE":"value"}}}}`
	w.importDrafts(actions)
	if len(w.entries) != 2 || w.paste != "" || w.jsonMode || w.imported != 2 {
		t.Fatal("JSON import did not replace placeholder and clear source")
	}
	if d, err := w.entries[1].draft(); err != nil || d.Command != "fixture" || len(d.Args) != 2 || d.Env["FIXTURE"] != "value" {
		t.Fatal("JSON import changed the server")
	}
	w.entries[0].name = "renamed"
	w.invalidate(w.entries[0])
	w.jsonMode = true
	tt.Frame()
	w.jsonMode = false
	tt.Frame()
	if !tt.HasText("renamed") || w.entries[0].name != "renamed" {
		t.Fatal("form edit lost when reopening JSON import")
	}
	if len(w.drafts()) != 2 {
		t.Fatal("batch selection lost")
	}
	w.entries[1].selected = false
	w.invalidate(nil)
	if len(w.drafts()) != 1 {
		t.Fatal("batch checkbox ignored")
	}
}
func TestMCPAddPreviewRequiresClickAndDropsCancelledAndStale(t *testing.T) {
	actions := mcpAddTestActions()
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	actions.Probe = func(context.Context, mcp.Draft) (mcp.ProbeResult, error) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return mcp.ProbeResult{ServerName: "old"}, nil
	}
	u, tt, _ := settingsMCPView(t, actions)
	// Only this test's controlled preview posts to the dedicated dispatcher.
	queue := detailMainQueue(t)
	e := mcpAddFixture(u)
	e.transport, e.command = "stdio", "fixture"
	u.mcp.add.step = mcpStepTools
	tt.Frame()
	if calls.Load() != 0 {
		t.Fatal("render started subprocess")
	}
	if !tt.HasText("fixture") {
		t.Fatal("program summary missing")
	}
	if err := tt.Click(tx(u.L(), xMCPStartFetch)); err != nil {
		t.Fatal(err)
	}
	<-started
	e.command = "other"
	u.mcp.add.invalidate(e)
	close(release)
	takeMCPCallback(t, queue)()
	if e.probed || e.probe.ServerName != "" || e.task.Running {
		t.Fatal("stale preview overwrote edited draft")
	}
	release = make(chan struct{})
	u.probeMCPDraft(actions, e)
	<-started
	u.closeMCPAdd()
	close(release)
	takeMCPCallback(t, queue)()
	if u.mcp.add != nil || e.probed {
		t.Fatal("cancelled preview retained draft")
	}
}
func TestMCPAddPreflightAllTargetsBlocksOnAnyFailure(t *testing.T) {
	actions := mcpAddTestActions()
	var calls atomic.Int32
	actions.PrepareAdd = func(_ context.Context, p store.Provider, _ []mcp.Draft) (mcp.AddPlan, error) {
		calls.Add(1)
		if p == store.ProviderClaude {
			return mcp.AddPlan{}, &mcp.DraftError{Code: "conflict", Field: "name"}
		}
		return mcp.AddPlan{Provider: mcp.Provider(p)}, nil
	}
	u, tt, queue := settingsMCPView(t, actions)
	mcpAddFixture(u)
	w := u.mcp.add
	w.step = mcpStepReview
	for _, p := range store.Providers() {
		w.targets[p] = true
	}
	u.prepareMCPDrafts(actions, false)
	if w.task.Running {
		t.Fatal("unverified bypassed confirmation")
	}
	w.unverified = true
	u.prepareMCPDrafts(actions, false)
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	tt.Frame()
	if calls.Load() != 3 || w.step != mcpStepReview || w.err == nil {
		t.Fatal("all target preflight must complete and block on any failure")
	}
	var writes atomic.Int32
	actions.Add = func(context.Context, store.Provider, mcp.AddPlan) (mcp.AddResult, error) {
		writes.Add(1)
		return mcp.AddResult{}, nil
	}
	u.submitMCPDrafts(actions)
	if writes.Load() != 0 {
		t.Fatal("failed preflight permitted writes")
	}
}
func TestMCPAddPartialRetryAndDuplicateSubmit(t *testing.T) {
	actions := mcpAddTestActions()
	var codex, claude atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	actions.Add = func(_ context.Context, p store.Provider, _ mcp.AddPlan) (mcp.AddResult, error) {
		if p == store.ProviderCodex {
			codex.Add(1)
			close(started)
			<-release
			return mcp.AddResult{Added: []string{"fixture"}}, nil
		}
		if claude.Add(1) == 1 {
			return mcp.AddResult{}, errors.New("sensitive fixture error")
		}
		return mcp.AddResult{Added: []string{"fixture"}}, nil
	}
	u, _, queue := settingsMCPView(t, actions)
	mcpAddFixture(u)
	w := u.mcp.add
	w.unverified = true
	w.targets[store.ProviderClaude] = true
	u.prepareMCPDrafts(actions, false)
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	u.submitMCPDrafts(actions)
	<-started
	u.submitMCPDrafts(actions)
	close(release)
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	if codex.Load() != 1 || claude.Load() != 1 || !w.completed {
		t.Fatal("duplicate submission or missing result")
	}
	u.prepareMCPDrafts(actions, true)
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	if retried := w.plan(store.ProviderClaude); len(w.plans) != 2 || retried == nil || retried.done || retried.err != nil || !w.plan(store.ProviderCodex).done {
		t.Fatal("retry skipped fresh preflight")
	}
	u.submitMCPDrafts(actions)
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	if codex.Load() != 1 || claude.Load() != 2 {
		t.Fatal("retry rewrote successful provider")
	}
}
func TestMCPAddConfirmedWriteSurvivesClose(t *testing.T) {
	actions := mcpAddTestActions()
	started, release := make(chan struct{}), make(chan struct{})
	actions.Add = func(context.Context, store.Provider, mcp.AddPlan) (mcp.AddResult, error) {
		close(started)
		<-release
		return mcp.AddResult{Added: []string{"fixture"}}, nil
	}
	u, _, queue := settingsMCPView(t, actions)
	mcpAddFixture(u)
	w := u.mcp.add
	w.unverified = true
	u.prepareMCPDrafts(actions, false)
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	u.submitMCPDrafts(actions)
	<-started
	u.closeMCPAdd()
	if w.entries != nil || w.paste != "" {
		t.Fatal("close retained editable secrets")
	}
	close(release)
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	u.openMCPAdd()
	if u.mcp.add != w || !w.completed || len(w.plans) != 1 || !w.plans[0].done {
		t.Fatal("confirmed result lost after closing")
	}
}

func TestMCPAddSharedSessionSurvivesWholeWindowRecreation(t *testing.T) {
	actions := mcpAddTestActions()
	started, release := make(chan struct{}), make(chan struct{})
	actions.Add = func(context.Context, store.Provider, mcp.AddPlan) (mcp.AddResult, error) {
		close(started)
		<-release
		return mcp.AddResult{Added: []string{"fixture"}}, nil
	}
	actions.Query = func(context.Context, store.Provider) (mcp.Snapshot, error) {
		return mcp.Snapshot{Path: "refreshed-new-window"}, nil
	}
	u, _, queue := settingsMCPView(t, actions)
	session := &mcpAddSession{owner: u}
	u.opts.mcpAddSession = session
	mcpAddFixture(u)
	w := u.mcp.add
	w.unverified = true
	u.prepareMCPDrafts(actions, false)
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	u.submitMCPDrafts(actions)
	<-started
	u.cancelMCPTasks()
	if session.wizard != w || len(w.entries) != 0 {
		t.Fatal("closed window lost confirmed operation or retained editable secrets")
	}
	next := newSettingsUI(u.s, SettingsOptions{mcpAddSession: session, MCP: func() MCPActions { return actions }})
	next.mcpState().loaded = true
	next.openMCPAdd()
	if next.mcp.add != w || !w.writing {
		t.Fatal("new window failed to reuse confirmed operation")
	}
	close(release)
	finishMCPUntil(t, queue, func() bool { return !w.writing && !next.mcp.task.Running })
	if !w.completed || !w.plans[0].done || next.mcp.snapshot.Path != "refreshed-new-window" {
		t.Fatal("result did not reach current window or refresh its list")
	}
	w.completed = false
	next.closeMCPAdd()
	if session.wizard != nil {
		t.Fatal("completed operation not released")
	}
	next.openMCPAdd()
	next.cancelMCPTasks()
	if session.wizard != nil || next.mcp.add != nil {
		t.Fatal("unconfirmed secret draft survived whole-window close")
	}
}

func TestMCPAddFormBuildsDraftAndRejectsBadRows(t *testing.T) {
	e := newMCPDraft(mcp.Draft{Name: " fixture ", Transport: "http", URL: " https://mcp.example.test ", Headers: map[string]string{"B": "2", "A": "1"}})
	if e.headers[0].key != "A" || e.headers[1].key != "B" {
		t.Fatal("headers not listed in order")
	}
	e.headers = append(e.headers, mcpPair{id: e.newID()})
	d, err := e.draft()
	if err != nil || d.Name != "fixture" || d.URL != "https://mcp.example.test" || len(d.Headers) != 2 || d.Command != "" {
		t.Fatal("blank row or spaces reached the draft")
	}
	e.command, e.args = "stale", []mcpArg{{id: e.newID(), value: "x"}}
	if d, _ = e.draft(); d.Command != "" || len(d.Args) != 0 {
		t.Fatal("other transport's fields reached the draft")
	}
	e.headers[2].value = "orphan"
	if _, err = e.draft(); !mcpErrorIs(err, "invalid", "headers") {
		t.Fatal("value without a name accepted")
	}
	e.headers[2].key = " A"
	if _, err = e.draft(); !mcpErrorIs(err, "duplicate", "headers") {
		t.Fatal("repeated header accepted")
	}
}

func TestMCPAddContinueShowsFirstInvalidServer(t *testing.T) {
	u, tt, _ := settingsMCPView(t, mcpAddTestActions())
	first := mcpAddFixture(u)
	w := u.mcp.add
	second := w.addEntry(mcp.Draft{Name: "fixture", Transport: "stdio", Command: "fixture"})
	third := w.addEntry(mcp.Draft{Name: "third", Transport: "http", URL: "https://third.test", BearerTokenEnv: "bad name"})
	third.advanced = false
	w.edit = 0
	w.continueToTools()
	if w.step != mcpStepConfigure || w.edit != 1 || !w.attempted {
		t.Fatal("duplicate name passed or was not shown")
	}
	tt.Frame()
	if !tt.HasText(tx(u.L(), xMCPErrorDuplicate)) || !tt.HasText(tx(u.L(), xMCPNeedsFix)) {
		t.Fatal("field error or list badge missing")
	}
	second.name = "second"
	w.continueToTools()
	if w.edit != 2 || !third.advanced {
		t.Fatal("folded field error stayed hidden")
	}
	third.selected = false
	w.continueToTools()
	if w.step != mcpStepTools || len(w.drafts()) != 2 || first.name != "fixture" {
		t.Fatal("valid servers did not continue")
	}
}

func TestMCPAddImportRejectsWhatTheFormCannotShow(t *testing.T) {
	actions := mcpAddTestActions()
	u, _, _ := settingsMCPView(t, actions)
	mcpAddFixture(u)
	w := u.mcp.add
	for _, paste := range []string{`{"command":"fixture","url":"https://a.test"}`, `{"type":"websocket","url":"https://a.test"}`} {
		w.paste = paste
		w.importDrafts(actions)
		if w.jsonErr == nil || len(w.entries) != 1 || w.entries[0].name != "fixture" || w.paste == "" {
			t.Fatalf("unrepresentable JSON accepted: %s", paste)
		}
	}
	w.paste = `{"mcpServers":{"beta":{"url":"https://b.test"}}}`
	w.importDrafts(actions)
	if w.jsonErr != nil || len(w.entries) != 2 || w.edit != 1 {
		t.Fatal("JSON did not append to a typed form")
	}
	w.replace = true
	w.paste = `{"mcpServers":{"gamma":{"url":"https://c.test"}}}`
	w.importDrafts(actions)
	if len(w.entries) != 1 || w.entries[0].name != "gamma" {
		t.Fatal("replace kept old servers")
	}
}

func TestMCPAddRemoveLastServerLeavesEmptyForm(t *testing.T) {
	u, _, _ := settingsMCPView(t, mcpAddTestActions())
	mcpAddFixture(u)
	w := u.mcp.add
	w.addEntry(mcp.Draft{Name: "second", Transport: "stdio"})
	w.removeEntry(1)
	if len(w.entries) != 1 || w.edit != 0 {
		t.Fatal("edit index out of range after removal")
	}
	w.removeEntry(0)
	if len(w.entries) != 1 || !w.entries[0].blank() || w.edit != 0 {
		t.Fatal("removing the last server left no form")
	}
}

func mcpErrorIs(err error, code, field string) bool {
	var d *mcp.DraftError
	return errors.As(err, &d) && d.Code == code && d.Field == field
}
