package app

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestGrokSubscriptionLooksKeepOldStyles(t *testing.T) {
	pairs := [][2]grok.TierKind{
		{grok.TierXSubscriptionPlus, grok.TierSuperGrokLite},
		{grok.TierXSubscriptionPremium, grok.TierSuperGrok},
		{grok.TierXSubscriptionSuper, grok.TierSuperGrokPlus},
		{grok.TierXSubscriptionUltra, grok.TierSuperGrokHeavy},
	}
	for _, dark := range []bool{false, true} {
		for _, pair := range pairs {
			theme := &ui.Theme{Dark: dark}
			if !reflect.DeepEqual(grokTierLook(theme, pair[0]), grokTierLook(theme, pair[1])) {
				t.Fatalf("new tier %s changed the corresponding old style", pair[0].DisplayName())
			}
		}
	}
}

func TestGrokPlansAgreeAcrossSurfacesAndFitNarrowRows(t *testing.T) {
	l := l10n.New(l10n.English)
	a, _ := newPanel(t, nil, runway.Snapshot{}, PanelActions{})
	plans := []string{"X · Plus", "X · Premium", "X · Super", "X · Ultra", "SuperGrok Lite", "SuperGrok", "SuperGrok Plus", "SuperGrok Heavy", "X Premium", "Tier 9", strings.Repeat("W", 80), strings.Repeat("W", 81), "https://invalid.test", "{\"token\":\"unsafe\"}", "unsafe\nname"}
	for _, plan := range plans {
		for _, dark := range []bool{false, true} {
			for _, width := range []int{320, 640} {
				for _, surface := range []string{"header", "card", "settings"} {
					t.Run(fmt.Sprintf("%s/%d/dark=%t/%d", surface, width, dark, len(plan)), func(t *testing.T) {
						q := runway.GrokAccountQuota{Plan: plan, PlanError: "HTTP 503"}
						acc := codex.ManagedAccount{ID: "one", DisplayName: "A long account display name for layout"}
						snap := runway.Snapshot{Grok: runway.GrokState{Plan: plan, PlanError: q.PlanError, Accounts: runway.AccountsView{Display: codex.AccountDisplay{DisplayName: acc.DisplayName}}}}
						tt := ui.NewTester(func(c *ui.Context) {
							ui.Column(c).Fill().Padding(12).Children(func() {
								switch surface {
								case "header":
									prefs := store.DefaultPreferences()
									prefs.SelectedProvider = store.ProviderGrok
									grokIdentityRow(c, a, l, prefs, snap)
								case "card":
									a.grokCardHeader(c, l, acc, q, false, false, runway.AccountsView{}, AccountActions{})
									grokQuotaBlock(c, l, &q, accountQuotaOptions{})
								case "settings":
									accountSettingsIdentity(c, l, accountIdentityView{account: acc, provider: store.ProviderGrok, grokQuota: &q})
								}
							})
						}, width, 500)
						tt.SetDark(dark)
						want, safe := grok.SafeTierDisplayName(plan)
						if !safe {
							want = "Grok"
						}
						r, found := tt.Find(want)
						if !found || r.X < 0 || r.X+r.W > float32(width)+0.5 {
							t.Fatalf("badge %q missing or outside %dpx: %v; %q", want, width, r, tt.Texts())
						}
						if name, ok := tt.Find(acc.DisplayName); !ok || name.X < 0 || name.X+name.W > float32(width)+0.5 {
							t.Fatalf("identity exceeds %dpx: %v", width, name)
						}
						if !tt.HasText(tx(l, xGrokPlanUnavailable)) {
							t.Fatal("metadata failure must be visible without hiding quota")
						}
						if !safe && tt.HasText(plan) {
							t.Fatal("unsafe cached plan displayed")
						}
					})
				}
			}
		}
	}
}

func TestGrokPlanProjectionPreservesMetadataAndWidgetName(t *testing.T) {
	l := l10n.New(l10n.English)
	old := "SuperGrok"
	for _, system := range []string{"BILLING_SYSTEM_COMMERCE", "BILLING_SYSTEM_LEGACY", "BILLING_SYSTEM_FUTURE", ""} {
		snap := grok.BillingSnapshot{Plan: &old, SubscriptionError: "HTTP 503", IncludedUsagePercent: 25, Source: grok.BillingSourceCurrent, UpdatedAt: time.Now()}
		if system != "" {
			snap.Subscription = &grok.SubscriptionInfo{Tier: "SUBSCRIPTION_TIER_GROK_PRO", BillingSystem: system}
		}
		plan, billing, failure := grokPlanProjection(snap)
		want := old
		if system == "BILLING_SYSTEM_COMMERCE" {
			want = "X · Premium"
		}
		if plan != want || failure != "HTTP 503" || (billing == "BILLING_SYSTEM_LEGACY") != (system == "BILLING_SYSTEM_LEGACY") {
			t.Fatalf("metadata projection wrong: %q %q %q", plan, billing, failure)
		}
		data, err := json.Marshal(snap)
		if err != nil {
			t.Fatal(err)
		}
		q, ok := grokAccountQuota(l, grok.ManagedAccount{CachedQuota: data})
		if !ok || q.Plan != plan || q.PlanBillingSystem != billing || q.PlanError != failure || len(q.Meters) == 0 {
			t.Fatalf("account projection diverged or lost quota: %+v", q)
		}
		widget := grokWidget(runway.GrokState{Plan: plan})
		if widget.Plan == nil || *widget.Plan != plan {
			t.Fatal("widget label diverged from the panel")
		}
	}
	unsafe := "{\"token\":\"unsafe\"}"
	if w := grokWidget(runway.GrokState{Plan: unsafe}); w.Plan != nil {
		t.Fatal("widget accepted unsafe cached plan")
	}
	for _, reason := range []string{unsafe, "HTTP 503 response body", "HTTP 999", "timeout\nunsafe"} {
		if safeGrokPlanError(reason) != "request failed" {
			t.Fatal("unsafe metadata diagnostic accepted")
		}
	}
}

func TestGrokBadgePreviewArtifact(t *testing.T) {
	dir := os.Getenv("QUOSPARR_TEST_BADGE_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set QUOSPARR_TEST_BADGE_PREVIEW_DIR for offscreen PNGs")
	}
	l := l10n.New(l10n.English)
	plans := []string{"X · Plus", "X · Premium", "X · Super", "X · Ultra", "SuperGrok Lite", "SuperGrok", "SuperGrok Plus", "SuperGrok Heavy", "X Premium", "Tier 9", strings.Repeat("W", 80), strings.Repeat("W", 81)}
	for _, dark := range []bool{false, true} {
		tt := ui.NewTester(func(c *ui.Context) {
			ground := pick(dark, ui.RGB(30, 30, 34), ui.RGB(246, 246, 248))
			ui.Column(c).Fill().Background(ground).Padding(20).Gap(10).Children(func() {
				for i, plan := range plans {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Text(c, fmt.Sprintf("%02d", i+1)).TextColor(c.Theme().Text).Width(24)
						grokPlanBadge(c, l, plan)
					})
				}
			})
		}, 420, 510)
		tt.SetDark(dark)
		path := filepath.Join(dir, fmt.Sprintf("grok-badges-dark-%t.png", dark))
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("write PNG: %v %v", err, closeErr)
		}
		t.Log(path)
	}
}

func TestGrokPlanStageDiagnosticsAreSafeAndLocalized(t *testing.T) {
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		for _, stage := range []string{"", "user: ", "subscriptions: "} {
			for _, category := range []string{"request failed", "timeout", "canceled", "authentication required", "invalid response", "identity mismatch", "subscription mismatch", "HTTP 403"} {
				reason := stage + category
				if got := safeGrokPlanError(reason); got != reason {
					t.Fatalf("lost safe diagnostic %q: %q", reason, got)
				}
				text := grokPlanErrorText(l, reason)
				if category == "HTTP 403" && !strings.Contains(text, "403") {
					t.Fatal("lost HTTP evidence")
				}
				if language != l10n.English && (strings.Contains(text, "subscriptions:") || strings.Contains(text, "invalid response") || strings.Contains(text, "authentication required")) {
					t.Fatalf("untranslated diagnostic %q", text)
				}
			}
		}
		text := grokPlanErrorText(l, "subscriptions: {\"unsafe\":1}")
		if strings.Contains(text, "unsafe") || strings.Contains(text, "{") {
			t.Fatal("upstream response leaked")
		}
	}
}

func TestGrokPlanMainAndAccountProjectionMatch(t *testing.T) {
	l := l10n.New(l10n.English)
	s := grok.NewStore(t.TempDir(), t.TempDir())
	idx := grok.NewAccountIndex()
	idx.CurrentAccountID = "current"
	for i, id := range []string{"current", "other"} {
		data, err := json.Marshal(grok.BillingSnapshot{Source: grok.BillingSourceCurrent, UpdatedAt: time.Now(), IncludedUsagePercent: 25,
			Subscription: &grok.SubscriptionInfo{Tier: "SUBSCRIPTION_TIER_SUPER_GROK_PLUS", BillingSystem: "BILLING_SYSTEM_COMMERCE"}, SubscriptionError: "subscriptions: HTTP 403"})
		if err != nil {
			t.Fatal(err)
		}
		idx.Accounts = append(idx.Accounts, grok.ManagedAccount{ID: id, SortIndex: i, CachedQuota: data, CreatedAt: time.Now(), Identity: grok.CredentialIdentity{Kind: grok.AuthKindOAuth}})
	}
	if err := s.SaveIndex(idx); err != nil {
		t.Fatal(err)
	}
	st := buildGrokState(grok.NewModule(s), l)
	if st.Availability != runway.GrokReady || st.LastError != "" || st.Plan != "X · Super" || st.PlanBillingSystem != "BILLING_SYSTEM_COMMERCE" || st.PlanError != "subscriptions: HTTP 403" {
		t.Fatalf("main projection lost metadata or promoted metadata failure to quota failure: %+v", st)
	}
	for _, q := range st.AccountQuotas {
		if q.Plan != st.Plan || q.PlanBillingSystem != st.PlanBillingSystem || q.PlanError != st.PlanError {
			t.Fatal("main and non-current account metadata diverged")
		}
	}
	if st.Widget == nil || st.Widget.Plan == nil || *st.Widget.Plan != st.Plan {
		t.Fatal("widget does not match metadata projection")
	}
}

func TestGrokPlanTooltipsDistinguishExplicitLegacy(t *testing.T) {
	l := l10n.New(l10n.English)
	for _, tc := range []struct{ plan, system, tip string }{
		{"SuperGrok", "", ""},
		{"X · Future", "", ""},
		{"SuperGrok", "BILLING_SYSTEM_LEGACY", "Legacy subscription"},
		{"X · Premium", "BILLING_SYSTEM_COMMERCE", "The X Subscription plan"},
	} {
		tt := ui.NewTester(func(c *ui.Context) { grokPlanBadgeInfo(c, l, grokPlanInfo{plan: tc.plan, billingSystem: tc.system}) }, 320, 200)
		r, ok := tt.Find(tc.plan)
		if !ok {
			t.Fatal("missing badge")
		}
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tt.Advance(time.Second)
		if tc.tip != "" && !tt.HasText(tc.tip) {
			t.Fatalf("missing product explanation: %q", tt.Texts())
		}
		if tc.system == "" && (tt.HasText("Legacy subscription") || tt.HasText("The X Subscription plan")) {
			t.Fatal("missing metadata falsely labeled legacy")
		}
	}
}
