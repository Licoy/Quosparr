package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
)

func TestAccountIndexLoadErrorsHaveSpecificSafeDisplay(t *testing.T) {
	home := t.TempDir()
	codexStore := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "quosparr"))
	grokStore := grok.NewStore(filepath.Join(home, "grok"), filepath.Join(home, "quosparr"))
	cases := []struct {
		path string
		load func() error
	}{
		{codexStore.IndexPath(), func() error { _, err := codexStore.LoadIndex(); return err }},
		{grokStore.IndexPath(), func() error { _, err := grokStore.LoadIndex(); return err }},
	}
	for _, tc := range cases {
		for _, document := range []string{`{"private-secret":`, `{"version":1,"accounts":"private-secret"}`} {
			if err := os.MkdirAll(filepath.Dir(tc.path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(tc.path, []byte(document), 0600); err != nil {
				t.Fatal(err)
			}
			err := tc.load()
			var codexInvalid *codex.InvalidIndexError
			var grokInvalid *grok.InvalidIndexError
			if !errors.As(err, &codexInvalid) && !errors.As(err, &grokInvalid) {
				t.Fatal("Store.LoadIndex lost its invalid-index category")
			}
			if errors.Unwrap(err) == nil {
				t.Fatal("invalid-index error discarded its parser cause")
			}
			for _, language := range l10n.Languages() {
				l := l10n.New(language)
				if accountReasonText(l, "invalid_index") == accountReasonText(l, "unknown") {
					t.Fatal("invalid-index classification has no dedicated translation")
				}
				message := accountFailure(l, tx(l, xAccountSaveFailed), err)
				assertSelectionReason(t, message, accountReasonText(l, "invalid_index"), home, "private-secret")
				if strings.Contains(err.Error(), "private-secret") || strings.Contains(err.Error(), home) {
					t.Fatal("invalid-index Error exposed parser input")
				}
			}
		}
	}
}
