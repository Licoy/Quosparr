package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestAPIApplyRemovesLegacyModelOverride(t *testing.T) {
	env := relayEnv()
	env.Model = "selected"
	out, err := ApplyAPISettings([]byte(`{"env":{"ANTHROPIC_MODEL":"legacy"}}`), env, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := envOf(t, out)["ANTHROPIC_MODEL"]; exists {
		t.Fatal("legacy model override survived explicit application")
	}
}

func TestAPIModelDriftIsOutdated(t *testing.T) {
	for _, field := range []string{"model", "ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_FABLE_MODEL", "ANTHROPIC_MODEL"} {
		t.Run(field, func(t *testing.T) {
			r := newAPIRig(t)
			a := r.addAPI(t, "", keyX, APIProfile{Model: "main", OpusModel: "opus", SonnetModel: "sonnet", HaikuModel: "haiku", FableModel: "fable", Context1M: true})
			if err := r.mod.Switch(context.Background(), a.ID, true); err != nil {
				t.Fatal(err)
			}
			data := r.settings(t)
			if field == "model" {
				data[field] = "external"
			} else {
				liveEnv(data)[field] = "external"
			}
			body, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			r.writeSettings(t, string(body))
			if used, stale := r.mod.APIAccountState(a.ID); !used || !stale {
				t.Fatal("model drift was not offered for application")
			}
		})
	}
}

func TestAPISaveOnlyModelsCleanedAfterReload(t *testing.T) {
	for _, targetAPI := range []bool{false, true} {
		t.Run(map[bool]string{false: "oauth", true: "api"}[targetAPI], func(t *testing.T) {
			r := newAPIRig(t)
			ctx := context.Background()
			oauth := r.loginOAuth(t, "test-access-a", "acct-a")
			a := r.addAPI(t, "", keyX, APIProfile{Model: "main", OpusModel: "opus", SonnetModel: "sonnet", HaikuModel: "haiku", FableModel: "fable", Context1M: true})
			if err := r.mod.Switch(ctx, a.ID, true); err != nil {
				t.Fatal(err)
			}
			before := fileBytes(t, r.store.SettingsPath())
			for _, p := range []APIProfile{{Model: "changed"}, {}} {
				if _, err := r.mod.EditAPIAccount(a.ID, APIEdit{Profile: p}); err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(before, fileBytes(t, r.store.SettingsPath())) {
				t.Fatal("save-only modified settings")
			}
			if used, stale := r.mod.APIAccountState(a.ID); !used || !stale {
				t.Fatal("save-only lost pending model application")
			}
			// Load from disk as the switch path does, with no in-memory ownership.
			if _, err := r.store.LoadIndex(); err != nil {
				t.Fatal(err)
			}
			target := oauth
			if targetAPI {
				target = r.addAPI(t, "", keyY, APIProfile{}).ID
			}
			if err := r.mod.Switch(ctx, target, true); err != nil {
				t.Fatal(err)
			}
			got := r.settings(t)
			if _, exists := got["model"]; exists {
				t.Fatal("previous applied default model survived switch")
			}
			for _, f := range mappingFields {
				if _, exists := liveEnv(got)[f.key]; exists {
					t.Fatalf("previous applied mapping survived: %s", f.key)
				}
			}
		})
	}
}

func TestAPIEmptyModelsPreserveUnrelatedManualValues(t *testing.T) {
	r := newAPIRig(t)
	a := r.addAPI(t, "", keyX, APIProfile{})
	if err := r.mod.Switch(context.Background(), a.ID, true); err != nil {
		t.Fatal(err)
	}
	data := r.settings(t)
	data["model"] = "manual"
	liveEnv(data)["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "manual-opus"
	liveEnv(data)["ANTHROPIC_MODEL"] = "manual-override"
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	r.writeSettings(t, string(body))
	if used, stale := r.mod.APIAccountState(a.ID); !used || stale {
		t.Fatal("unmanaged manual values reported outdated")
	}
	if _, err := r.mod.EditAPIAccount(a.ID, APIEdit{Profile: APIProfile{}, Apply: true}); err != nil {
		t.Fatal(err)
	}
	got := r.settings(t)
	if got["model"] != "manual" || liveEnv(got)["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "manual-opus" || liveEnv(got)["ANTHROPIC_MODEL"] != "manual-override" {
		t.Fatal("application removed unmanaged manual values")
	}
}

func TestAPIPendingApplicationUpdatesOwnership(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	oauth := r.loginOAuth(t, "test-access-a", "acct-a")
	a := r.addAPI(t, "", keyX, APIProfile{Model: "main", OpusModel: "opus", Context1M: true})
	if err := r.mod.Switch(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	p := *a.API
	p.Context1M = false
	if _, err := r.mod.EditAPIAccount(a.ID, APIEdit{Profile: p}); err != nil {
		t.Fatal(err)
	}
	if used, stale := r.mod.APIAccountState(a.ID); !used || !stale {
		t.Fatal("1M pending change not reported outdated")
	}
	if _, err := r.mod.EditAPIAccount(a.ID, APIEdit{Profile: p, Apply: true}); err != nil {
		t.Fatal(err)
	}
	if used, stale := r.mod.APIAccountState(a.ID); !used || stale {
		t.Fatal("unchanged form could not apply pending profile")
	}
	saved, _ := r.index(t).Account(a.ID)
	if saved.AppliedAPIModels == nil || saved.AppliedAPIModels.Model != "main" {
		t.Fatal("new applied models not persisted")
	}
	data := r.settings(t)
	data["model"] = "external"
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	r.writeSettings(t, string(body))
	if err := r.mod.Switch(ctx, oauth, true); err != nil {
		t.Fatal(err)
	}
	if r.settings(t)["model"] != "external" {
		t.Fatal("external default model removed")
	}
}

func TestAPILegacySaveOnlyRetainsOldProfileOwnership(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	oauth := r.loginOAuth(t, "test-access-a", "acct-a")
	a := r.addAPI(t, "", keyX, APIProfile{Model: "old", OpusModel: "old-opus"})
	if err := r.mod.Switch(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	a, _ = r.index(t).Account(a.ID)
	a.AppliedAPIModels = nil
	if _, err := r.store.PutAPI(a, "", r.now); err != nil {
		t.Fatal(err)
	}
	data := r.settings(t)
	liveEnv(data)["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "manual"
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	r.writeSettings(t, string(body))
	if _, err := r.mod.EditAPIAccount(a.ID, APIEdit{Profile: APIProfile{}}); err != nil {
		t.Fatal(err)
	}
	if err := r.mod.Switch(ctx, oauth, true); err != nil {
		t.Fatal(err)
	}
	got := r.settings(t)
	if _, exists := got["model"]; exists {
		t.Fatal("legacy applied model survived")
	}
	if liveEnv(got)["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "manual" {
		t.Fatal("legacy snapshot absorbed external model")
	}
}

func TestAPISaveOnlyKeyImportRetainsAppliedModelOwnership(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	oauth := r.loginOAuth(t, "test-access-a", "acct-a")
	a := r.addAPI(t, "", keyX, APIProfile{Model: "old", OpusModel: "old-opus"})
	if err := r.mod.Switch(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.mod.EditAPIAccount(a.ID, APIEdit{Profile: APIProfile{}, Key: keyY}); err != nil {
		t.Fatal(err)
	}
	if used, _ := r.mod.APIAccountState(a.ID); used {
		t.Fatal("save-only rotated key still identified as old login")
	}
	data := r.settings(t)
	liveEnv(data)["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "manual"
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	r.writeSettings(t, string(body))
	if _, err := r.mod.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.mod.Switch(ctx, oauth, true); err != nil {
		t.Fatal(err)
	}
	got := r.settings(t)
	if _, exists := got["model"]; exists {
		t.Fatal("key import lost old applied ownership")
	}
	if liveEnv(got)["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "manual" {
		t.Fatal("key import adopted external mapping")
	}
}

func TestAPICleanupDoesNotOwnManualLegacyOverride(t *testing.T) {
	env := relayEnv()
	env.Model = "same"
	in := []byte(`{"model":"same","env":{"ANTHROPIC_MODEL":"same"}}`)
	out, _, err := RemoveAPISettings(in, env)
	if err != nil {
		t.Fatal(err)
	}
	if envOf(t, out)["ANTHROPIC_MODEL"] != "same" {
		t.Fatal("cleanup removed manual override matching applied default")
	}
	next := relayEnv()
	out, err = ApplyAPISettings(in, next, &env)
	if err != nil {
		t.Fatal(err)
	}
	if envOf(t, out)["ANTHROPIC_MODEL"] != "same" {
		t.Fatal("empty default adopted manual override")
	}
	stale, err := apiSettingsOutdated(out, next, env)
	if err != nil || stale {
		t.Fatal("manual override alone made empty default outdated")
	}
}
