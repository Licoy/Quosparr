package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

func skillAddWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillAddText(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\nBody of " + name + ".\n"
}

// skillAddFixture is a folder of two skills, one nested, and the add page
// of a settings window whose actions read, check and write for real into
// a throwaway home.
func skillAddFixture(t *testing.T) (*settingsUI, *ui.Tester, chan func(), skills.Env, string) {
	t.Helper()
	home := t.TempDir()
	env := skills.Env{Home: home, CodexHome: filepath.Join(home, ".codex"), ClaudeDir: filepath.Join(home, ".claude"),
		GrokHome: filepath.Join(home, ".grok"), GOOS: runtime.GOOS, Now: time.Unix(1_700_000_000, 0)}
	src := t.TempDir()
	skillAddWrite(t, filepath.Join(src, "README.md"), "not a skill")
	skillAddWrite(t, filepath.Join(src, "skills", "pdf", "SKILL.md"), skillAddText("pdf", "Fill PDF forms"))
	skillAddWrite(t, filepath.Join(src, "skills", "pdf", "node_modules", "x.js"), "dependency")
	skillAddWrite(t, filepath.Join(src, "skills", "docs", "word", "SKILL.md"), skillAddText("word", "Edit documents"))
	u, tt, queue, _ := settingsSkillsView(t, false)
	acts := u.skillActions()
	acts.LoadImport = func(ctx context.Context, s skills.Source, _ func(int64, int64)) (*skills.Snapshot, []skills.Candidate, error) {
		snap, err := skills.LoadFolder(ctx, s.Path)
		if err != nil {
			return nil, nil, err
		}
		found, err := skills.Discover(snap)
		return snap, found, err
	}
	acts.PrepareInstall = func(target skills.InstallTarget, sums []skills.Summary, snap *skills.Snapshot, picked []skills.Candidate) (skills.InstallPlan, error) {
		return skills.PrepareInstall(env, target, sums, snap, picked)
	}
	acts.Install = func(ctx context.Context, plan skills.InstallPlan) (skills.InstallResult, error) {
		return skills.Install(ctx, plan, env.Now)
	}
	u.opts.Skills = func() SkillActions { return acts }
	tt.Frame()
	return u, tt, queue, env, src
}

func TestSkillAddReadsPicksAndInstallsAFolder(t *testing.T) {
	u, tt, queue, env, src := skillAddFixture(t)
	l := u.L()
	if err := tt.Click(tx(l, xSkillAdd)); err != nil {
		t.Fatal(err)
	}
	w := u.skills.add
	if w == nil || !w.open || !w.targets[skills.TargetCodex] {
		t.Fatal("add page did not open with the current CLI as target")
	}
	if err := tt.Click(tx(l, xSkillAddFolder)); err != nil {
		t.Fatal(err)
	}
	w.folder = src
	tt.Frame()
	if err := tt.Click(tx(l, xMCPNext)); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 1)
	if w.step != skillAddStepPick || len(w.found) != 2 || len(w.pickedSkills()) != 0 {
		t.Fatalf("step %d, found %v, picked %d", w.step, w.found, len(w.pickedSkills()))
	}
	for _, text := range []string{"pdf", "word", "Fill PDF forms", tx(l, xSkillAddFound, 2)} {
		if !hasExact(tt, text) {
			t.Fatalf("pick step missing %q; texts %q", text, tt.Texts())
		}
	}
	if err := tt.Click(tx(l, xSkillAddPickOne, "pdf")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(l, xSkillAddShowDetails, "pdf")); err != nil {
		t.Fatal(err)
	}
	if !hasExact(tt, "SKILL.md") || tt.HasText("x.js") {
		t.Fatalf("details show the wrong files; texts %q", tt.Texts())
	}
	if err := tt.Click(tx(l, xMCPNext)); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 1)
	plan := w.plan(skills.TargetCodex)
	if w.step != skillAddStepReview || plan == nil || plan.err != nil || len(plan.plan.Items) != 1 || plan.plan.Items[0].Status != skills.InstallAdd {
		t.Fatalf("review plan %+v", plan)
	}
	if err := tt.Click(tx(l, xSkillAddShared)); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 1)
	if !hasExact(tt, tx(l, xSkillAddSharedOverlap)) || w.plan(skills.TargetShared) == nil {
		t.Fatalf("shared target not checked or overlap not named; texts %q", tt.Texts())
	}
	if err := tt.Click(tx(l, xSkillAddSubmit, 2)); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 2) // the write, then the rescan
	if w.writing || !w.completed || skillAddFailed(w.plans) || !hasExact(tt, tx(l, xSkillAddResultsDone)) {
		t.Fatalf("results %+v; texts %q", w.plans, tt.Texts())
	}
	for _, dir := range []string{filepath.Join(env.CodexHome, "skills", "pdf"), filepath.Join(env.Home, ".agents", "skills", "pdf")} {
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "node_modules")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("dependencies were installed")
		}
	}
	if err := tt.Click(tx(l, xMCPDone)); err != nil {
		t.Fatal(err)
	}
	if u.skills.add != nil {
		t.Fatal("done did not close the page")
	}
}

func TestSkillAddChangingSourceDropsWhatWasRead(t *testing.T) {
	u, tt, queue, _, src := skillAddFixture(t)
	u.openSkillAdd()
	w := u.skills.add
	w.kind, w.folder = 1, filepath.Join(src, "skills", "pdf")
	u.loadSkillSource(u.skillActions())
	settleSkills(t, tt, queue, 1)
	if w.step != skillAddStepPick || len(w.pickedSkills()) != 1 {
		t.Fatalf("a single skill was not picked; step %d", w.step)
	}
	w.step = skillAddStepSource
	tt.Frame()
	if err := tt.Click(tx(u.L(), xMCPNext)); err != nil || w.step != skillAddStepPick {
		t.Fatal("the same source was read again", err)
	}
	w.step = skillAddStepSource
	w.kind = 0
	w.sourceChanged()
	if w.snap != nil || w.found != nil || w.plans != nil {
		t.Fatal("a new source kept the old one's skills")
	}
	w.github = "not a repo"
	u.loadSkillSource(u.skillActions())
	if importCode(w.load.Err) != skills.ImportInvalidSource {
		t.Fatalf("invalid source: %v", w.load.Err)
	}
	tt.Frame()
	if !tt.HasText(tx(u.L(), xSkillAddErrInvalidSource)) {
		t.Fatalf("source error not shown; texts %q", tt.Texts())
	}
}

func TestSkillAddRetriesOnlyFailedTargets(t *testing.T) {
	u, tt, queue, _, src := skillAddFixture(t)
	acts := u.skillActions()
	var installs []skills.InstallTarget
	real := acts.Install
	acts.Install = func(ctx context.Context, plan skills.InstallPlan) (skills.InstallResult, error) {
		installs = append(installs, plan.Target)
		if plan.Target == skills.TargetClaude && len(installs) < 3 {
			return skills.InstallResult{}, &skills.ImportError{Code: skills.ImportChanged}
		}
		return real(ctx, plan)
	}
	u.opts.Skills = func() SkillActions { return acts }
	u.openSkillAdd()
	w := u.skills.add
	w.kind, w.folder = 1, filepath.Join(src, "skills", "pdf")
	u.loadSkillSource(acts)
	settleSkills(t, tt, queue, 1)
	w.targets[skills.TargetClaude] = true
	w.step = skillAddStepReview
	u.prepareSkillInstall(acts, false)
	settleSkills(t, tt, queue, 1)
	u.submitSkillInstall(acts)
	settleSkills(t, tt, queue, 2)
	if !skillAddFailed(w.plans) || !w.plan(skills.TargetCodex).done || w.plan(skills.TargetClaude).done {
		t.Fatalf("plans %+v", w.plans)
	}
	l := u.L()
	if !tt.HasText(tx(l, xSkillAddErrChanged)) {
		t.Fatalf("failure not reported; texts %q", tt.Texts())
	}
	if err := tt.Click(tx(l, xSkillAddRetry)); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 1)
	if len(w.plans) != 2 || !w.plan(skills.TargetCodex).done || w.plan(skills.TargetClaude).done {
		t.Fatalf("retry plans %+v", w.plans)
	}
	u.submitSkillInstall(acts)
	settleSkills(t, tt, queue, 2)
	if !slices.Equal(installs, []skills.InstallTarget{skills.TargetCodex, skills.TargetClaude, skills.TargetClaude}) || skillAddFailed(w.plans) {
		t.Fatalf("installs %v, plans %+v", installs, w.plans)
	}
}

func TestSkillAddConfirmedWriteSurvivesClosingTheWindow(t *testing.T) {
	u, tt, queue, _, src := skillAddFixture(t)
	session := &skillAddSession{owner: u}
	u.opts.skillAddSession = session
	acts := u.skillActions()
	release := make(chan struct{})
	real := acts.Install
	acts.Install = func(ctx context.Context, plan skills.InstallPlan) (skills.InstallResult, error) {
		<-release
		return real(ctx, plan)
	}
	u.opts.Skills = func() SkillActions { return acts }
	u.openSkillAdd()
	w := u.skills.add
	w.kind, w.folder = 1, filepath.Join(src, "skills", "pdf")
	u.loadSkillSource(acts)
	settleSkills(t, tt, queue, 1)
	w.step = skillAddStepReview
	u.prepareSkillInstall(acts, false)
	settleSkills(t, tt, queue, 1)
	u.submitSkillInstall(acts)
	u.closeSkillAdd()
	if session.wizard != w || !w.writing {
		t.Fatal("closing dropped a confirmed write")
	}
	next := newSettingsUI(u.s, SettingsOptions{skillAddSession: session, Skills: func() SkillActions { return acts }})
	next.skillsState().result = u.skills.result
	next.openSkillAdd()
	if next.skills.add != w {
		t.Fatal("a new window did not take over the write")
	}
	close(release)
	finishDetail(t, queue) // the write
	finishDetail(t, queue) // the new window's rescan
	if !w.completed || !w.plan(skills.TargetCodex).done || next.skills.result == nil {
		t.Fatal("the result did not reach the new window")
	}
	next.finishSkillAdd()
	if session.wizard != nil {
		t.Fatal("finished results were kept")
	}
	next.openSkillAdd()
	next.closeSkillAdd()
	if session.wizard != nil || next.skills.add != nil {
		t.Fatal("an unconfirmed page survived closing")
	}
}

func TestSkillAddShowsDownloadProgressAndTheErrorOnce(t *testing.T) {
	u, tt, queue, _, _ := skillAddFixture(t)
	l := u.L()
	release := make(chan struct{})
	acts := u.opts.Skills()
	acts.LoadImport = func(ctx context.Context, src skills.Source, progress func(int64, int64)) (*skills.Snapshot, []skills.Candidate, error) {
		progress(50<<20, 200<<20)
		<-release
		return nil, nil, &skills.ImportError{Code: skills.ImportTooLarge}
	}
	u.opts.Skills = func() SkillActions { return acts }
	if err := tt.Click(tx(l, xSkillAdd)); err != nil {
		t.Fatal(err)
	}
	u.skills.add.github = "acme/tools"
	tt.Frame()
	if err := tt.Click(tx(l, xMCPNext)); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 1)
	if !hasExact(tt, tx(l, xSkillAddDownloading, 25)) {
		t.Fatalf("no progress; texts %q", tt.Texts())
	}
	close(release)
	settleSkills(t, tt, queue, 1)
	shown := 0
	for _, text := range tt.Texts() {
		if text == tx(l, xSkillAddErrTooLarge) {
			shown++
		}
	}
	if shown != 1 {
		t.Fatalf("error shown %d times; texts %q", shown, tt.Texts())
	}
	if got := skillAddLoadText(l, 3<<20, 0); got != tx(l, xSkillAddDownloadingSize, "3.0 MB") {
		t.Fatalf("unknown total = %q", got)
	}
	if got := skillAddLoadText(l, 300, 200); got != tx(l, xSkillAddDownloading, 99) {
		t.Fatalf("past the estimate = %q", got)
	}
}

func TestSkillAddErrorsAreLocalized(t *testing.T) {
	u, _, _, _, _ := skillAddFixture(t)
	l := u.L()
	if got := skillAddErrorText(l, errors.New("secret detail")); strings.Contains(got, "secret") || got != tx(l, xSkillAddFailed) {
		t.Fatalf("unknown error = %q", got)
	}
	if got := skillAddErrorText(l, &skills.ImportError{Code: skills.ImportHTTP, Detail: "502"}); got != tx(l, xSkillAddErrHTTP)+" · 502" {
		t.Fatalf("http error = %q", got)
	}
	if got := skillAddErrorText(l, context.Canceled); got != tx(l, xSkillAddErrCancelled) {
		t.Fatalf("cancel = %q", got)
	}
	for k, texts := range extraTexts {
		if strings.HasPrefix(string(k), "skillAdd") || strings.HasPrefix(string(k), "skillUpdate") {
			for i, text := range texts {
				if strings.TrimSpace(text) == "" {
					t.Fatalf("missing locale %d for %s", i, k)
				}
			}
		}
	}
}

func TestSkillPageChecksAndUpdatesASkillFromGitHub(t *testing.T) {
	u, tt, queue, calls := settingsSkillsView(t, false)
	res := settingsSkillsFixture()
	sum := res[store.ProviderCodex]
	sum.Skills[0].Origin = skills.Origin{From: skills.OriginQuosparr, Installer: string(skills.SourceGitHub), Repository: settingsSkillsRepo,
		GitHub: "acme/agent-skills", SkillPath: "skills/review", Commit: strings.Repeat("a", 40)}
	res[store.ProviderCodex] = sum
	calls.queryResult = res
	u.skills.result = res
	acts := u.skillActions()
	var forced []bool
	acts.CheckUpdate = func(context.Context, skills.Skill) skills.UpdateCheck {
		return skills.UpdateCheck{State: skills.UpdateAvailable, Commit: strings.Repeat("b", 40)}
	}
	acts.Update = func(_ context.Context, req skills.UpdateRequest) (skills.UpdateResult, error) {
		forced = append(forced, req.Force)
		if !req.Force {
			return skills.UpdateResult{}, &skills.ImportError{Code: skills.ImportModified}
		}
		return skills.UpdateResult{Path: req.Skill.Dir}, nil
	}
	u.opts.Skills = func() SkillActions { return acts }
	tt.Frame()
	if err := tt.Click("review"); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 1) // Inspect
	l := u.L()
	for _, text := range []string{tx(l, xSkillsInstQuosparr, "GitHub"), "aaaaaaa"} {
		if !hasExact(tt, text) {
			t.Fatalf("origin missing %q; texts %q", text, tt.Texts())
		}
	}
	if err := tt.Click(tx(l, xSkillUpdateCheck)); err != nil {
		t.Fatal(err)
	}
	settleSkills(t, tt, queue, 1)
	if !hasExact(tt, tx(l, xSkillUpdateAvailable, "bbbbbbb")) {
		t.Fatalf("no update offered; texts %q", tt.Texts())
	}
	update := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		cancel, ok := tt.Find(l.Text(l10n.KeyCancel))
		if !ok || !tt.HasText(tx(l, xSkillUpdateTitle)) {
			t.Fatalf("no confirmation for %q; texts %q", label, tt.Texts())
		}
		tt.ClickAt(cancel.X+cancel.W+8+20, cancel.Y+cancel.H/2)
		settleSkills(t, tt, queue, 3) // the update, the rescan and the trash
	}
	update(tx(l, xSkillUpdate))
	if !u.skills.update.force || !u.skills.noticeErr || !hasExact(tt, tx(l, xSkillAddErrModified)) {
		t.Fatalf("a modified copy was not reported; texts %q", tt.Texts())
	}
	update(tx(l, xSkillUpdateForce))
	if !slices.Equal(forced, []bool{false, true}) || u.skills.notice != tx(l, xSkillUpdated) {
		t.Fatalf("forced %v, notice %q", forced, u.skills.notice)
	}
}

func TestSkillAddUIReviewPNGs(t *testing.T) {
	if os.Getenv("RUNWAY_UI_QA") == "" {
		t.Skip("set RUNWAY_UI_QA to export Skill UI PNGs")
	}
	dir := filepath.Join("..", "..", ".build", "v1-ui-qa")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	u, _, queue, _, src := skillAddFixture(t)
	u.s.SetLanguage(store.LanguageSimplifiedChinese)
	skillAddWrite(t, filepath.Join(src, "skills", "nodesc", "SKILL.md"), "---\nname: nodesc\n---\n")
	tt := ui.NewTester(u.view, settingsMinWidth, settingsHeight)
	u.openSkillAdd()
	w := u.skills.add
	w.github = "https://github.com/acme/agent-skills/tree/main/skills"
	tt.Frame()
	exportReviewPNG(t, tt, filepath.Join(dir, "skill-add-source-zh-Hans.png"))
	w.kind, w.folder = 1, src
	u.loadSkillSource(u.skillActions())
	for w.load.Running {
		finishDetail(t, queue)
	}
	tt.Frame()
	if w.snap == nil {
		t.Fatalf("load: %v", w.load.Err)
	}
	w.picked["skills/pdf"] = true
	w.expanded["skills/pdf"] = true
	tt.Frame()
	exportReviewPNG(t, tt, filepath.Join(dir, "skill-add-pick-zh-Hans.png"))
	w.targets[skills.TargetShared] = true
	w.step = skillAddStepReview
	u.prepareSkillInstall(u.skillActions(), false)
	settleSkills(t, tt, queue, 1)
	exportReviewPNG(t, tt, filepath.Join(dir, "skill-add-review-zh-Hans.png"))
	u.submitSkillInstall(u.skillActions())
	settleSkills(t, tt, queue, 2)
	exportReviewPNG(t, tt, filepath.Join(dir, "skill-add-results-zh-Hans.png"))
}
