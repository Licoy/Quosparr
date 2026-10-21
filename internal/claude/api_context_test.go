package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestAPIProfileContext1M(t *testing.T) {
	p := APIProfile{Model: "main", OpusModel: "opus[1M]", SonnetModel: "sonnet", HaikuModel: "haiku", FableModel: "fable"}
	original := p.Env("test")
	p.Context1M = true
	got := p.Env("test")
	if got.Model != "main[1m]" || got.OpusModel != "opus[1M]" || got.SonnetModel != "sonnet[1m]" || got.HaikuModel != "haiku" || got.FableModel != "fable" {
		t.Fatal("unexpected 1M model mapping")
	}
	if p.Model != "main" {
		t.Fatal("profile model was mutated")
	}
	p.Context1M = false
	if p.Env("test") != original {
		t.Fatal("disabling did not restore entered models")
	}
	if e := (APIProfile{Context1M: true}).Env("test"); e.Model != "" || e.OpusModel != "" || e.SonnetModel != "" {
		t.Fatal("empty models gained suffixes")
	}
}

func TestAPIContext1MEditApplyAndCleanup(t *testing.T) {
	for _, apply := range []bool{false, true} {
		t.Run(map[bool]string{false: "save-only", true: "apply"}[apply], func(t *testing.T) {
			r := newAPIRig(t)
			ctx := context.Background()
			oauth := r.loginOAuth(t, "test-access-a", "acct-a")
			acc := r.addAPI(t, "", keyX, APIProfile{Model: "main", OpusModel: "opus", SonnetModel: "sonnet", Context1M: true})
			if err := r.mod.Switch(ctx, acc.ID, true); err != nil {
				t.Fatal(err)
			}
			before := fileBytes(t, r.store.SettingsPath())
			p := *acc.API
			p.Context1M = false
			if _, err := r.mod.EditAPIAccount(acc.ID, APIEdit{Profile: p, Apply: apply}); err != nil {
				t.Fatal(err)
			}
			if !apply && !bytes.Equal(before, fileBytes(t, r.store.SettingsPath())) {
				t.Fatal("save-only rewrote settings")
			}
			want := "main[1m]"
			if apply {
				want = "main"
			}
			if r.settings(t)["model"] != want {
				t.Fatal("wrong applied model")
			}
			if !apply {
				return
			}
			// External edits survive cleanup while the other applied values disappear.
			data := r.settings(t)
			liveEnv(data)["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "external"
			body, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			r.writeSettings(t, string(body))
			if err := r.mod.Switch(ctx, oauth, true); err != nil {
				t.Fatal(err)
			}
			got := r.settings(t)
			if _, ok := got["model"]; ok {
				t.Fatal("applied default model survived cleanup")
			}
			env := liveEnv(got)
			if _, ok := env["ANTHROPIC_DEFAULT_OPUS_MODEL"]; ok {
				t.Fatal("applied Opus survived cleanup")
			}
			if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "external" {
				t.Fatal("external mapping was removed")
			}
		})
	}
}

func TestAPIContext1MPersistsAndImportedSuffixRemainsExplicit(t *testing.T) {
	r := newAPIRig(t)
	acc := r.addAPI(t, "", keyX, APIProfile{Model: "main", Context1M: true})
	stored, ok := r.index(t).Account(acc.ID)
	if !ok || !stored.API.Context1M || stored.API.Model != "main" {
		t.Fatal("context choice or original model did not persist")
	}
	r.writeSettings(t, `{"model":"manual[1M]","env":{"ANTHROPIC_BASE_URL":"`+r.relay.url()+`","ANTHROPIC_AUTH_TOKEN":"`+keyY+`","ANTHROPIC_DEFAULT_OPUS_MODEL":"opus"}}`)
	if _, err := r.mod.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	var imported ManagedAccount
	for _, a := range r.index(t).Accounts {
		if a.ID != acc.ID && a.IsAPI() {
			imported = a
		}
	}
	if imported.API == nil || imported.API.Context1M || imported.API.Model != "manual[1M]" || imported.API.Env("test").OpusModel != "opus" {
		t.Fatal("import inferred a context switch from per-model suffixes")
	}
}

func TestAPIContext1MEnabledCleanup(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	oauth := r.loginOAuth(t, "test-access-a", "acct-a")
	acc := r.addAPI(t, "", keyX, APIProfile{Model: "main", OpusModel: "opus", Context1M: true})
	if err := r.mod.Switch(ctx, acc.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := r.mod.Switch(ctx, oauth, true); err != nil {
		t.Fatal(err)
	}
	got := r.settings(t)
	if _, ok := got["model"]; ok {
		t.Fatal("enabled default model survived cleanup")
	}
	if _, ok := liveEnv(got)["ANTHROPIC_DEFAULT_OPUS_MODEL"]; ok {
		t.Fatal("enabled Opus model survived cleanup")
	}
}
