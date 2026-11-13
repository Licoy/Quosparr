package gemini

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCredentialKeepsUnknownFieldsAndOldTokens(t *testing.T) {
	in := []byte(`{"token":{"access_token":"a1","token_type":"Bearer","refresh_token":"r1","expiry":"2026-11-13T10:30:00Z","extra_tok":1},"auth_method":"consumer","id_token":"i1","future":{"x":true}}`)
	c := mustCred(t, in)
	if c.AccessToken != "a1" || c.RefreshToken != "r1" || c.IDToken != "i1" || !c.Expiry.Equal(testNow.Add(30*time.Minute)) {
		t.Fatalf("parsed %+v", c)
	}
	out, err := c.WithRefreshedTokens("a2", "", "", 3600, testNow).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	token := root["token"].(map[string]any)
	if root["future"] == nil || token["extra_tok"] == nil || root["auth_method"] != "consumer" {
		t.Fatalf("unknown fields lost: %s", out)
	}
	if token["access_token"] != "a2" || token["refresh_token"] != "r1" || root["id_token"] != "i1" {
		t.Fatalf("tokens not kept: %v", token)
	}
	if got := mustCred(t, out).Expiry; !got.Equal(testNow.Add(time.Hour)) {
		t.Fatalf("expiry = %v", got)
	}
}

func TestCredentialRejectsDocumentsWithoutTokens(t *testing.T) {
	for _, bad := range []string{``, `[]`, `{}`, `{"token":{}}`, `{"token":{"access_token":"a","expiry":"nope"}}`} {
		if _, err := ParseCredential([]byte(bad)); err != ErrInvalidCredential {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
}

func TestNeedsRefreshUsesFifteenMinuteSkew(t *testing.T) {
	c := NewCredential("a", "r", "", testNow.Add(16*time.Minute))
	if c.NeedsRefresh(testNow, RefreshSkew) {
		t.Fatal("16 minutes left should not refresh")
	}
	if !NewCredential("a", "r", "", testNow.Add(14*time.Minute)).NeedsRefresh(testNow, RefreshSkew) {
		t.Fatal("14 minutes left should refresh")
	}
	if !NewCredential("a", "r", "", time.Time{}).NeedsRefresh(testNow, RefreshSkew) {
		t.Fatal("no expiry should refresh")
	}
	if !NewCredential("a", "", "", testNow.Add(-time.Minute)).RequiresReauthentication(testNow) {
		t.Fatal("expired without refresh token needs sign-in")
	}
}

func TestIdentityKeysAndMatching(t *testing.T) {
	c := NewCredential("a", "r", idToken("123", "User@Example.com"), testNow)
	id := c.IDTokenIdentity()
	if id.Subject != "123" || id.Email != "User@Example.com" {
		t.Fatalf("claims = %+v", id)
	}
	if id.StableID() != (Identity{Subject: "123"}).StableID() {
		t.Fatal("stable id must follow the subject")
	}
	if !identitiesMatch(Identity{Email: "user@example.com"}, id) {
		t.Fatal("email fallback must be case-insensitive")
	}
	if identitiesMatch(Identity{Subject: "999", Email: "user@example.com"}, id) {
		t.Fatal("different subjects are different accounts")
	}
}
