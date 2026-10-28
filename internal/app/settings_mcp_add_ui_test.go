package app

import (
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

func TestMCPAddMapRowsKeepFocusAndSecretValues(t *testing.T) {
	actions := mcpAddTestActions()
	u, _, _ := settingsMCPView(t, actions)
	e := mcpAddFixture(u)
	e.draft.Headers = map[string]string{"Z": "fixture-secret"}
	u.mcp.add.showSecrets = true
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(20).Gap(10).Children(func() { u.mcpDraftMap(c, e, "headers", xMCPHeaders, &e.draft.Headers) })
	}, 600, 400)
	tt.Frame()
	if err := tt.Click(tx(u.L(), xMCPKey)); err != nil {
		t.Fatal(err)
	}
	tt.Command("selectAll")
	tt.Type("Authorization")
	tt.Type("-Fixture")
	if e.draft.Headers["Authorization-Fixture"] != "fixture-secret" || len(e.draft.Headers) != 1 {
		t.Fatalf("renaming lost stable row or value; keys=%d", len(e.draft.Headers))
	}
	if err := tt.Click(tx(u.L(), xMCPValue)); err != nil {
		t.Fatal(err)
	}
	tt.Command("selectAll")
	tt.Type("new-fixture")
	if e.draft.Headers["Authorization-Fixture"] != "new-fixture" {
		t.Fatal("value edit failed")
	}
	e.draft.Env = map[string]string{}
	tt = ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() { u.mcpDraftMap(c, e, "env", xMCPEnv, &e.draft.Env) })
	}, 600, 400)
	tt.Frame()
	if err := tt.Click(tx(u.L(), xMCPAddPair)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(u.L(), xMCPKey)); err != nil {
		t.Fatal(err)
	}
	tt.Type("FIXTURE_ENV")
	if _, ok := e.draft.Env["FIXTURE_ENV"]; !ok {
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

func TestMCPAddUnverifiedTargetChangesRemainActionable(t *testing.T) {
	actions := mcpAddTestActions()
	u, tt, queue := settingsMCPView(t, actions)
	mcpAddFixture(u)
	w := u.mcp.add
	w.step = 1
	tt.Frame()
	if err := tt.Click(tx(u.L(), xMCPUnverified)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(tx(u.L(), xMCPNext)); err != nil {
		t.Fatal(err)
	}
	if w.step != 2 || !w.unverified {
		t.Fatal("unverified acknowledgement was lost")
	}
	for _, p := range []store.Provider{store.ProviderClaude, store.ProviderGrok} {
		if err := tt.Click(u.L().Text(providerKey(p))); err != nil {
			t.Fatal(err)
		}
	}
	if !w.unverified {
		t.Fatal("provider change erased draft acknowledgement")
	}
	if err := tt.Click(tx(u.L(), xMCPNext)); err != nil {
		t.Fatal(err)
	}
	finishDetail(t, queue)
	if w.step != 3 || len(w.plans) != 3 {
		t.Fatal("unverified three-provider path was silently blocked")
	}
}
func TestMCPAddTransportSwitchConfirmsClearingAndArgsDelete(t *testing.T) {
	u, _, _ := settingsMCPView(t, mcpAddTestActions())
	e := mcpAddFixture(u)
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).Padding(20).Gap(10).Children(func() { u.mcpDraftForm(c, e) }) }, 650, 900)
	tt.Frame()
	if err := tt.Click(tx(u.L(), xMCPTransport)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("stdio"); err != nil {
		t.Fatal(err)
	}
	if e.draft.Transport != "http" || e.draft.URL == "" {
		t.Fatal("transport cleared fields before confirmation")
	}
	if err := tt.Click(u.text(l10n.KeyOk)); err != nil {
		t.Fatal(err)
	}
	if e.draft.Transport != "stdio" || e.draft.URL != "" {
		t.Fatal("confirmed transport did not clear incompatible URL")
	}
	e.draft.Command = "fixture"
	e.draft.Args = []string{"one", "two", "three"}
	tt.Frame()
	if err := tt.Click("−"); err != nil {
		t.Fatal(err)
	}
	if len(e.draft.Args) != 2 || e.draft.Args[0] != "two" {
		t.Fatal("argument removal failed")
	}
}

func TestMCPAddSevenLanguageWorkflowLayouts(t *testing.T) {
	u, tt, _ := settingsMCPView(t, mcpAddTestActions())
	e := mcpAddFixture(u)
	w := u.mcp.add
	e.draft.Headers = map[string]string{"Authorization": "fixture-hidden-value"}
	e.probed = true
	e.probe = mcp.ProbeResult{ServerName: "Fixture MCP", ServerVersion: "1.0", ProtocolVersion: "2025-11-25", Tools: []mcp.Tool{{Name: "search", Title: "Search fixtures", Description: "Read-only fixture search; never invoked by this preview", InputSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`, Parameters: []mcp.ToolParameter{{Name: "query", Type: "string", Required: true, Description: "Fixture query"}}}}}
	e.expanded[0] = true
	for _, language := range l10n.Languages() {
		u.s.loc = l10n.New(language)
		for _, width := range []int{820, 980} {
			for _, dark := range []bool{false, true} {
				tt.SetSize(width, 720)
				tt.SetDark(dark)
				for _, step := range []int{0, 1, 3} {
					w.step = step
					if step == 3 {
						w.plans = []mcpAddTarget{{provider: store.ProviderCodex, plan: mcp.AddPlan{Items: []mcp.AddItem{{Name: "fixture", Status: "add", Details: `http · https://mcp.example.test · headers: Authorization=[hidden]`}}}}}
					}
					tt.Frame()
					if !tt.HasText(tx(u.L(), xMCPAdd)) {
						t.Fatalf("missing wizard locale=%s step=%d", language, step)
					}
					if step == 0 && tt.HasText("fixture-hidden-value") {
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
	u.mcp.add.step = 1
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
