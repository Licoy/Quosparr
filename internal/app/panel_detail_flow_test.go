package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/resettoday"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

func detailMainQueue(t *testing.T) chan func() {
	t.Helper()
	queue := make(chan func(), 20)
	t.Cleanup(setRunOnMain(func(f func()) { queue <- f }))
	return queue
}
func finishDetail(t *testing.T, queue chan func()) {
	t.Helper()
	select {
	case f := <-queue:
		f()
	case <-time.After(3 * time.Second):
		t.Fatal("detail operation did not finish")
	}
}
func receiveCost(t *testing.T, queries chan CostQuery) CostQuery {
	t.Helper()
	select {
	case q := <-queries:
		return q
	case <-time.After(3 * time.Second):
		t.Fatal("no real range query")
		return CostQuery{}
	}
}

func TestPanelCostRangeControlsQueryAndRenderResults(t *testing.T) {
	queue := detailMainQueue(t)
	queries := make(chan CostQuery, 8)
	acts := PanelActions{QueryCost: func(_ context.Context, _ store.Provider, q CostQuery) (usage.Summary, error) {
		queries <- q
		s := *uiReviewDetails(panelFixture()).Codex.DisplayedCost
		s.Totals.TotalTokens = 1234
		return s, nil
	}}
	a, tt := newPanel(t, nil, panelFixture(), acts)
	a.panelState.detail = store.ModuleAPICost
	tt.Frame()
	if got := receiveCost(t, queries); got.Range != a.settings.Prefs().APICostSummaryRange {
		t.Fatal("initial range ignored preferences")
	}
	finishDetail(t, queue)
	tt.Frame()
	if !tt.HasText("1.23K") {
		t.Fatalf("query result not rendered: %q", tt.Texts())
	}
	for _, r := range store.CostRanges() {
		if err := tt.Click(a.L().Text(costRangeKey(r))); err != nil {
			t.Fatal(err)
		}
		if got := receiveCost(t, queries); got.Range != r || got.Custom != nil {
			t.Fatalf("wrong query: %#v", got)
		}
		finishDetail(t, queue)
		tt.Frame()
	}
	if a.settings.Prefs().APICostSummaryRange != store.DefaultPreferences().APICostSummaryRange {
		t.Fatal("detail selection modified summary preference")
	}
	if err := tt.Click(a.L().Text(l10n.KeyCustomRange)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.panelState.cost.start = time.Date(2026, 10, 2, 12, 0, 0, 0, time.Local)
	a.panelState.cost.end = time.Date(2026, 10, 4, 12, 0, 0, 0, time.Local)
	tt.Frame()
	if err := tt.Click(a.L().Text(l10n.KeyCalculate)); err != nil {
		t.Fatal(err)
	}
	q := receiveCost(t, queries)
	if q.Custom == nil || q.Custom.APIStartDate != "2026-10-02" || q.Custom.APIEndDate != "2026-10-04" {
		t.Fatalf("wrong custom query: %#v", q)
	}
	finishDetail(t, queue)
	tt.Frame()
	a.panelState.cost.start = a.panelState.cost.end.AddDate(0, 0, 1)
	tt.Frame()
	if err := tt.Click(a.L().Text(l10n.KeyCalculate)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(a.L().Text(l10n.KeyInvalidDateRange)) {
		t.Fatal("invalid range hidden")
	}
	select {
	case <-queries:
		t.Fatal("invalid range sent query")
	default:
	}
}

func TestPanelCostDropsCancelledAndChangedAccountResults(t *testing.T) {
	for _, change := range []string{"range", "account", "provider", "close", "runtime"} {
		t.Run(change, func(t *testing.T) {
			queue := detailMainQueue(t)
			started := make(chan context.Context, 2)
			release := make(chan struct{})
			acts := PanelActions{QueryCost: func(ctx context.Context, _ store.Provider, _ CostQuery) (usage.Summary, error) {
				started <- ctx
				<-release
				return usage.Summary{Totals: usage.Totals{TotalTokens: 999}}, nil
			}}
			a, tt := newPanel(t, nil, panelFixture(), acts)
			a.panelState.detail = store.ModuleAPICost
			tt.Frame()
			var ctx context.Context
			select {
			case ctx = <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("query not started")
			}
			switch change {
			case "range":
				a.panelState.cost.selected = 4
				a.startCostQuery(CostQuery{Range: store.CostRangeToday}, PanelActions{QueryCost: func(context.Context, store.Provider, CostQuery) (usage.Summary, error) {
					return usage.Summary{}, errors.New("new result")
				}})
			case "account":
				a.snap.Accounts.ActiveID = "other" // completion must reject before next frame
			case "provider":
				a.settings.SetSelectedProvider(store.ProviderGrok)
			case "close":
				a.cancelPanelDetails()
			case "runtime":
				a.SetPanelActions(PanelActions{})
			}
			close(release)
			finishDetail(t, queue)
			if change == "range" {
				finishDetail(t, queue)
			}
			if a.panelState.cost.summary != nil {
				t.Fatal("stale account/range result applied")
			}
			if change == "close" || change == "runtime" {
				if ctx.Err() == nil || a.panelState.cost.task.Running {
					t.Fatal("closed query was not cancelled")
				}
			}
		})
	}
}

func TestPanelCostQueryErrorIsVisibleAndLocalized(t *testing.T) {
	queue := detailMainQueue(t)
	a, tt := newPanel(t, func(p *store.Preferences) { p.Language = store.LanguageSimplifiedChinese }, panelFixture(), PanelActions{QueryCost: func(context.Context, store.Provider, CostQuery) (usage.Summary, error) {
		return usage.Summary{}, errGrokCycleUnavailable
	}})
	a.panelState.detail = store.ModuleAPICost
	tt.Frame()
	// Other main-thread posts may be queued ahead of the query's completion.
	for a.panelState.cost.task.Running {
		finishDetail(t, queue)
	}
	tt.Frame()
	if !tt.HasText(tx(a.L(), xGrokCycleUnavailable)) {
		t.Fatal("query error not localized")
	}
}

func TestPanelEstimateHistoryAndUnavailableReason(t *testing.T) {
	snap := panelFixture()
	reason := estimate.ReasonMissingCredits
	previous := estimate.HistorySample{CycleStartDate: "2026-09-28", EstimatedCredits: 1600, UsedPercent: 40, RecordedAt: time.Now()}
	older := previous
	older.CycleStartDate = "2026-09-21"
	previous.EstimatedCredits = 1800
	snap.Codex.Estimate = &estimate.Snapshot{History: []estimate.HistorySample{older, previous}, PreviousEstimate: &previous, UnavailableReason: &reason, CurrentRows: []estimate.Row{{Date: "2026-10-12", Credits: 25, CreditsReported: true, Tokens: 1800}}}
	a, tt := newPanel(t, nil, snap, PanelActions{})
	a.panelState.detail = store.ModuleQuotaEstimate
	tt.Frame()
	rows := estimateHistoryRows(*snap.Codex.Estimate)
	if len(rows) != 2 || rows[0].change == nil || *rows[0].change != 12.5 {
		t.Fatal("history delta was not calculated from adjacent cycles")
	}
	for _, text := range []string{a.L().Text(l10n.KeyQuotaEstimateHistory), "2026-09-28", "+13%", a.L().Text(l10n.KeyQuotaEstimateMissingCredits), "2026-10-12"} {
		if !tt.HasText(text) {
			t.Fatalf("estimate detail missing %q", text)
		}
	}
}

func TestResetReactionPostsOnlyOnClickAndShowsLimit(t *testing.T) {
	queue := detailMainQueue(t)
	var posts atomic.Int32
	client := &resettoday.ReactionClient{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		code, body := 200, `{"ok":true,"data":{"enabled":true,"ready":true,"polarity":"yes","epochId":"fixture","count":5,"remaining":1,"dailyLimit":1}}`
		if r.Method == http.MethodPost {
			posts.Add(1)
			code = 429
			body = `{"ok":false,"error":"daily_limit"}`
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}, URL: "https://fixture.invalid/reaction"}
	a, tt := newPanel(t, nil, panelFixture(), PanelActions{QueryReaction: client.Fetch, React: client.Click})
	finishDetail(t, queue)
	tt.Frame()
	if posts.Load() != 0 {
		t.Fatal("opening the main page posted feedback")
	}
	if a.panelState.detail != "" || !tt.HasText("5") {
		t.Fatal("main-page reaction count missing")
	}
	label := a.L().Text(l10n.KeyRateLimitResetTodayReactionThank)
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	finishDetail(t, queue)
	tt.Frame()
	if posts.Load() != 1 || !tt.HasText(a.L().Text(l10n.KeyRateLimitResetTodayReactionLimitReached)) {
		t.Fatal("feedback limit not shown")
	}
	if !a.panelState.reaction.snapshot.IsExhausted() {
		t.Fatal("rate-limit response did not disable feedback")
	}
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 {
		t.Fatal("exhausted feedback posted again")
	}
}
