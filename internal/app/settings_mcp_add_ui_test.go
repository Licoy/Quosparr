package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPAddPairRowsKeepFocusAndSecretValues(t *testing.T) {
	actions := mcpAddTestActions()
	u, _, _ := settingsMCPView(t, actions)
	e := mcpAddFixture(u)
	e.headers = []mcpPair{{id: e.newID(), key: "Z", value: "fixture-secret"}}
	l := u.L()
	headers := mcpPairsText{title: tx(l, xMCPHeaders), key: tx(l, xMCPHeaderName), value: tx(l, xMCPValue), add: tx(l, xMCPAddHeader)}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(20).Gap(10).Children(func() { u.mcpPairs(c, e, "headers", &e.headers, headers, true, "") })
	}, 600, 400)
	tt.Frame()
	if tt.HasText("fixture-secret") {
		t.Fatal("secret value shown before asking")
	}
	if err := tt.Click(tx(l, xMCPShowSecrets)); err != nil || !u.mcp.add.showSecrets {
		t.Fatal("show values toggle missing", err)
	}
	if err := tt.Click(headers.key); err != nil {
		t.Fatal(err)
	}
	tt.Command("selectAll")
	tt.Type("Authorization")
	tt.Type("-Fixture")
	if d, err := e.draft(); err != nil || d.Headers["Authorization-Fixture"] != "fixture-secret" || len(d.Headers) != 1 {
		t.Fatalf("renaming lost stable row or value; err=%v", err)
	}
	if err := tt.Click(headers.value); err != nil {
		t.Fatal(err)
	}
	tt.Command("selectAll")
	tt.Type("new-fixture")
	if d, _ := e.draft(); d.Headers["Authorization-Fixture"] != "new-fixture" {
		t.Fatal("value edit failed")
	}
	e.transport = "stdio"
	env := mcpPairsText{title: tx(l, xMCPEnv), key: tx(l, xMCPVariable), value: tx(l, xMCPValue), add: tx(l, xMCPAddVariable)}
	tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() { u.mcpPairs(c, e, "env", &e.env, env, true, "") })
	}, 600, 400)
	tt.Frame()
	if err := tt.Click(env.add); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(env.key); err != nil {
		t.Fatal(err)
	}
	tt.Type("FIXTURE_ENV")
	if err := tt.Click(env.value); err != nil {
		t.Fatal(err)
	}
	tt.Type("x")
	if d, _ := e.draft(); d.Env["FIXTURE_ENV"] != "x" {
		t.Fatal("env row typing failed")
	}
}
func TestMCPAddErrorsAreLocalizedAndRedacted(t *testing.T) {
	u, _, _ := settingsMCPView(t, mcpAddTestActions())
	if strings.Contains(u.mcpAddError(errors.New("fixture-secret")), "fixture-secret") {
		t.Fatal("unknown error leaked")
	}
	message := u.mcpAddError(&mcp.ProbeError{Code: "http_status", Stage: "tools", HTTPStatus: 401, RPCCode: -32001, ExitCode: 2})
	for _, part := range []string{"401", "-32001", "2"} {
		if !strings.Contains(message, part) {
			t.Fatal("safe error detail lost")
		}
	}
	for k, texts := range extraTexts {
		if strings.HasPrefix(string(k), "mcpAdd") {
			for i, text := range texts {
				if strings.TrimSpace(text) == "" {
					t.Fatalf("missing locale %d for %s", i, k)
				}
			}
		}
	}
}
func TestMCPAddNarrowWindowAndFixtureScreenshot(t *testing.T) {
	u, tt, _ := settingsMCPView(t, mcpAddTestActions())
	mcpAddFixture(u)
	for _, width := range []int{820, 980} {
		tt.SetSize(width, 760)
		tt.Frame()
		if !tt.HasText(tx(u.L(), xMCPAdd)) {
			t.Fatal("wizard heading missing")
		}
		r, ok := tt.Find(tx(u.L(), xMCPNext))
		if !ok || r.X < 0 || r.X+r.W > float32(width) {
			t.Fatalf("navigation outside width %d: %+v", width, r)
		}
		if dir := os.Getenv("QUOSPARR_TEST_SCREENSHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("mcp-add-fixture-%d.png", width)))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Leaving for another settings tab erases the shared secret draft.
	u.tab = tabGeneral
	tt.Frame()
	if u.mcp.add != nil {
		t.Fatal("navigation retained draft")
	}
}

func TestMCPAddBackLinkLeavesFromEveryStep(t *testing.T) {
	for _, step := range []int{mcpStepConfigure, mcpStepTools, mcpStepReview, mcpStepResults} {
		t.Run(fmt.Sprint(step), func(t *testing.T) {
			u, tt, _ := settingsMCPView(t, mcpAddTestActions())
			mcpAddFixture(u)
			u.mcp.add.step = step
			u.mcp.add.completed = step == mcpStepResults
			tt.Frame()
			if err := tt.Click(u.L().Text(l10n.KeyBack)); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if (u.mcp.add != nil && u.mcp.add.open) || !tt.HasText("alpha") {
				t.Fatal("back link did not return to the server list")
			}
		})
	}
}

func TestMCPAddUnverifiedTargetChangesRemainActionable(t *testing.T) {
	actions := mcpAddTestActions()
	var writes []store.Provider
	actions.Add = func(_ context.Context, p store.Provider, _ mcp.AddPlan) (mcp.AddResult, error) {
		writes = append(writes, p)
		return mcp.AddResult{Added: []string{"fixture"}}, nil
	}
	actions.Query = func(context.Context, store.Provider) (mcp.Snapshot, error) {
		return mcp.Snapshot{Fingerprint: "added-v2"}, nil
	}
	u, tt, queue := settingsMCPView(t, actions)
	mcpAddFixture(u)
	w := u.mcp.add
	w.step = mcpStepTools
	tt.Frame()
	if err := tt.Click(tx(u.L(), xMCPUnverified)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(u.L(), xMCPNext)); err != nil {
		t.Fatal(err)
	}
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	if w.step != mcpStepReview || !w.unverified || len(w.plans) != 1 {
		t.Fatal("unverified acknowledgement was lost or the target was not checked")
	}
	tt.Frame()
	if err := tt.Click(u.L().Text(providerKey(store.ProviderClaude))); err != nil {
		t.Fatal(err)
	}
	// Hold the two-provider completion until the newer three-provider check lands.
	oldCallback := takeMCPCallback(t, queue)
	unrelatedDelivered := 0
	for range 2 {
		queue <- func() { unrelatedDelivered++ }
	}
	if err := tt.Click(u.L().Text(providerKey(store.ProviderGrok))); err != nil {
		t.Fatal(err)
	}
	finishMCPUntil(t, queue, func() bool { return unrelatedDelivered == 2 && !w.task.Running })
	oldCallback()
	if !w.unverified {
		t.Fatal("provider change erased draft acknowledgement")
	}
	if w.step != mcpStepReview || len(w.plans) != 3 || w.task.Running {
		t.Fatal("unverified three-provider path was silently blocked")
	}
	tt.Frame()
	if err := tt.Click(tx(u.L(), xMCPSubmit, 3)); err != nil {
		t.Fatal(err)
	}
	if !w.writing || w.step != mcpStepResults {
		t.Fatal("submit did not start the confirmed write")
	}
	finishMCPUntil(t, queue, func() bool { return !w.task.Running && !w.writing && !u.mcp.task.Running })
	if !w.completed || len(writes) != 3 || u.mcp.snapshot.Fingerprint != "added-v2" {
		t.Fatal("confirmed three-provider write or owner refresh did not finish")
	}
}
func TestMCPAddTransportSwitchKeepsFieldsAndArgsDelete(t *testing.T) {
	u, _, _ := settingsMCPView(t, mcpAddTestActions())
	e := mcpAddFixture(u)
	l := u.L()
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).Padding(20).Gap(10).Children(func() { u.mcpDraftForm(c, e) }) }, 650, 900)
	tt.Frame()
	if err := tt.Click(tx(l, xMCPTransportStdio)); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.draft(); e.transport != "stdio" || d.URL != "" {
		t.Fatal("switched draft kept the URL")
	}
	tt.Frame()
	if err := tt.Click("HTTP"); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.draft(); d.URL != "https://mcp.example.test" {
		t.Fatal("switching back lost the URL")
	}
	e.transport, e.command = "stdio", "fixture"
	e.args = []mcpArg{{id: e.newID(), value: "one"}, {id: e.newID(), value: "two"}, {id: e.newID(), value: "three"}}
	tt.Frame()
	if err := tt.Click(tx(l, xMCPRemove)); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.draft(); len(d.Args) != 2 || d.Args[0] != "two" {
		t.Fatal("argument removal failed")
	}
}

func TestMCPAddSevenLanguageWorkflowLayouts(t *testing.T) {
	u, tt, _ := settingsMCPView(t, mcpAddTestActions())
	e := mcpAddFixture(u)
	w := u.mcp.add
	e.headers = []mcpPair{{id: e.newID(), key: "Authorization", value: "fixture-hidden-value"}}
	e.probed = true
	e.probe = mcp.ProbeResult{ServerName: "Fixture MCP", ServerVersion: "1.0", ProtocolVersion: "2025-11-25", Tools: []mcp.Tool{{Name: "search", Title: "Search fixtures", Description: "Read-only fixture search; never invoked by this preview", InputSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`, Parameters: []mcp.ToolParameter{{Name: "query", Type: "string", Required: true, Description: "Fixture query"}}}}}
	e.expanded[0] = true
	for _, language := range l10n.Languages() {
		u.s.loc = l10n.New(language)
		for _, width := range []int{820, 980} {
			for _, dark := range []bool{false, true} {
				tt.SetSize(width, 720)
				tt.SetDark(dark)
				for _, step := range []int{mcpStepConfigure, mcpStepTools, mcpStepReview} {
					w.step = step
					if step == mcpStepReview {
						w.plans = []mcpAddTarget{{provider: store.ProviderCodex, plan: mcp.AddPlan{Items: []mcp.AddItem{{Name: "fixture", Status: "add", Details: `http · https://mcp.example.test · headers: Authorization=[hidden]`}}}}}
					}
					tt.Frame()
					if !tt.HasText(tx(u.L(), xMCPAdd)) {
						t.Fatalf("missing wizard locale=%s step=%d", language, step)
					}
					if step == mcpStepConfigure && tt.HasText("fixture-hidden-value") {
						t.Fatal("password value appeared in form text")
					}
					if language == l10n.English || language == l10n.SimplifiedChinese {
						if dir := os.Getenv("QUOSPARR_TEST_SCREENSHOTS"); dir != "" {
							if err := os.MkdirAll(dir, 0700); err != nil {
								t.Fatal(err)
							}
							path := filepath.Join(dir, fmt.Sprintf("mcp-%s-%d-dark%t-step%d.png", language, width, dark, step))
							f, err := os.Create(path)
							if err != nil {
								t.Fatal(err)
							}
							if err = png.Encode(f, tt.Image()); err != nil {
								t.Fatal(err)
							}
							if err = f.Close(); err != nil {
								t.Fatal(err)
							}
						}
					}
				}
			}
		}
	}
}

func TestMCPAddPreviewFailureAlsoShowsCleanupWarning(t *testing.T) {
	u, tt, _ := settingsMCPView(t, mcpAddTestActions())
	e := mcpAddFixture(u)
	u.mcp.add.step = mcpStepTools
	e.task = Task{Err: errors.New("fixture-private-server-error")}
	e.probe = mcp.ProbeResult{CleanupFailed: true}
	tt.Frame()
	if !tt.HasText(tx(u.L(), xMCPCleanupWarning)) || !tt.HasText(u.mcpAddError(e.task.Err)) {
		t.Fatal("failed preview lost its error or cleanup warning")
	}
	if tt.HasText("fixture-private-server-error") {
		t.Fatal("preview exposed a raw error")
	}
}
