package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

type settingsSkillCalls struct {
	queries        int
	trashReads     int
	enabled        []bool
	deleted        []string
	inspect        []string
	restored       []string
	purged         []string
	emptied        int
	copied         []string
	transfers      []skills.TransferRequest
	transferResult skills.TransferResult
	transferErr    error
	queryResult    map[store.Provider]skills.Summary
	lifted         []string
	described      []string
}

const settingsSkillsRepo = "https://github.com/acme/agent-skills"

func settingsSkillsFixture() map[store.Provider]skills.Summary {
	res := skillsFixture()
	sum := res[store.ProviderCodex]
	sum.Skills[0].Origin = skills.Origin{
		From: skills.OriginLock, Repository: settingsSkillsRepo, GitHub: "acme/agent-skills",
		Folder: settingsSkillsRepo + "/tree/HEAD/skills/review", SkillPath: "skills/review",
		InstalledAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}
	sum.Skills[0].ModTime = time.Date(2026, 10, 8, 14, 35, 0, 0, time.Local)
	sum.Skills[1].ModTime = time.Date(2026, 10, 7, 9, 10, 0, 0, time.Local)
	res[store.ProviderCodex] = sum
	return res
}

func settingsTrashFixture() []skills.TrashEntry {
	return []skills.TrashEntry{
		{ID: "t1", Kind: skills.TrashSkill, Provider: skills.ProviderCodex, Name: "lint", InvokeName: "lint", Description: "Lint the code",
			Original: skillsTestRoot + "/lint", OriginalLabel: "~/.codex/skills/lint", DeletedAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), Size: 2048},
		{ID: "t2", Kind: skills.TrashBrokenLink, Provider: skills.ProviderClaude, Name: "gone",
			Original: "/home/u/.claude/skills/gone", OriginalLabel: "~/.claude/skills/gone", DeletedAt: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC), Size: -1},
	}
}

// settingsSkillsView opens the settings window on the skills tab with
// fake actions and runs the first scan and trash read.
func settingsSkillsView(t *testing.T, system bool) (*settingsUI, *ui.Tester, chan func(), *settingsSkillCalls) {
	t.Helper()
	calls := &settingsSkillCalls{}
	trash := settingsTrashFixture()
	acts := SkillActions{
		Query: func(context.Context) (map[store.Provider]skills.Summary, error) {
			calls.queries++
			if calls.queryResult != nil {
				return calls.queryResult, nil
			}
			return settingsSkillsFixture(), nil
		},
		ReadBody: func(skills.Skill) (string, bool, error) { return "# Steps", false, nil },
		SetEnabled: func(_ skills.Skill, on bool) error {
			calls.enabled = append(calls.enabled, on)
			return nil
		},
		Delete: func(s skills.Skill, _ []skills.RootInfo) (bool, error) {
			calls.deleted = append(calls.deleted, s.Removal.Path)
			return system, nil
		},
		DeleteLink: func(skills.Summary, string) (bool, error) { return system, nil },
		LiftNested: func(_ skills.Summary, s skills.Skill) (skills.LiftResult, error) {
			calls.lifted = append(calls.lifted, s.File)
			return skills.LiftResult{Paths: []string{"/home/u/.claude/skills/" + s.Name}}, nil
		},
		SuggestDescription: func(skills.Skill) string { return "Suggested text" },
		AddDescription: func(s skills.Skill, d string) error {
			calls.described = append(calls.described, s.Name+": "+d)
			return nil
		},
		OpenFolder: func(string) error { return nil },
		Reveal:     func(string) {},
		Copy:       func(p string) { calls.copied = append(calls.copied, p) },
		Transfer: func(req skills.TransferRequest) (skills.TransferResult, error) {
			calls.transfers = append(calls.transfers, req)
			got := calls.transferResult
			if got.Path == "" {
				got.Path = "/home/u/." + string(req.Target) + "/skills/" + req.Skill.Name
			}
			return got, calls.transferErr
		},
		Inspect: func(_ context.Context, s skills.Skill) (skills.Detail, error) {
			calls.inspect = append(calls.inspect, s.Name)
			return skills.Detail{Files: []skills.FileEntry{{Path: "SKILL.md", Size: 512}, {Path: "scripts", Dir: true}, {Path: "scripts/run.sh", Size: 64}}, FileCount: 2, DirCount: 1, Size: 576}, nil
		},
		TrashDir: "/home/u/.quosparr/skills-trash",
		ListTrash: func(context.Context) ([]skills.TrashEntry, error) {
			calls.trashReads++
			return trash, nil
		},
		Restore: func(id string) (skills.TrashEntry, error) {
			calls.restored = append(calls.restored, id)
			return trash[0], nil
		},
		Purge:      func(id string) error { calls.purged = append(calls.purged, id); return nil },
		EmptyTrash: func() (int, error) { calls.emptied++; return len(trash), nil },
	}
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Log("load:", err)
	}
	e.s.Apply()
	queue := detailMainQueue(t) // after newEnv, which runs main work inline
	u := newSettingsUI(e.s, SettingsOptions{Version: "9.9.9", Skills: func() SkillActions { return acts }})
	u.tab = tabSkills
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	tt.Frame()
	settleSkills(t, tt, queue, 2) // the scan and the trash
	return u, tt, queue, calls
}

// settleSkills runs n finished background steps, in whatever order they
// arrive, and draws a frame.
func settleSkills(t *testing.T, tt *ui.Tester, queue chan func(), n int) {
	t.Helper()
	for range n {
		finishDetail(t, queue)
	}
	tt.Frame()
}

func TestSettingsSkillsTabFollowsNotificationsInItsOwnGroup(t *testing.T) {
	i := slices.Index(settingsTabs, tabSkills)
	if i <= 0 || settingsTabs[i-1] != tabSessions || settingsTabs[i+1] != tabMCP || settingsTabs[i+2] != tabModelPricing {
		t.Fatalf("tabs = %v", settingsTabs)
	}
	if !sidebarGroupStarts[tabSessions] || sidebarGroupStarts[tabSkills] || sidebarGroupStarts[tabMCP] || !sidebarGroupStarts[tabModelPricing] || sidebarGroupStarts[tabAdvanced] {
		t.Fatal("sessions, skills, and MCP are not one sidebar group")
	}
}

func TestSettingsSkillsListsProvidersStatesAndFilters(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, _, _ := settingsSkillsView(t, false)
	for _, text := range []string{"Codex", "Claude", "Grok", tx(l, xSkillsTrash), tx(l, xSkillsAll), "review", "pdf", "imagegen", "GitHub", tx(l, xSkillsRoots)} {
		if !hasExact(tt, text) {
			t.Fatalf("skills tab missing %q; texts %q", text, tt.Texts())
		}
	}
	// The state cells filter: only pdf is off.
	if err := tt.Click(tx(l, xSkillsStateOff)); err != nil {
		t.Fatal(err)
	}
	if !hasExact(tt, "pdf") || hasExact(tt, "review") || hasExact(tt, "imagegen") {
		t.Fatalf("state filter did not apply; texts %q", tt.Texts())
	}
	if !hasExact(tt, tx(l, xSkillsShowing, 1, 3)) {
		t.Fatalf("count of shown skills missing; texts %q", tt.Texts())
	}
	f := &u.skills.filter[0]
	f.state, f.flags = "", skillFlagGitHub
	tt.Frame()
	if !hasExact(tt, "review") || hasExact(tt, "pdf") {
		t.Fatalf("GitHub flag did not filter; texts %q", tt.Texts())
	}
	if err := tt.Click(tx(l, xSkillsClearFilters)); err != nil {
		t.Fatal(err)
	}
	if f.flags != 0 || !hasExact(tt, "pdf") {
		t.Fatalf("clear filters left flags=%d; texts %q", f.flags, tt.Texts())
	}
	f.search = "acme"
	tt.Frame()
	if !hasExact(tt, "review") || hasExact(tt, "pdf") {
		t.Fatalf("search by repository did not filter; texts %q", tt.Texts())
	}
}

func TestSettingsSkillListActionsAreIconOnlyAndBelowInfo(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, calls := settingsSkillsView(t, false)
	const longDescription = "Review a pull request with a deliberately long description that must stay on one line even in the narrow settings window"
	sum := u.skills.result[store.ProviderCodex]
	sum.Skills[0].Description = longDescription
	u.skills.result[store.ProviderCodex] = sum
	tt = ui.NewTester(u.view, settingsMinWidth, settingsHeight)
	tt.Frame()
	description, ok := tt.Find(longDescription)
	if !ok {
		t.Fatal("review description missing")
	}
	if description.H > 18 {
		t.Fatalf("description height = %v, want one line", description.H)
	}
	if hasExact(tt, "~/.codex/skills/review") || hasExact(tt, tx(l, xSkillsEnabled)) {
		t.Fatalf("list still shows its path or visible enabled label; texts %q", tt.Texts())
	}
	switchLabel := tx(l, xSkillsEnabled) + " review"
	action, ok := tt.Find(switchLabel)
	if !ok || action.Y <= description.Y+description.H {
		t.Fatalf("action row is not below the information row: description=%v action=%v", description, action)
	}
	if action.W != 30 || action.H != 16 {
		t.Fatalf("compact switch bounds = %v, want 30x16", action)
	}
	for _, label := range []string{tx(l, xSkillsOpenFolder), tx(l, xSkillsReveal), tx(l, xSkillsCopyPath), tx(l, xSkillsTransfer), tx(l, xSkillsDelete)} {
		assertSkillIconButton(t, tt, label)
	}
	if err := tt.Click(switchLabel); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 3)
	if len(calls.enabled) != 1 || calls.enabled[0] {
		t.Fatalf("compact switch calls = %v, want [false]", calls.enabled)
	}

	u.skills.filter[0].search = "pdf"
	tt.Frame()
	shared := tx(l, xSkillsSharedCompact, "Grok")
	date := "2026-10-07"
	sharedBox, sharedOK := tt.Find(shared)
	dateBox, dateOK := tt.Find(date)
	trashBox, trashOK := tt.Find(tx(l, xSkillsDelete))
	if !sharedOK || !dateOK || !trashOK || sharedBox.Y != dateBox.Y || trashBox.X >= sharedBox.X || sharedBox.X >= dateBox.X {
		t.Fatalf("action metadata is not one right-aligned row: trash=%v shared=%v date=%v", trashBox, sharedBox, dateBox)
	}
	if hasExact(tt, "~/.agents/skills/pdf") {
		t.Fatalf("list still shows item path; texts %q", tt.Texts())
	}
	tt.Move(dateBox.X+dateBox.W/2, dateBox.Y+dateBox.H/2)
	tt.Advance(700 * time.Millisecond)
	if !hasExact(tt, tx(l, xSkillsModified)+" 2026-10-07 09:10") {
		t.Fatalf("modified tooltip missing; texts %q", tt.Texts())
	}

	u.skills.filter[0].search = "imagegen"
	tt.Frame()
	if err := tt.Click(tx(l, xSkillsTransfer)); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), tx(l, xSkillsTransferNotMovable)) {
		t.Fatalf("non-movable reason missing from menu %q", tt.Menu())
	}
	tt.CloseMenu()
}

func TestSettingsSkillTransferCopiesAndRefreshes(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt, queue, calls := settingsSkillsView(t, false)
	if err := tt.Click(tx(l, xSkillsTransfer)); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(tx(l, xSkillsCopyTo), "Claude"); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 3)
	if len(calls.transfers) != 1 || calls.transfers[0].Target != skills.ProviderClaude || calls.transfers[0].Move {
		t.Fatalf("transfers = %+v", calls.transfers)
	}
	if calls.queries != 2 || calls.trashReads != 2 {
		t.Fatalf("query/trash reads = %d/%d, want 2/2", calls.queries, calls.trashReads)
	}
	if !hasExact(tt, tx(l, xSkillsCopiedTo, "/home/u/.claude/skills/review")) {
		t.Fatalf("copy notice missing; texts %q", tt.Texts())
	}
}

func TestSettingsSkillTransferMoveConfirmsTargetAndSource(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, calls := settingsSkillsView(t, false)
	if err := tt.Click("review"); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 1)
	if err := tt.Click(tx(l, xSkillsTransfer)); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(tx(l, xSkillsMoveTo), "Grok"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{tx(l, xSkillsMoveTitle, "review", "Grok"), "~/.codex/skills/review"} {
		if !tt.HasText(text) {
			t.Fatalf("move confirmation missing %q; texts %q", text, tt.Texts())
		}
	}
	if len(calls.transfers) != 0 {
		t.Fatal("moved before confirmation")
	}
	if err := tt.Click(tx(l, xSkillsMove)); err != nil {
		t.Fatal(err)
	}
	calls.queryResult = map[store.Provider]skills.Summary{
		store.ProviderCodex: {Provider: skills.ProviderCodex},
	}
	settleSkills(t, tt, queue, 3)
	if len(calls.transfers) != 1 || calls.transfers[0].Target != skills.ProviderGrok || !calls.transfers[0].Move {
		t.Fatalf("transfers = %+v", calls.transfers)
	}
	want := tx(l, xSkillsMovedTo, "/home/u/.grok/skills/review", tx(l, xSkillsMovedTrash))
	if u.skills.open != "" || !hasExact(tt, want) {
		t.Fatalf("move result lost after detail closed: open=%q texts=%q", u.skills.open, tt.Texts())
	}
}

func TestSettingsSkillTransferMoveFromListKeepsFilter(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, _ := settingsSkillsView(t, false)
	u.skills.filter[0].search = "review"
	tt.Frame()
	if err := tt.Click(tx(l, xSkillsTransfer)); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(tx(l, xSkillsMoveTo), "Claude"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(l, xSkillsMove)); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 3)
	if u.skills.filter[0].search != "review" || u.skills.open != "" {
		t.Fatalf("list state changed after move: filter=%q open=%q", u.skills.filter[0].search, u.skills.open)
	}
}

func TestSettingsSkillTransferErrorsAreActionable(t *testing.T) {
	l := l10n.New(l10n.English)
	t.Run("conflict", func(t *testing.T) {
		_, tt, queue, calls := settingsSkillsView(t, false)
		calls.transferErr = skills.ErrTransferConflict
		if err := tt.Click(tx(l, xSkillsTransfer)); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(tx(l, xSkillsCopyTo), "Claude"); err != nil {
			t.Fatal(err)
		}
		settleSkills(t, tt, queue, 3)
		want := tx(l, xSkillsTransferFailed, tx(l, xSkillsTransferConflict))
		if !hasExact(tt, want) {
			t.Fatalf("conflict notice missing; texts %q", tt.Texts())
		}
	})
	t.Run("partial move", func(t *testing.T) {
		_, tt, queue, calls := settingsSkillsView(t, false)
		calls.transferResult.Path = "/custom/claude/skills/review"
		calls.transferErr = errors.Join(skills.ErrTransferPartial, skills.ErrChanged)
		if err := tt.Click(tx(l, xSkillsTransfer)); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(tx(l, xSkillsMoveTo), "Claude"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(tx(l, xSkillsMove)); err != nil {
			t.Fatal(err)
		}
		settleSkills(t, tt, queue, 3)
		want := tx(l, xSkillsTransferFailed, tx(l, xSkillsTransferPartial, calls.transferResult.Path))
		if !hasExact(tt, want) {
			t.Fatalf("partial notice missing; texts %q", tt.Texts())
		}
	})
	t.Run("ordinary changed", func(t *testing.T) {
		_, tt, queue, calls := settingsSkillsView(t, false)
		calls.transferErr = skills.ErrChanged
		if err := tt.Click(tx(l, xSkillsTransfer)); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(tx(l, xSkillsCopyTo), "Claude"); err != nil {
			t.Fatal(err)
		}
		settleSkills(t, tt, queue, 3)
		if !hasExact(tt, tx(l, xSkillsChanged)) {
			t.Fatalf("changed notice missing; texts %q", tt.Texts())
		}
	})
}

func assertSkillIconButton(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("accessible label %q missing", label)
	}
	if r.W != 26 || r.H != 26 {
		t.Fatalf("%q bounds = %v, want a 26x26 icon button", label, r)
	}
}

func TestSettingsSkillPageShowsOriginFilesAndLinks(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, calls := settingsSkillsView(t, false)
	if err := tt.Click("review"); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 1) // Inspect
	if len(calls.inspect) != 1 || calls.inspect[0] != "review" {
		t.Fatalf("inspect calls %v", calls.inspect)
	}
	for _, text := range []string{tx(l, xSkillsSecOrigin), tx(l, xSkillsInstSkillsCLI), "skills/review", tx(l, xSkillsSecFiles), "run.sh", tx(l, xSkillsFilesCount, 2, 1) + " · 576 B", tx(l, xSkillsOpenOnline)} {
		if !hasExact(tt, text) {
			t.Fatalf("skill page missing %q; texts %q", text, tt.Texts())
		}
	}
	repo, _ := tt.Find("acme/agent-skills")
	tt.Move(repo.X+repo.W/2, repo.Y+repo.H/2)
	tt.Advance(700 * time.Millisecond)
	if !hasExact(tt, "acme/agent-skills\n"+settingsSkillsRepo) {
		t.Fatalf("source icon tooltip does not include its label and URL; texts %q", tt.Texts())
	}
	if err := tt.Click("acme/agent-skills"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"acme/agent-skills", tx(l, xSkillsOpenOnline), tx(l, xSkillsOpenFolder), tx(l, xSkillsReveal), tx(l, xSkillsCopyPath), tx(l, xSkillsTransfer), tx(l, xSkillsDelete)} {
		assertSkillIconButton(t, tt, label)
	}
	if got := tt.OpenedURLs(); len(got) != 1 || got[0] != settingsSkillsRepo {
		t.Fatalf("opened %v", got)
	}
	if err := tt.Click(tx(l, xSkillsCopyPath)); err != nil {
		t.Fatal(err)
	}
	if len(calls.copied) != 1 || calls.copied[0] != skillsTestRoot+"/review/SKILL.md" {
		t.Fatalf("copied %v", calls.copied)
	}
	if err := tt.Click(tx(l, xSkillsBack)); err != nil {
		t.Fatal(err)
	}
	if u.skills.open != "" {
		t.Fatal("the breadcrumb did not return to the list")
	}
}

func TestSettingsSkillPageHidesTabsAndFilters(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, _ := settingsSkillsView(t, false)
	u.skills.open = u.skills.result[store.ProviderCodex].Skills[0].ID
	tt.Frame()
	settleSkills(t, tt, queue, 1)
	for _, text := range []string{"Claude", "Grok", tx(l, xSkillsTrash), tx(l, xSkillsFilter)} {
		if hasExact(tt, text) {
			t.Fatalf("skill page kept %q; texts %q", text, tt.Texts())
		}
	}
	if !hasExact(tt, "Codex") || !hasExact(tt, tx(l, xSkillsBack)) {
		t.Fatalf("breadcrumb missing; texts %q", tt.Texts())
	}
}

func TestSettingsSkillPageWithoutOriginSaysUnknown(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, _ := settingsSkillsView(t, false)
	u.skills.open = "codex\x00" + skillsTestShared + "/pdf/SKILL.md"
	tt.Frame()
	settleSkills(t, tt, queue, 1)
	if !hasExact(tt, tx(l, xSkillsOriginUnknown)) || hasExact(tt, tx(l, xSkillsOpenOnline)) {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestSettingsSkillDeleteNamesTheSystemTrashFallback(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, calls := settingsSkillsView(t, true)
	u.skills.open = "codex\x00" + skillsTestShared + "/pdf/SKILL.md"
	tt.Frame()
	settleSkills(t, tt, queue, 1)
	if err := tt.Click(tx(l, xSkillsDelete)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	cancel, ok := tt.Find(l.Text(l10n.KeyCancel))
	if !ok {
		t.Fatalf("Cancel missing; texts %q", tt.Texts())
	}
	tt.ClickAt(cancel.X+cancel.W+8+20, cancel.Y+cancel.H/2)
	settleSkills(t, tt, queue, 3) // the move, then the scan and the trash
	if len(calls.deleted) != 1 || calls.deleted[0] != skillsTestShared+"/pdf" {
		t.Fatalf("deleted %v", calls.deleted)
	}
	if u.skills.open != "" || !hasExact(tt, tx(l, xSkillsMovedSystem)) {
		t.Fatalf("after delete: open=%q texts %q", u.skills.open, tt.Texts())
	}
}

func TestSettingsSkillsTrashRestoresPurgesAndFilters(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, calls := settingsSkillsView(t, false)
	if err := tt.Click(tx(l, xSkillsTrash)); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"lint", "Lint the code", "gone", tx(l, xSkillsKindBrokenLink), tx(l, xSkillsTrashTotal, 2, "2.0 KB")} {
		if !hasExact(tt, text) {
			t.Fatalf("trash missing %q; texts %q", text, tt.Texts())
		}
	}
	for _, label := range []string{tx(l, xSkillsOpenTrashDir), tx(l, xSkillsEmptyTrash), tx(l, xSkillsRestore), tx(l, xSkillsPurge)} {
		assertSkillIconButton(t, tt, label)
	}
	// The provider picker is a select, as the font picker: typing narrows
	// its options and Enter chooses one.
	if err := tt.Click(tx(l, xSkillsProviderAll)); err != nil {
		t.Fatal(err)
	}
	tt.Type("Claude")
	tt.Key(0, ui.KeyEnter)
	if u.skills.trash.provider != 2 {
		t.Fatalf("picker chose provider %d; texts %q", u.skills.trash.provider, tt.Texts())
	}
	if hasExact(tt, "lint") || !hasExact(tt, "gone") {
		t.Fatalf("provider filter did not apply; texts %q", tt.Texts())
	}
	u.skills.trash.provider = 0
	tt.Frame()
	if err := tt.Click(tx(l, xSkillsRestore)); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 3)
	if len(calls.restored) != 1 || calls.restored[0] != "t1" || !hasExact(tt, tx(l, xSkillsRestored, "~/.codex/skills/lint")) {
		t.Fatalf("restored %v; texts %q", calls.restored, tt.Texts())
	}
	if err := tt.Click(tx(l, xSkillsPurge)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !hasExact(tt, tx(l, xSkillsPurgeTitle)) || len(calls.purged) != 0 {
		t.Fatalf("purge did not confirm first; purged %v", calls.purged)
	}
	cancel, _ := tt.Find(l.Text(l10n.KeyCancel))
	tt.ClickAt(cancel.X+cancel.W+8+20, cancel.Y+cancel.H/2)
	settleSkills(t, tt, queue, 3)
	if len(calls.purged) != 1 || calls.purged[0] != "t1" {
		t.Fatalf("purged %v", calls.purged)
	}
}

func TestFilterSkillsSortsAndMatchesFlags(t *testing.T) {
	sum := settingsSkillsFixture()[store.ProviderCodex]
	names := func(list []skills.Skill) []string {
		var out []string
		for _, sk := range list {
			out = append(out, sk.Name)
		}
		return out
	}
	if got := names(filterSkills(sum, skillFilter{sort: skillSortName})); !slices.Equal(got, []string{"imagegen", "pdf", "review"}) {
		t.Fatalf("by name %v", got)
	}
	if got := names(filterSkills(sum, skillFilter{flags: skillFlagShared})); !slices.Equal(got, []string{"pdf"}) {
		t.Fatalf("shared %v", got)
	}
	if got := names(filterSkills(sum, skillFilter{flags: skillFlagDeletable | skillFlagSwitchable})); !slices.Equal(got, []string{"review", "pdf"}) {
		t.Fatalf("deletable and switchable %v", got)
	}
	if got := names(filterSkills(sum, skillFilter{state: skills.StateOn, source: skillsTestRoot})); len(got) > 2 {
		t.Fatalf("state and source %v", got)
	}
}

func TestSkillsUIReviewPNGs(t *testing.T) {
	if os.Getenv("RUNWAY_UI_QA") == "" {
		t.Skip("set RUNWAY_UI_QA to export Skill UI PNGs")
	}
	dir := filepath.Join("..", "..", ".build", "v1-ui-qa")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	u, settings, queue, _ := settingsSkillsView(t, false)
	u.s.SetLanguage(store.LanguageSimplifiedChinese)
	sum := u.skills.result[store.ProviderCodex]
	long := &sum.Skills[0]
	long.DisplayName = "面向复杂代码审查与发布前检查的团队工作流 Skill"
	long.InvokeName = "review-production-release-checker"
	long.Description = "自动检查跨平台项目中的代码变更、测试结果、发布资产与文档一致性，并生成便于团队复核的完整结论"
	long.Root = "/home/u/.codex/skills/team-platform-engineering-and-release-workflows"
	long.RootLabel = "~/.codex/skills/team-platform-engineering-and-release-workflows"
	long.Dir = long.Root + "/review-production-release-checker"
	long.File = long.Dir + "/SKILL.md"
	long.Removal.Path = long.Dir
	long.SharedWith = []skills.Provider{skills.ProviderClaude, skills.ProviderGrok}
	long.ModTime = time.Date(2026, 10, 18, 16, 42, 0, 0, time.Local)
	u.skills.result[store.ProviderCodex] = sum
	settings = ui.NewTester(u.view, settingsMinWidth, settingsHeight)
	settings.Frame()
	exportReviewPNG(t, settings, filepath.Join(dir, "skills-settings-list-zh-Hans.png"))
	u.skills.open = long.ID
	settings.Frame()
	settleSkills(t, settings, queue, 1)
	exportReviewPNG(t, settings, filepath.Join(dir, "skills-settings-detail-zh-Hans.png"))
	l := l10n.New(l10n.SimplifiedChinese)
	if err := settings.Click(tx(l, xSkillsTransfer)); err != nil {
		t.Fatal(err)
	}
	if err := settings.ChooseMenuItem(tx(l, xSkillsMoveTo), "Claude"); err != nil {
		t.Fatal(err)
	}
	exportReviewPNG(t, settings, filepath.Join(dir, "skills-settings-move-confirm-zh-Hans.png"))

	a, panel, _, _ := skillsPanel(t, skillsFixture())
	a.settings.SetLanguage(store.LanguageSimplifiedChinese)
	a.panelState.detail = store.ModuleSkills
	a.panelState.skills.open = "codex\x00" + skillsTestRoot + "/review/SKILL.md"
	panel.Frame()
	exportReviewPNG(t, panel, filepath.Join(dir, "skills-panel-detail-zh-Hans.png"))
}

// settingsSkillsWithIssues shows the Codex tab's diagnostics as issues,
// kept across the rescans a fix runs.
func settingsSkillsWithIssues(t *testing.T, issues func(sum skills.Summary) []skills.Issue) (*settingsUI, *ui.Tester, chan func(), *settingsSkillCalls) {
	t.Helper()
	u, tt, queue, calls := settingsSkillsView(t, false)
	res := settingsSkillsFixture()
	sum := res[store.ProviderCodex]
	sum.Issues = issues(sum)
	res[store.ProviderCodex] = sum
	calls.queryResult, u.skills.result, u.skills.showIssues = res, res, true
	tt.Frame()
	return u, tt, queue, calls
}

// clickFixConfirm clicks the confirmation's action, right of Cancel.
func clickFixConfirm(t *testing.T, tt *ui.Tester, l l10n.Localizer) {
	t.Helper()
	cancel, ok := tt.Find(l.Text(l10n.KeyCancel))
	if !ok {
		t.Fatalf("no confirmation; texts %q", tt.Texts())
	}
	tt.ClickAt(cancel.X+cancel.W+28, cancel.Y+cancel.H/2)
}

func TestSettingsSkillFixAddsDescriptionAfterRisks(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, calls := settingsSkillsWithIssues(t, func(sum skills.Summary) []skills.Issue {
		pdf := sum.Skills[1]
		return []skills.Issue{{Kind: skills.IssueMissingDesc, Path: pdf.File, Name: pdf.Name, Fixes: []skills.Fix{skills.FixAddDescription}}}
	})
	if err := tt.Click(tx(l, xSkillsFix)); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{tx(l, xSkillsFixRisks), tx(l, xSkillsRiskEditBackup), tx(l, xSkillsRiskEditUpdate)} {
		if !tt.HasText(text) {
			t.Fatalf("fix confirmation missing %q; texts %q", text, tt.Texts())
		}
	}
	if u.skills.fixDesc != "Suggested text" || len(calls.described) != 0 {
		t.Fatalf("opening wrote %v or lost the suggestion %q", calls.described, u.skills.fixDesc)
	}
	u.skills.fixDesc = "  Fill in PDF forms  "
	tt.Frame()
	clickFixConfirm(t, tt, l)
	settleSkills(t, tt, queue, 3)
	if len(calls.described) != 1 || calls.described[0] != "pdf: Fill in PDF forms" || !tt.HasText(tx(l, xSkillsFixedDesc)) {
		t.Fatalf("described %v; texts %q", calls.described, tt.Texts())
	}
}

func TestSettingsSkillFixDuplicateChoosesBetweenFixes(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, calls := settingsSkillsWithIssues(t, func(sum skills.Summary) []skills.Issue {
		review := sum.Skills[0]
		return []skills.Issue{{Kind: skills.IssueDuplicateName, Path: review.File, Name: review.Name, Detail: "/home/u/.agents/skills/review/SKILL.md",
			Fixes: []skills.Fix{skills.FixDisableCopy, skills.FixTrashCopy}}}
	})
	if err := tt.Click(tx(l, xSkillsFix)); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(tx(l, xSkillsRiskConfigBackup)) || !tt.HasText(tx(l, xSkillsFixTrash)) {
		t.Fatalf("duplicate fix choices missing; texts %q", tt.Texts())
	}
	if err := tt.Click(tx(l, xSkillsFixTrash)); err != nil {
		t.Fatal(err)
	}
	if u.skills.fixChoice != 1 || !tt.HasText(tx(l, xSkillsDeleteRestart)) || tt.HasText(tx(l, xSkillsRiskConfigBackup)) {
		t.Fatalf("choice %d; texts %q", u.skills.fixChoice, tt.Texts())
	}
	clickFixConfirm(t, tt, l)
	settleSkills(t, tt, queue, 3)
	if len(calls.deleted) != 1 || len(calls.enabled) != 0 {
		t.Fatalf("deleted %v, switched %v", calls.deleted, calls.enabled)
	}
}

func TestSettingsSkillFixLiftNamesWrapperRisk(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt, queue, calls := settingsSkillsWithIssues(t, func(sum skills.Summary) []skills.Issue {
		pdf := sum.Skills[1]
		wrapper := filepath.Dir(pdf.Dir)
		return []skills.Issue{{Kind: skills.IssueNested, Path: pdf.File, Name: pdf.Name, Fixes: []skills.Fix{skills.FixLiftNested},
			Lift: skills.NestedFix{Wrapper: wrapper, Lifts: []skills.Lift{{File: pdf.File, Name: pdf.Name, Target: filepath.Join(wrapper, "pdf")}}, Extra: 2}}}
	})
	if err := tt.Click(tx(l, xSkillsFix)); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(tx(l, xSkillsRiskReinstall)) || !tt.HasText("2 other items") {
		t.Fatalf("lift risks missing; texts %q", tt.Texts())
	}
	if err := tt.Click(l.Text(l10n.KeyCancel)); err != nil {
		t.Fatal(err)
	}
	if len(calls.lifted) != 0 {
		t.Fatal("cancel lifted the skill")
	}
	if err := tt.Click(tx(l, xSkillsFix)); err != nil {
		t.Fatal(err)
	}
	clickFixConfirm(t, tt, l)
	settleSkills(t, tt, queue, 3)
	if len(calls.lifted) != 1 || !tt.HasText(tx(l, xSkillsFixedLift, "/home/u/.claude/skills/pdf")) {
		t.Fatalf("lifted %v; texts %q", calls.lifted, tt.Texts())
	}
}

func TestSettingsSkillIssueWithoutFixHasNoButton(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt, _, _ := settingsSkillsWithIssues(t, func(sum skills.Summary) []skills.Issue {
		return []skills.Issue{{Kind: skills.IssueTruncated, Path: skillsTestRoot}}
	})
	if _, ok := tt.Find(tx(l, xSkillsFix)); ok {
		t.Fatalf("offered a fix for a truncated scan; texts %q", tt.Texts())
	}
}

func TestSettingsSkillFixLiftNamesWholeCollection(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt, _, _ := settingsSkillsWithIssues(t, func(sum skills.Summary) []skills.Issue {
		pdf := sum.Skills[1]
		wrapper := filepath.Dir(pdf.Dir)
		lifts := []skills.Lift{{File: pdf.File, Name: "pdf", Target: "/r/pdf"}, {File: "/x/SKILL.md", Name: "pdf-audit", Target: "/r/pdf-audit"}}
		return []skills.Issue{{Kind: skills.IssueNested, Path: pdf.File, Name: pdf.Name, Fixes: []skills.Fix{skills.FixLiftNested},
			Lift: skills.NestedFix{Wrapper: wrapper, Lifts: lifts}}}
	})
	if err := tt.Click(tx(l, xSkillsFix)); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(tx(l, xSkillsFixLiftAllMsg, "~/.agents", 2, "pdf, pdf-audit")) {
		t.Fatalf("collection lift message missing; texts %q", tt.Texts())
	}
}

func TestSettingsSkillRowOffersItsFix(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, _, _ := settingsSkillsWithIssues(t, func(sum skills.Summary) []skills.Issue {
		pdf := sum.Skills[1]
		return []skills.Issue{{Kind: skills.IssueMissingDesc, Path: pdf.File, Name: pdf.Name, Fixes: []skills.Fix{skills.FixAddDescription}}}
	})
	u.skills.showIssues = false
	tt.Frame()
	if _, ok := tt.Find(tx(l, xSkillsFix)); !ok {
		t.Fatalf("the row has no fix; texts %q", tt.Texts())
	}
	u.skills.open = u.skills.result[store.ProviderCodex].Skills[1].ID
	tt.Frame()
	if _, ok := tt.Find(tx(l, xSkillsFix)); !ok {
		t.Fatalf("the skill page has no fix; texts %q", tt.Texts())
	}
}

func TestSettingsSkillBatchDeleteAfterRisks(t *testing.T) {
	l := l10n.New(l10n.English)
	u, tt, queue, calls := settingsSkillsView(t, false)
	if err := tt.Click(tx(l, xSkillsSelectFiltered, 2)); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(tx(l, xSkillsSelectionCounts, 3, 2)) {
		t.Fatalf("select all missed; texts %q", tt.Texts())
	}
	if err := tt.Click(tx(l, xSkillsDeleteSelected, 2)); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(tx(l, xSkillsBatchDeleteMsg)) || !tt.HasText(tx(l, xSkillsDeleteShared, "Grok")) || len(calls.deleted) != 0 {
		t.Fatalf("no risks before deleting %v; texts %q", calls.deleted, tt.Texts())
	}
	clickFixConfirm(t, tt, l)
	settleSkills(t, tt, queue, 3)
	if len(calls.deleted) != 2 || len(u.skills.selected) != 0 || !tt.HasText(tx(l, xSkillsBatchDeleted, 2)) {
		t.Fatalf("deleted %v, selection %v; texts %q", calls.deleted, u.skills.selected, tt.Texts())
	}
}

func TestSettingsSkillBatchFixUsesFirstFixes(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt, queue, calls := settingsSkillsWithIssues(t, func(sum skills.Summary) []skills.Issue {
		review, pdf := sum.Skills[0], sum.Skills[1]
		return []skills.Issue{
			{Kind: skills.IssueMissingDesc, Path: pdf.File, Name: pdf.Name, Fixes: []skills.Fix{skills.FixAddDescription}},
			{Kind: skills.IssueDuplicateName, Path: review.File, Name: review.Name, Detail: "/home/u/.agents/skills/review/SKILL.md",
				Fixes: []skills.Fix{skills.FixDisableCopy, skills.FixTrashCopy}},
		}
	})
	if err := tt.Click(tx(l, xSkillsSelectFiltered, 2)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(l, xSkillsFixSelected, 2)); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{tx(l, xSkillsBatchDescNote), tx(l, xSkillsRiskConfigBackup), tx(l, xSkillsRiskEditBackup)} {
		if !tt.HasText(text) {
			t.Fatalf("batch fix missing %q; texts %q", text, tt.Texts())
		}
	}
	clickFixConfirm(t, tt, l)
	settleSkills(t, tt, queue, 3)
	if !slices.Equal(calls.enabled, []bool{false}) || !slices.Equal(calls.described, []string{"pdf: Suggested text"}) || len(calls.deleted) != 0 {
		t.Fatalf("switched %v, described %v, deleted %v", calls.enabled, calls.described, calls.deleted)
	}
	if !tt.HasText(tx(l, xSkillsBatchFixed, 2)) {
		t.Fatalf("no batch result; texts %q", tt.Texts())
	}
}

func TestSkillBatchDeletesSkipsFoldersInsideOthers(t *testing.T) {
	acts := SkillActions{Delete: func(skills.Skill, []skills.RootInfo) (bool, error) { return false, nil }}
	mk := func(path string) skills.Skill {
		return skills.Skill{Deletable: true, Removal: skills.Removal{Path: path}}
	}
	got := skillBatchDeletes(acts, []skills.Skill{mk("/r/pack/one"), mk("/r/pack"), mk("/r/package"), mk("/r/pack")})
	if len(got) != 2 || got[0].Removal.Path != "/r/pack" || got[1].Removal.Path != "/r/package" {
		t.Fatalf("deletes = %+v", got)
	}
}
