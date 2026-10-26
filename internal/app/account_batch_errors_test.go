package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestAccountBatchFailureKeepsCountsAndSafeReasons(t *testing.T) {
	l := l10n.New(l10n.English)
	private := &os.PathError{Op: "open", Path: "/private/credential-secret", Err: os.ErrPermission}
	message := accountBatchFailure(l, 2, 3, []error{private, private, errors.New("secret-response-body")})
	if !strings.Contains(message, tx(l, xAccountBatchResult, 2, 3)) || !strings.Contains(message, accountErrorReason(l, private)) {
		t.Fatal("batch must preserve counts and safe reason")
	}
	for _, secret := range []string{"credential-secret", "secret-response-body", "/private/"} {
		if strings.Contains(message, secret) {
			t.Fatal("batch exposed private error data")
		}
	}
	if strings.Count(message, accountErrorReason(l, private)) != 1 {
		t.Fatal("duplicate safe reasons must appear once")
	}
	if accountBatchFailure(l, 2, 0, nil) != "" {
		t.Fatal("successful refresh should retain empty notice")
	}
}

func TestGrokRefreshMessageReportsPartialCompletion(t *testing.T) {
	l := l10n.New(l10n.English)
	report := grok.RefreshReport{Outcomes: []grok.RefreshOutcome{{}, {Err: os.ErrPermission}}}
	message := grokRefreshMessage(l, report)
	if !strings.Contains(message, tx(l, xAccountBatchResult, 1, 1)) || !strings.Contains(message, accountErrorReason(l, os.ErrPermission)) {
		t.Fatal("refresh lost partial completion or safe reason")
	}
}

func TestRefreshClaudeAllReportsEveryMissingCredential(t *testing.T) {
	home := t.TempDir()
	st := claude.NewStoreAt(filepath.Join(home, "accounts"), filepath.Join(home, "claude"), home)
	idx := claude.NewAccountIndex()
	idx.Accounts = []claude.ManagedAccount{{ID: "missing-a"}, {ID: "missing-b"}}
	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(st.IndexPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(st.IndexPath(), data, 0600); err != nil {
		t.Fatal(err)
	}
	prefs := store.DefaultPreferences()
	prefs.Language = store.LanguageEnglish
	mod := claude.NewModule(st)
	rt := &liveApp{claudeMod: mod, claudeB: newClaudeBridge(mod, prefs, nil), svc: runway.New(runway.Options{Preferences: prefs})}
	message := rt.refreshClaudeAll(context.Background())
	l := l10n.New(l10n.English)
	if !strings.Contains(message, tx(l, xAccountBatchResult, 0, 2)) || !strings.Contains(message, accountErrorReason(l, os.ErrNotExist)) {
		t.Fatal("all-account refresh swallowed missing credential failures")
	}
	if strings.Contains(message, home) || strings.Contains(message, "missing-a") {
		t.Fatal("refresh notice exposed private account data")
	}
}
