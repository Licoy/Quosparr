package grok

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/licoy/quosparr/internal/codex"
)

func TestTransferKeepsFailureCauses(t *testing.T) {
	store := newTestStore(t)
	importer := TransferImporter{Store: store}
	preview := importer.PreviewFiles([]string{filepath.Join(t.TempDir(), "missing.json")})
	if len(preview.FailureErrors) != 1 || !errors.Is(preview.FailureErrors[0], os.ErrNotExist) {
		t.Fatal("preview lost missing-file cause")
	}
	batch, err := importer.ImportPreviewSelection([]ImportCandidate{{Preview: codex.PreviewItem{ID: "bad"}, CredentialData: []byte(`{}`)}}, []string{"bad"}, false)
	if err != nil || len(batch.FailureErrors) != 1 || len(batch.Failures) != 1 {
		t.Fatal("selection lost invalid-credential cause")
	}
	partial, err := importer.ImportPreviewSelection([]ImportCandidate{
		{Preview: codex.PreviewItem{ID: "valid"}, CredentialData: credSpec{email: "partial@example.invalid", user: "partial", team: "T"}.json()},
		{Preview: codex.PreviewItem{ID: "invalid"}, CredentialData: []byte(`{}`)},
	}, []string{"valid", "invalid"}, false)
	if err != nil || partial.SuccessCount() != 1 || partial.FailureCount() != 1 || len(partial.FailureErrors) != 1 {
		t.Fatal("partial selection lost success count or invalid-credential cause")
	}
	a := mustUpsert(t, store, credSpec{email: "fixture@example.com", user: "fixture", team: "T"}.json(), false)
	result, err := (Exporter{Store: store}).Export([]string{a.ID, "missing"}, testClock)
	var missing *AccountNotFoundError
	if err != nil || result.ExportedCount != 1 || len(result.FailureErrors) != 1 || !errors.As(result.FailureErrors[0], &missing) {
		t.Fatal("partial export lost missing-account cause")
	}
}

func TestLoginErrorKeepsCause(t *testing.T) {
	cause := &os.PathError{Op: "open", Path: "/private/test", Err: os.ErrPermission}
	err := loginError(context.Background(), cause)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatal("login lost filesystem cause")
	}
	if err.Error() != "grok: Grok OAuth login failed." {
		t.Fatal("login public message must remain fixed")
	}
}

func TestRefreshFailureRetainsCauseWithoutLeakingServerBody(t *testing.T) {
	refresher, _ := newRefresher(200, `{"error":"secret-server-code","error_description":"secret-description https://user:password@example.invalid/?token=private"}`)
	_, err := refresher.Refresh(context.Background(), cred("fixture@example.invalid", "fixture"), fixtureNow)
	var request *TokenRequestError
	if !errors.As(err, &request) || request.StatusCode != 200 {
		t.Fatal("token server error lost its safe status")
	}
	if strings.Contains(err.Error(), "secret-") || strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "private") {
		t.Fatal("token error exposed server response")
	}
	wrapped := (&Module{}).accountError(&TokenRequestError{Message: "token transport failed", Cause: os.ErrPermission}, "private-id")
	if !errors.Is(wrapped, os.ErrPermission) {
		t.Fatal("refresh outcome lost its underlying cause")
	}
}

func TestFailedExportWriteRetainsPerAccountCause(t *testing.T) {
	result, err := (Exporter{Store: newTestStore(t)}).Write([]string{"missing"}, filepath.Join(t.TempDir(), "out.json"), testClock)
	var missing *AccountNotFoundError
	if !errors.Is(err, codex.ErrNoAccountsExported) || len(result.FailureErrors) != 1 || !errors.As(result.FailureErrors[0], &missing) {
		t.Fatal("failed export write discarded missing-account cause")
	}
}

type privateFailureTransport struct{}

func (privateFailureTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, &url.Error{Op: "Post", URL: "https://user:private-transport@example.invalid?token=private-token", Err: os.ErrPermission}
}

func TestRefreshTransportMessageIsFixedAndCauseRetained(t *testing.T) {
	r := TokenRefresher{HTTP: &http.Client{Transport: privateFailureTransport{}}}
	_, err := r.Refresh(context.Background(), cred("fixture@example.invalid", "fixture"), fixtureNow)
	var request *TokenRequestError
	if !errors.As(err, &request) || !errors.Is(err, os.ErrPermission) {
		t.Fatal("transport failure lost its category")
	}
	if strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), "https://") {
		t.Fatal("transport error exposed URL credentials")
	}
}
