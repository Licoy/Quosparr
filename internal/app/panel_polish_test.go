package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/resettoday"
	"github.com/licoy/quosparr/internal/store"
)

func TestPanelIdentityStartsAtLeadingInset(t *testing.T) {
	snap := panelFixture()
	snap.Accounts.Display.Tier = codex.TierPro200
	_, tt := newPanel(t, nil, snap, PanelActions{})
	badge, ok := tt.Find("Pro 200")
	if !ok || badge.X < 15 || badge.X > 17 {
		t.Fatalf("tier badge must begin at the panel's 16pt inset, got %v (found %v)", badge, ok)
	}
	if os.Getenv("RUNWAY_UI_QA") != "" {
		snap := uiReviewSnapshot()
		snap.Accounts.Display.Tier = codex.TierPro200
		snap.Accounts.Display.DisplayName = "m***@***.com"
		expiry := time.Now().Add(11*24*time.Hour + 9*time.Hour)
		snap.Accounts.Display.SubscriptionExpiresAt = &expiry
		_, panel := newPanel(t, func(p *store.Preferences) { p.Language = store.LanguageSimplifiedChinese }, snap, PanelActions{})
		panel.SetScale(2)
		for _, dark := range []bool{false, true} {
			panel.SetDark(dark)
			name := "light"
			if dark {
				name = "dark"
			}
			exportReviewPNG(t, panel, filepath.Join("..", "..", ".build", "v1-ui-qa", "polish-panel-"+name+".png"))
		}
	}
}

func TestPanelExpiryUsesAvailableWidth(t *testing.T) {
	for _, lang := range l10n.Languages() {
		t.Run(string(lang), func(t *testing.T) {
			l := l10n.New(lang)
			now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local)
			expires := now.Add(11*24*time.Hour + 9*time.Hour + 32*time.Minute)
			remaining := expiryRemaining(l, expires, now)
			var naturalWidth float32
			tt := ui.NewTester(func(c *ui.Context) {
				applyTheme(c)
				naturalWidth, _ = c.MeasureText(1000, ui.Span{Text: remaining, Size: fsCaption, Weight: 500, Features: "tnum"})
				ui.Column(c).Fill().Padding(16).Children(func() {
					ui.Row(c).Children(func() { expiryBadge(c, l, expires, now) })
				})
			}, 400, 100)
			tt.SetScale(2)
			if os.Getenv("RUNWAY_UI_QA") != "" {
				exportReviewPNG(t, tt, filepath.Join("..", "..", ".build", "v1-ui-qa", "polish-expiry-"+string(lang)+".png"))
			}
			r, ok := tt.Find(remaining)
			if !ok || r.W+1 < naturalWidth || r.X+r.W > 384 {
				t.Fatalf("countdown clipped despite available panel space: bounds=%v naturalWidth=%v found=%v", r, naturalWidth, ok)
			}
		})
	}
}

func TestPanelHeatmapHoverShowsDayDetail(t *testing.T) {
	snap := uiReviewSnapshot()
	_, tt := newPanel(t, func(p *store.Preferences) { p.MainPanelHeight = 860 }, snap, PanelActions{})
	cell, ok := tt.Find("2026-10-11")
	if !ok {
		t.Fatal("heatmap day missing")
	}
	tt.Move(cell.X+cell.W/2, cell.Y+cell.H/2)
	tt.Frame()
	want := heatTooltip(l10n.New(l10n.English), store.ProviderCodex, "2026-10-11", snap.Codex.Heatmap.AllDevices["2026-10-11"], snap.Codex.Heatmap.AllDevices["2026-10-11"], snap.Codex.Heatmap.Local["2026-10-11"])
	if !tt.HasText(want) {
		t.Fatalf("hover did not show day details at %v", cell)
	}
	tip, _ := tt.Find(want)
	if tip.X < 0 || tip.X+tip.W > panelWidth {
		t.Fatalf("tooltip outside panel: %v", tip)
	}
	if os.Getenv("RUNWAY_UI_QA") != "" {
		exportReviewPNG(t, tt, filepath.Join("..", "..", ".build", "v1-ui-qa", "polish-heatmap.png"))
	}
	tt.Move(5, 5)
	if tt.HasText(want) {
		t.Fatal("tooltip remained after leaving heatmap")
	}
}

func TestPanelMetallicBadgeActuallyAnimates(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Padding(10).Children(func() { tierBadge(c, codexTierLook(c.Theme(), codex.TierPro200), "Pro 200") })
	}, 150, 45)
	before := append([]byte(nil), tt.Image().Pix...)
	deadline := tt.Now().Add(4 * time.Second)
	for tt.Now().Before(deadline) {
		tt.Advance(50 * time.Millisecond)
		if !bytes.Equal(before, tt.Image().Pix) {
			return
		}
	}
	t.Fatal("metallic badge stayed static through a complete Swift sheen cycle")
}

func TestPanelReactionClickFloatsBeforeResponse(t *testing.T) {
	queue := detailMainQueue(t)
	a, _ := newPanel(t, nil, panelFixture(), PanelActions{})
	defer a.cancelPanelDetails()
	snap := resettoday.ReactionDevMock(resettoday.MockNo, 321)
	a.panelState.reaction.snapshot = &snap
	a.panelState.startReaction = a.startReaction
	acts := PanelActions{
		QueryReaction: func(context.Context) (resettoday.ReactionSnapshot, error) { return snap, nil },
		React: func(ctx context.Context) (resettoday.ReactionPostResult, error) {
			<-ctx.Done()
			return resettoday.ReactionPostResult{}, ctx.Err()
		},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Padding(30).Children(func() { (resetCard{l: a.L()}).reactionButton(c, acts, &a.panelState) })
	}, 300, 100)
	if err := tt.Click(a.L().Text(l10n.KeyRateLimitResetTodayReactionPlease)); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("+1") || !tt.HasText("322") {
		t.Fatal("click did not immediately show the floating +1 and updated count")
	}
	start, _ := tt.Find("+1")
	// Animation frames use the tester's frozen clock, not wall time.
	a.panelState.reaction.deltaAt = tt.Now().Add(-800 * time.Millisecond)
	tt.Frame()
	floating, ok := tt.Find("+1")
	if !ok || floating.Y >= start.Y-8 {
		t.Fatalf("delta did not float up: %v -> %v", start, floating)
	}
	if os.Getenv("RUNWAY_UI_QA") != "" {
		exportReviewPNG(t, tt, filepath.Join("..", "..", ".build", "v1-ui-qa", "polish-reaction.png"))
	}
	a.panelState.reaction.deltaAt = tt.Now().Add(-3 * time.Second)
	tt.Frame()
	if tt.HasText("+1") {
		t.Fatal("floating delta did not disappear")
	}
	a.cancelPanelDetails()
	finishDetail(t, queue)
}

func TestPanelMetallicBadgeRespectsReduceMotion(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Padding(10).Children(func() { tierBadge(c, codexTierLook(c.Theme(), codex.TierPro200), "Pro 200") })
	}, 150, 45)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	before := append([]byte(nil), tt.Image().Pix...)
	tt.Advance(100 * time.Millisecond)
	if !bytes.Equal(before, tt.Image().Pix) {
		t.Fatal("badge animated with Reduce Motion enabled")
	}
}

func TestPanelReactionRollsBackFailures(t *testing.T) {
	for _, failure := range []error{errors.New("offline"), resettoday.ErrReactionRateLimited} {
		t.Run(failure.Error(), func(t *testing.T) {
			queue := detailMainQueue(t)
			a, _ := newPanel(t, nil, panelFixture(), PanelActions{})
			defer a.cancelPanelDetails()
			before := resettoday.ReactionDevMock(resettoday.MockNo, 321)
			remaining := 5
			before.Remaining, before.DailyLimit = &remaining, 5
			a.panelState.reaction.snapshot = &before
			a.startReaction(PanelActions{
				QueryReaction: func(context.Context) (resettoday.ReactionSnapshot, error) { return before, nil },
				React: func(context.Context) (resettoday.ReactionPostResult, error) {
					return resettoday.ReactionPostResult{}, failure
				},
			}, true)
			if *a.panelState.reaction.snapshot.Count != 322 {
				t.Fatal("optimistic count missing")
			}
			finishDetail(t, queue)
			state := a.panelState.reaction
			if *state.snapshot.Count != 321 || state.delta != 0 || state.task.Err != failure {
				t.Fatalf("failed click was not rolled back: %+v", state)
			}
			if state.snapshot.IsExhausted() != errors.Is(failure, resettoday.ErrReactionRateLimited) {
				t.Fatal("wrong remaining limit after failure")
			}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Padding(20).Children(func() { (resetCard{l: a.L()}).hero(c, PanelActions{}, &a.panelState) })
			}, 400, 180)
			message := tx(a.L(), xReactionFailed)
			if state.snapshot.IsExhausted() {
				message = a.L().Text(l10n.KeyRateLimitResetTodayReactionLimitReached)
			}
			if !tt.HasText(message) {
				t.Fatal("reaction failure was not visible")
			}
			if *before.Count != 321 || *before.Remaining != 5 {
				t.Fatal("mutated pre-click snapshot")
			}
		})
	}
}

func TestPanelReactionSuccessReconcilesCountAndDelta(t *testing.T) {
	for _, count := range []int{321, 322, 325} {
		before := resettoday.ReactionDevMock(resettoday.MockNo, 321)
		updated := resettoday.ReactionDevMock(resettoday.MockNo, count)
		optimistic := resettoday.OptimisticClick(before)
		state := panelReactionState{snapshot: &optimistic, delta: 1, deltaAt: time.Now()}
		state.finish(&before, updated, true, nil)
		expected := 1
		if count == 321 {
			expected = 0
		} else if count > 322 {
			expected = count - 322
		}
		if *state.snapshot.Count != count || state.delta != expected || state.task.Running || state.task.Err != nil {
			t.Fatalf("count %d reconciled incorrectly: %+v", count, state)
		}
	}
}
