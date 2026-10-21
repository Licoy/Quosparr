package claude

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAPIExternalLoginDoesNotInheritModelOwnership(t *testing.T) {
	for _, differentSite := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-site-different-key", true: "different-site"}[differentSite], func(t *testing.T) {
			r := newAPIRig(t)
			ctx := context.Background()
			a := r.addAPI(t, "", keyX, APIProfile{Model: "same", OpusModel: "owned-opus"})
			if err := r.mod.Switch(ctx, a.ID, true); err != nil {
				t.Fatal(err)
			}
			base, key := r.relay.url(), keyY
			if differentSite {
				base, key = "https://different.example", keyX
			}
			data := map[string]any{"model": "same", "env": map[string]any{envBaseURL: base, envAuthToken: key, "ANTHROPIC_DEFAULT_OPUS_MODEL": "external-opus"}}
			body, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			r.writeSettings(t, string(body))
			idx, err := r.mod.importAPISettings(r.index(t))
			if err != nil {
				t.Fatal(err)
			}
			var imported ManagedAccount
			for _, row := range idx.Accounts {
				if row.IsAPI() && row.ID != a.ID {
					imported = row
				}
			}
			if imported.API == nil {
				t.Fatal("external login not imported")
			}
			if imported.AppliedAPIModels != nil || imported.AppliedAPIIdentity != "" {
				t.Fatal("unrelated external identity inherited previous model ownership")
			}
		})
	}
}

func TestAPIBaseSaveOnlyImportRetainsAppliedOwnership(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	a := r.addAPI(t, "", keyX, APIProfile{Model: "old", OpusModel: "owned-opus"})
	if err := r.mod.Switch(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.mod.EditAPIAccount(a.ID, APIEdit{Profile: APIProfile{BaseURL: "https://new.example"}}); err != nil {
		t.Fatal(err)
	}
	idx, err := r.mod.importAPISettings(r.index(t))
	if err != nil {
		t.Fatal(err)
	}
	var imported ManagedAccount
	for _, row := range idx.Accounts {
		if row.IsAPI() && row.ID != a.ID {
			imported = row
		}
	}
	if imported.AppliedAPIModels == nil || imported.AppliedAPIModels.OpusModel != "owned-opus" || imported.AppliedAPIIdentity != mustAppliedIdentity(t, r.relay.url(), keyX) {
		t.Fatal("old live identity lost saved model ownership")
	}
}

func TestAPIAppliedIdentityIsFullAndCanonical(t *testing.T) {
	a := mustAppliedIdentity(t, "https://relay.example/v1/", keyX)
	if len(a) != 64 || a != mustAppliedIdentity(t, "https://relay.example", keyX) {
		t.Fatal("identity digest is not full and canonical")
	}
	if a == mustAppliedIdentity(t, "https://relay.example", keyY) || a == mustAppliedIdentity(t, "https://different.example", keyX) {
		t.Fatal("distinct logins share applied identity")
	}
}

func mustAppliedIdentity(t *testing.T, base, key string) string {
	t.Helper()
	identity, err := appliedAPIIdentity(base, key)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestAPIInvalidAppliedIdentityFailsBeforeSettingsChange(t *testing.T) {
	r := newAPIRig(t)
	a := r.addAPI(t, "", keyX, APIProfile{Model: "main"})
	r.writeSettings(t, `{"model":"manual"}`)
	original := fileBytes(t, r.store.SettingsPath())
	env := a.API.Env(keyX)
	env.BaseURL = "not-a-url"
	if _, err := r.store.applyAPIAccount(a, "", env, nil, r.now, true); err == nil {
		t.Fatal("invalid relay identity silently accepted")
	}
	if string(original) != string(fileBytes(t, r.store.SettingsPath())) {
		t.Fatal("invalid identity changed settings before rejection")
	}
}
