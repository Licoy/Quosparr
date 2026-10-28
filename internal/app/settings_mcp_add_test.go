package app

import (
	"context"
	"errors"
	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
	"sync/atomic"
	"testing"
)

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
	w.entries = []*mcpDraftEntry{newMCPDraft(mcp.Draft{Name: "fixture", Transport: "http", URL: "https://mcp.example.test"})}
	return w.entries[0]
}
func TestMCPAddJSONBatchSharesEditedDraft(t *testing.T) {
	actions := mcpAddTestActions()
	u, tt, _ := settingsMCPView(t, actions)
	u.openMCPAdd()
	w := u.mcp.add
	w.paste = `{"mcpServers":{"alpha":{"url":"https://a.test"},"beta":{"command":"fixture","args":["one","two"],"env":{"FIXTURE":"value"}}}}`
	w.importDrafts(actions)
	if len(w.entries) != 2 || w.paste != "" || w.importJSON {
		t.Fatal("JSON import did not replace placeholder and clear source")
	}
	w.entries[0].draft.Name = "renamed"
	w.invalidate(w.entries[0])
	w.importJSON = true
	tt.Frame()
	if !tt.HasText("renamed") || w.entries[0].draft.Name != "renamed" {
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
	u, tt, queue := settingsMCPView(t, actions)
	e := mcpAddFixture(u)
	e.draft = mcp.Draft{Name: "fixture", Transport: "stdio", Command: "fixture"}
	u.mcp.add.step = 1
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
	e.draft.Command = "other"
	u.mcp.add.invalidate(e)
	close(release)
	finishDetail(t, queue)
	if e.probed || e.probe.ServerName != "" || e.task.Running {
		t.Fatal("stale preview overwrote edited draft")
	}
	release = make(chan struct{})
	u.probeMCPDraft(actions, e)
	<-started
	u.closeMCPAdd()
	close(release)
	finishDetail(t, queue)
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
	w.step = 2
	for _, p := range store.Providers() {
		w.targets[p] = true
	}
	u.prepareMCPDrafts(actions, false)
	if w.task.Running {
		t.Fatal("unverified bypassed confirmation")
	}
	w.unverified = true
	u.prepareMCPDrafts(actions, false)
	finishDetail(t, queue)
	tt.Frame()
	if calls.Load() != 3 || w.step != 2 || w.err == nil {
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
	finishDetail(t, queue)
	u.submitMCPDrafts(actions)
	<-started
	u.submitMCPDrafts(actions)
	close(release)
	finishDetail(t, queue)
	if codex.Load() != 1 || claude.Load() != 1 || !w.completed {
		t.Fatal("duplicate submission or missing result")
	}
	u.prepareMCPDrafts(actions, true)
	finishDetail(t, queue)
	if w.step != 3 {
		t.Fatal("retry skipped fresh preflight")
	}
	u.submitMCPDrafts(actions)
	finishDetail(t, queue)
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
	finishDetail(t, queue)
	u.submitMCPDrafts(actions)
	<-started
	u.closeMCPAdd()
	if w.entries != nil || w.paste != "" {
		t.Fatal("close retained editable secrets")
	}
	close(release)
	finishDetail(t, queue)
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
	finishDetail(t, queue)
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
	finishDetail(t, queue)
	finishDetail(t, queue)
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
