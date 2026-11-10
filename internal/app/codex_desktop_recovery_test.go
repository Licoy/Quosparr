package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexdesktop"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

func recoveryUITest(t *testing.T, l l10n.Localizer) (*ui.Tester, **codexRecoveryDialog, chan func(), *int) {
	t.Helper()
	writes := new(int)
	d := &codexRecoveryDialog{loaded: true, selected: -1, target: codexdesktop.Owner{UserID: "current", AccountID: "current-workspace"}, sources: []codexdesktop.Source{
		{Key: "first-private-hash", Projects: 2, Threads: 99, Names: []string{"Project Alpha"}},
		{Key: "second-private-hash", Projects: 1, Threads: 25, Names: []string{"Project Beta"}},
	}}
	d.prepare = func(r codexdesktop.Request) (*codexdesktop.Plan, error) {
		if r.SourceKey != "second-private-hash" || r.Target != d.target {
			return nil, errors.New("wrong request")
		}
		return &codexdesktop.Plan{Summary: codexdesktop.Summary{Projects: 1, Threads: 3, Skipped: 2, Reasons: []string{"missing_thread", "remote_project"}}}, nil
	}
	d.apply = func(target codexdesktop.Owner, p *codexdesktop.Plan) (codexdesktop.Outcome, error) {
		*writes++
		return "", codexdesktop.ErrPartialWrite
	}
	queue := make(chan func(), 8)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { renderCodexRecovery(c, l, &d, nil, false, func(fn func()) { queue <- fn }) })
	}, 620, 760)
	return tt, &d, queue, writes
}

func hasSummary(tt *ui.Tester, l l10n.Localizer, projects, threads, skipped int) bool {
	_, ok := tt.Find(tx(l, xProjectSummary, projects, threads, skipped))
	return ok
}

func recoveryDelivery(t *testing.T, queue chan func()) {
	t.Helper()
	select {
	case fn := <-queue:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("no recovery UI delivery")
	}
}

func TestCodexRecoveryPreviewSelectionFailureAndCancel(t *testing.T) {
	l := l10n.New(l10n.English)
	tt, slot, queue, writes := recoveryUITest(t, l)
	if err := tt.Click(tx(l, xProjectConfirm)); err != nil {
		t.Fatal(err)
	}
	if (*slot).busy || (*slot).plan != nil || *writes != 0 {
		t.Fatal("confirmation without explicit source started")
	}
	label := desktopSourceLabel(l, (*slot).sources[1], 1, nil, false)
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	if (*slot).selected != 1 || !(*slot).busy {
		t.Fatal("selecting a source did not start its preview")
	}
	recoveryDelivery(t, queue)
	tt.Frame()
	if !hasSummary(tt, l, 1, 3, 2) || *writes != 0 {
		t.Fatalf("preview did not validate counts: %v", tt.Texts())
	}
	if err := tt.Click(tx(l, xProjectConfirm)); err != nil {
		t.Fatal(err)
	}
	recoveryDelivery(t, queue)
	tt.Frame()
	if *writes != 1 || (*slot).selected != 1 || (*slot).plan == nil || (*slot).done {
		t.Fatal("failure lost selection or reported success")
	}
	if !tt.HasText(tx(l, xProjectPartial)) {
		t.Fatalf("partial error missing: %v", tt.Texts())
	}
	if err := tt.Click(l.Text(l10n.KeyCancel)); err != nil {
		t.Fatal(err)
	}
	if *slot != nil || *writes != 1 {
		t.Fatal("cancel wrote data or retained dialog")
	}
}

func TestCodexRecoveryCancelBeforeConfirmationDoesNotWrite(t *testing.T) {
	l := l10n.New(l10n.English)
	tt, slot, _, writes := recoveryUITest(t, l)
	if err := tt.Click(l.Text(l10n.KeyCancel)); err != nil {
		t.Fatal(err)
	}
	if *slot != nil || *writes != 0 {
		t.Fatal("cancel changed data")
	}
}

func TestCodexRecoveryEntryAndErrors(t *testing.T) {
	l := l10n.New(l10n.English)
	rt := &liveApp{codex: &codex.Store{}}
	for _, api := range []bool{false, true} {
		mode := codex.AuthModeOAuth
		if api {
			mode = codex.AuthModeAPIKey
		}
		view := runway.AccountsView{ActiveID: "current", Sidebar: []codex.ManagedAccount{{ID: "current", AuthMode: mode}}}
		if got := codexRecoveryAvailable(rt, view); got != (codexDesktopShareSupported && !api) {
			t.Fatalf("eligibility api=%v got=%v", api, got)
		}
		var d *codexRecoveryDialog
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Children(func() { codexRecoveryEntry(c, l, &d, rt, view, false, false, func(fn func()) { fn() }) })
		}, 620, 180)
		if shown := tt.HasText(tx(l, xProjectRecover)); shown != (codexDesktopShareSupported && !api) {
			t.Fatalf("recovery entry shown=%v for api=%v", shown, api)
		}
	}
	for _, err := range []error{codexdesktop.ErrPartialWrite, codexdesktop.ErrConflict, codexdesktop.ErrStateChanged, runway.ErrDesktopProjectTargetChanged, runway.ErrDesktopProjectRestart, errors.New("private-token-internal-path")} {
		for _, lang := range l10n.Languages() {
			text := desktopRecoveryError(l10n.New(lang), err)
			if text == "" || strings.Contains(text, "private-token") || strings.Contains(text, "runway:") || strings.Contains(text, "codexdesktop:") {
				t.Fatalf("unsafe error %q", text)
			}
		}
	}
	source := codexdesktop.Source{Key: "private-hash", Projects: 2, Names: []string{"Private Project"}}
	if label := desktopSourceLabel(l, source, 0, nil, true); strings.Contains(label, "Private") || strings.Contains(label, "hash") {
		t.Fatalf("privacy leaked %q", label)
	}
}

func TestCodexRecoveryShareRemainsOptIn(t *testing.T) {
	l := l10n.New(l10n.English)
	on := false
	undo := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { codexShareOption(c, l, &on, false, func() { undo++ }) })
	}, 620, 340)
	if on {
		t.Fatal("sharing enabled by default")
	}
	if err := tt.Click(tx(l, xCodexDesktopShare)); err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("sharing could not be enabled")
	}
	if err := tt.Click(tx(l, xCodexDesktopRestore)); err != nil {
		t.Fatal(err)
	}
	if undo != 1 {
		t.Fatal("undo action unavailable")
	}
}

func TestCodexRecoveryReviewArtifact(t *testing.T) {
	dir := os.Getenv("QUOSPARR_CODEX_RECOVERY_REVIEW_DIR")
	if dir == "" {
		t.Skip("optional recovery screenshots")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	l := l10n.New(l10n.SimplifiedChinese)
	tt, slot, queue, _ := recoveryUITest(t, l)
	if err := tt.Click(desktopSourceLabel(l, (*slot).sources[1], 1, nil, false)); err != nil {
		t.Fatal(err)
	}
	recoveryDelivery(t, queue)
	tt.Frame()
	exportReviewPNG(t, tt, filepath.Join(dir, "recovery-preview.png"))
	if err := tt.Click(tx(l, xProjectConfirm)); err != nil {
		t.Fatal(err)
	}
	recoveryDelivery(t, queue)
	tt.Frame()
	exportReviewPNG(t, tt, filepath.Join(dir, "recovery-partial.png"))
}

func TestCodexRecoveryLoadsOffThreadAndRejectsChangedTarget(t *testing.T) {
	l := l10n.New(l10n.English)
	queue := make(chan func(), 4)
	entered, release := make(chan struct{}), make(chan struct{})
	d := &codexRecoveryDialog{selected: -1}
	target := codexdesktop.Owner{UserID: "current", AccountID: "workspace"}
	d.load = func() (codexdesktop.Owner, []codexdesktop.Source, error) {
		close(entered)
		<-release
		return target, []codexdesktop.Source{{Key: "source", Projects: 1}}, nil
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { renderCodexRecovery(c, l, &d, nil, false, func(fn func()) { queue <- fn }) })
	}, 620, 760)
	<-entered
	if !d.busy || d.loaded {
		t.Fatal("source read blocked UI or enabled preview early")
	}
	close(release)
	recoveryDelivery(t, queue)
	tt.Frame()
	if d.target != target || !d.loaded || d.selected != -1 {
		t.Fatal("load did not fix target or auto-selected a source")
	}
	d.plan = &codexdesktop.Plan{Summary: codexdesktop.Summary{Projects: 1}}
	d.selected = 0
	d.apply = func(got codexdesktop.Owner, p *codexdesktop.Plan) (codexdesktop.Outcome, error) {
		if got != target {
			t.Error("UI changed preview target")
		}
		return "", runway.ErrDesktopProjectTargetChanged
	}
	tt.Frame()
	if err := tt.Click(tx(l, xProjectConfirm)); err != nil {
		t.Fatal(err)
	}
	recoveryDelivery(t, queue)
	tt.Frame()
	if d.done || d.selected != 0 || !tt.HasText(tx(l, xProjectChanged)) {
		t.Fatalf("changed target was not rejected: %v", tt.Texts())
	}
}

func TestCodexRecoveryRecheckFiltersTargetAndPreservesSelection(t *testing.T) {
	l := l10n.New(l10n.English)
	tt, slot, queue, _ := recoveryUITest(t, l)
	d := *slot
	d.selected = 1
	d.failure = runway.ErrDesktopProjectTargetChanged
	d.plan = &codexdesktop.Plan{}
	target := d.target
	d.load = func() (codexdesktop.Owner, []codexdesktop.Source, error) {
		return target, []codexdesktop.Source{{Key: target.Key(), Projects: 99}, d.sources[1]}, nil
	}
	tt.Frame()
	if err := tt.Click(tx(l, xProjectRecheck)); err != nil {
		t.Fatal(err)
	}
	if d.plan != nil || d.loaded {
		t.Fatal("recheck retained stale plan")
	}
	recoveryDelivery(t, queue)
	tt.Frame()
	if len(d.sources) != 1 || d.sources[0].Key == target.Key() || d.selected != 0 {
		t.Fatal("self-source was offered or explicit selection was lost")
	}
}

func TestCodexRecoverySuccessfulSkipsRemainVisible(t *testing.T) {
	l := l10n.New(l10n.English)
	tt, slot, queue, _ := recoveryUITest(t, l)
	d := *slot
	d.selected = 1
	d.plan = &codexdesktop.Plan{Summary: codexdesktop.Summary{Projects: 1, Threads: 3, Skipped: 2, Reasons: []string{"missing_thread"}}}
	d.apply = func(codexdesktop.Owner, *codexdesktop.Plan) (codexdesktop.Outcome, error) {
		return codexdesktop.SharedWithSkips, nil
	}
	tt.Frame()
	if err := tt.Click(tx(l, xProjectConfirm)); err != nil {
		t.Fatal(err)
	}
	recoveryDelivery(t, queue)
	tt.Frame()
	if !d.done || !hasSummary(tt, l, 1, 3, 2) || !tt.HasText(tx(l, xProjectSkipThread)) {
		t.Fatalf("successful skip summary hidden: %v", tt.Texts())
	}
}

func TestCodexRecoveryCombinedWriteAndRestartErrors(t *testing.T) {
	l := l10n.New(l10n.English)
	for _, failure := range []error{codexdesktop.ErrPartialWrite, codexdesktop.ErrConflict} {
		err := errors.Join(runway.ErrDesktopProjects, failure, runway.ErrCodexRuntime, runway.ErrDesktopProjectRestart)
		text := desktopRecoveryError(l, err)
		if !strings.Contains(text, desktopRecoveryPrimaryError(l, failure)) || !strings.Contains(text, tx(l, xProjectReopen)) || strings.Contains(text, tx(l, xProjectStopFailed)) || strings.Contains(text, tx(l, xProjectRestartFailed)) {
			t.Fatalf("combined error misreported: %q", text)
		}
	}
}

func TestCodexRecoveryEmptyPreviewCannotConfirm(t *testing.T) {
	l := l10n.New(l10n.English)
	tt, slot, queue, writes := recoveryUITest(t, l)
	d := *slot
	d.selected = 1
	d.plan = &codexdesktop.Plan{Summary: codexdesktop.Summary{Skipped: 2, Reasons: []string{"missing_thread"}}}
	tt.Frame()
	if !tt.HasText(tx(l, xProjectNothingRecoverable)) || !tt.HasText(tx(l, xProjectSkipThread)) {
		t.Fatalf("empty preview explanation missing: %v", tt.Texts())
	}
	if err := tt.Click(tx(l, xProjectConfirm)); err != nil {
		t.Fatal(err)
	}
	d.run("apply", func(fn func()) { queue <- fn })
	if d.busy || *writes != 0 || d.done {
		t.Fatal("empty recovery executed or reported success")
	}
	d.undo = true
	if d.emptyPreview() {
		t.Fatal("empty-summary undo incorrectly disabled")
	}
	for _, language := range l10n.Languages() {
		loc := l10n.New(language)
		if got := desktopRecoveryError(loc, codexdesktop.ErrValidation); got != tx(loc, xProjectSkipUnknown) {
			t.Fatalf("validation error not localized: %q", got)
		}
	}
}
