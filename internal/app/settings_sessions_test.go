package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
)

func managedSession(id, title string, updated time.Time) sessions.ManagedSession {
	model := "gpt-6"
	return sessions.ManagedSession{
		Item: sessions.Item{ID: id, Title: title, ProjectName: "Quosparr", UpdatedAt: updated, State: sessions.StateRecent,
			Totals: sessions.Totals{TotalTokens: 1200, UncachedInputTokens: 500, CachedInputTokens: 400, OutputTokens: 300, Turns: 4}, Model: &model},
		Provider: sessions.ProviderCodex, Path: "/synthetic/" + id + ".jsonl", Root: "/synthetic",
		Files: []sessions.ManagedFile{{Path: "/synthetic/" + id + ".jsonl", Size: 128}},
	}
}

func withProject(item sessions.ManagedSession, name, cwd string) sessions.ManagedSession {
	item.ProjectName = name
	if cwd != "" {
		item.Cwd = &cwd
	}
	return item
}

func settingsSessionsView(t *testing.T, actions SessionActions) (*settingsUI, *ui.Tester, chan func()) {
	t.Helper()
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Log("load:", err)
	}
	e.s.Apply()
	queue := detailMainQueue(t)
	u := newSettingsUI(e.s, SettingsOptions{Version: "9.9.9", Sessions: func() SessionActions { return actions }})
	u.tab = tabSessions
	s := u.sessionsState()
	s.loaded = true
	s.items = []sessions.ManagedSession{
		managedSession("new", "Review session management", time.Date(2026, 10, 18, 16, 30, 0, 0, time.Local)),
		managedSession("old", "Older release notes", time.Date(2026, 10, 10, 9, 0, 0, 0, time.Local)),
	}
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	tt.Frame()
	return u, tt, queue
}

func TestSessionScanDropsStaleProviderResult(t *testing.T) {
	e := newEnv(t, nil)
	queue := detailMainQueue(t)
	u := newSettingsUI(e.s, SettingsOptions{})
	codex, claude := make(chan struct{}), make(chan struct{})
	actions := SessionActions{Query: func(_ context.Context, provider store.Provider) ([]sessions.ManagedSession, error) {
		if provider == store.ProviderCodex {
			<-codex
			return []sessions.ManagedSession{managedSession("codex", "Codex", time.Now())}, nil
		}
		<-claude
		return []sessions.ManagedSession{managedSession("claude", "Claude", time.Now())}, nil
	}}
	u.startSessionsScan(actions)
	u.sessionsState().switchProvider(store.ProviderClaude)
	u.startSessionsScan(actions)
	close(codex)
	finishDetail(t, queue)
	if u.sessions.loaded || len(u.sessions.items) != 0 {
		t.Fatalf("stale Codex result landed: %+v", u.sessions.items)
	}
	close(claude)
	finishDetail(t, queue)
	if !u.sessions.loaded || len(u.sessions.items) != 1 || u.sessions.items[0].ID != "claude" {
		t.Fatalf("Claude result missing: %+v", u.sessions.items)
	}
}

func TestCancelSessionTasksStopsScanAndDropsCallback(t *testing.T) {
	e := newEnv(t, nil)
	queue := detailMainQueue(t)
	u := newSettingsUI(e.s, SettingsOptions{})
	started := make(chan struct{})
	actions := SessionActions{Query: func(ctx context.Context, _ store.Provider) ([]sessions.ManagedSession, error) {
		close(started)
		<-ctx.Done()
		return []sessions.ManagedSession{managedSession("late", "Late result", time.Now())}, ctx.Err()
	}}
	u.startSessionsScan(actions)
	<-started
	u.cancelSessionTasks()
	finishDetail(t, queue)
	if u.sessions.loaded || len(u.sessions.items) != 0 || u.sessions.cancel != nil {
		t.Fatalf("cancelled scan wrote state: loaded=%t items=%+v cancel=%v", u.sessions.loaded, u.sessions.items, u.sessions.cancel)
	}
}

func TestSessionDetailFailureWaitsForExplicitRetry(t *testing.T) {
	var calls atomic.Int32
	retryGate := make(chan struct{})
	var release sync.Once
	releaseRetry := func() { release.Do(func() { close(retryGate) }) }
	defer releaseRetry()
	actions := SessionActions{Detail: func(context.Context, sessions.ManagedSession) (sessions.ManagedDetail, error) {
		if calls.Load() == 1 {
			<-retryGate
		}
		calls.Add(1)
		return sessions.ManagedDetail{}, errors.New("preview unavailable")
	}}
	u, tt, queue := settingsSessionsView(t, actions)
	u.sessions.open = sessionKey(u.sessions.items[0])
	tt.Frame()
	for u.sessions.detail.task.Running {
		finishDetail(t, queue)
	}
	tt.Frame()
	tt.Frame()
	if calls.Load() != 1 {
		t.Fatalf("detail retried automatically %d times", calls.Load())
	}
	// A proxy toast's timer can post unrelated work to this same main queue.
	oldDelay := proxyMsgTime
	proxyMsgTime = 0
	u.flashProxyMsg("unrelated toast")
	proxyMsgTime = oldDelay
	var unrelated func()
	select {
	case unrelated = <-queue:
	case <-time.After(3 * time.Second):
		t.Fatal("toast callback did not arrive")
	}
	queue <- func() { unrelated(); releaseRetry() }
	if err := tt.Click(tx(u.L(), xSessionsRetry)); err != nil {
		t.Fatal(err)
	}
	for u.sessions.detail.task.Running {
		finishDetail(t, queue)
	}
	if calls.Load() != 2 {
		t.Fatalf("retry calls = %d, want 2", calls.Load())
	}
	if u.sessions.detail.task.Running || u.sessions.detail.task.Err == nil {
		t.Fatal("retry failure completion was not applied")
	}
	tt.Frame()
	tt.Frame()
	if calls.Load() != 2 {
		t.Fatalf("detail retried after the explicit retry %d times", calls.Load())
	}
}

func TestSessionDetailDropsStaleProviderABAResult(t *testing.T) {
	e := newEnv(t, nil)
	queue := detailMainQueue(t)
	u := newSettingsUI(e.s, SettingsOptions{})
	item := managedSession("same", "Same path", time.Now())
	first, second, firstStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	calls := 0
	actions := SessionActions{Detail: func(context.Context, sessions.ManagedSession) (sessions.ManagedDetail, error) {
		calls++
		if calls == 1 {
			close(firstStarted)
			<-first
			return sessions.ManagedDetail{Session: item, Messages: []sessions.ManagedMessage{{Text: "stale"}}}, nil
		}
		<-second
		return sessions.ManagedDetail{Session: item, Messages: []sessions.ManagedMessage{{Text: "current"}}}, nil
	}}
	u.loadSessionDetail(actions, item)
	<-firstStarted
	u.sessionsState().switchProvider(store.ProviderClaude)
	u.sessionsState().switchProvider(store.ProviderCodex)
	u.loadSessionDetail(actions, item)
	close(first)
	finishDetail(t, queue)
	if u.sessions.detail.loaded {
		t.Fatal("stale detail landed after provider ABA")
	}
	close(second)
	finishDetail(t, queue)
	if !u.sessions.detail.loaded || len(u.sessions.detail.detail.Messages) != 1 || u.sessions.detail.detail.Messages[0].Text != "current" {
		t.Fatalf("current detail = %+v", u.sessions.detail)
	}
}

func TestSessionDeleteConfirmationAndPartialResult(t *testing.T) {
	partial := errors.New("one file changed")
	actions := SessionActions{
		Query: func(context.Context, store.Provider) ([]sessions.ManagedSession, error) { return nil, nil },
		Delete: func(_ context.Context, selected []sessions.ManagedSession) ([]sessions.DeleteResult, error) {
			return []sessions.DeleteResult{
				{Session: selected[0], Removed: 1, Total: 1},
				{Session: selected[1], Removed: 1, Total: 2, Err: partial},
			}, errors.Join(partial)
		},
	}
	u, tt, queue := settingsSessionsView(t, actions)
	u.sessions.selected[sessionKey(u.sessions.items[0])] = true
	u.sessions.selected[sessionKey(u.sessions.items[1])] = true
	u.sessions.confirm = true
	tt.Frame()
	want := tx(u.L(), xSessionsDeleteConfirmMessage, "Codex", 2)
	if !tt.HasText(want) {
		t.Fatalf("confirmation missing provider/count; texts %q", tt.Texts())
	}
	cancel, ok := tt.Find(u.text(l10n.KeyCancel))
	if !ok {
		t.Fatal("cancel button missing")
	}
	tt.ClickAt(cancel.X+cancel.W+8+20, cancel.Y+cancel.H/2)
	finishDetail(t, queue) // delete
	finishDetail(t, queue) // rescan
	tt.Frame()
	if !u.sessions.noticeErr || !strings.Contains(u.sessions.notice, "1/2") || !strings.Contains(u.sessions.notice, partial.Error()) {
		t.Fatalf("partial feedback = %q", u.sessions.notice)
	}
}

func TestSettingsSessionsListAndDetailRender(t *testing.T) {
	actions := SessionActions{Detail: func(_ context.Context, item sessions.ManagedSession) (sessions.ManagedDetail, error) {
		return sessions.ManagedDetail{Session: item, Messages: []sessions.ManagedMessage{{Role: "user", Text: "Please review the release", At: item.UpdatedAt}, {Role: "assistant", Text: "The release checks pass"}}, Truncated: true}, nil
	}}
	u, tt, queue := settingsSessionsView(t, actions)
	for _, text := range []string{"Codex", "Claude", "Grok", "Review session management", tx(u.L(), xSessionsSelectFiltered, 2), "· " + tx(u.L(), xSessionsCacheHitRate), "44.4%"} {
		if !tt.HasText(text) {
			t.Fatalf("list missing %q; texts %q", text, tt.Texts())
		}
	}
	u.sessions.open = sessionKey(u.sessions.items[0])
	tt.Frame()
	finishDetail(t, queue)
	tt.Frame()
	for _, text := range []string{tx(u.L(), xSessionsPreview), tx(u.L(), xSessionsRoleUser), "Please review the release", tx(u.L(), xSessionsPreviewTruncated), "gpt-6", tx(u.L(), xSessionsCacheHitRate), "44.4%"} {
		if !tt.HasText(text) {
			t.Fatalf("detail missing %q; texts %q", text, tt.Texts())
		}
	}
}

func TestExportSettingsSessionsReview(t *testing.T) {
	if os.Getenv("QUOSPARR_SESSION_QA") == "" {
		t.Skip("set QUOSPARR_SESSION_QA=1 to export offscreen PNGs")
	}
	actions := SessionActions{Detail: func(_ context.Context, item sessions.ManagedSession) (sessions.ManagedDetail, error) {
		return sessions.ManagedDetail{Session: item, Messages: []sessions.ManagedMessage{{Role: "user", Text: "请检查这次发布的会话管理改动", At: item.UpdatedAt}, {Role: "assistant", Text: "筛选、详情与回收站删除流程均已覆盖"}}, Truncated: true}, nil
	}}
	u, tt, queue := settingsSessionsView(t, actions)
	u.sessions.items[0] = withProject(u.sessions.items[0], "Quosparr", "/Users/demo/Projects/Clients/Very-Long-Organization-Name/Productivity/Quosparr")
	u.sessions.items[1] = withProject(u.sessions.items[1], "Quosparr", "/Users/demo/Projects/Personal/Experiments/Quosparr")
	u.s.SetLanguage(store.LanguageSimplifiedChinese)
	tt.Frame()
	dir := filepath.Join("..", "..", ".build", "v1-session-qa")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exportReviewPNG(t, tt, filepath.Join(dir, "sessions-list-zh-Hans.png"))
	if err := tt.Click(tx(u.L(), xSessionsStartDate)); err != nil {
		t.Fatal(err)
	}
	exportReviewPNG(t, tt, filepath.Join(dir, "sessions-calendar-open-zh-Hans.png"))
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click(tx(u.L(), xSessionsProjectFilter)); err != nil {
		t.Fatal(err)
	}
	exportReviewPNG(t, tt, filepath.Join(dir, "sessions-project-open-zh-Hans.png"))
	tt.Key(0, ui.KeyEscape)
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageRussian} {
		u.s.SetLanguage(language)
		tt.SetSize(settingsMinWidth, settingsHeight)
		tt.Frame()
		exportReviewPNG(t, tt, filepath.Join(dir, "sessions-list-"+string(language)+"-min.png"))
	}
	u.s.SetLanguage(store.LanguageSimplifiedChinese)
	tt.SetSize(settingsWidth, settingsHeight)
	u.sessions.open = sessionKey(u.sessions.items[0])
	tt.Frame()
	for range 4 {
		if u.sessions.detail.loaded {
			break
		}
		finishDetail(t, queue)
		tt.Frame()
	}
	if !u.sessions.detail.loaded {
		t.Fatal("detail did not finish before screenshot")
	}
	exportReviewPNG(t, tt, filepath.Join(dir, "sessions-detail-zh-Hans.png"))
}
