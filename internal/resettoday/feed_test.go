package resettoday

import (
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
)

func TestFeedEndpoints(t *testing.T) {
	var zero Feed
	if zero.StatusURL() != StatusURL || zero.ReactionURL() != ReactionURL ||
		zero.Origin() != ReactionOrigin || zero.Referer() != ReactionReferer {
		t.Fatalf("zero feed = %q %q %q %q", zero.StatusURL(), zero.ReactionURL(), zero.Origin(), zero.Referer())
	}
	f := AntigravityFeed
	if f.StatusURL() != "https://agyreset.com/api/status.json" || f.ReactionURL() != "https://agyreset.com/api/reaction" ||
		f.Origin() != "https://agyreset.com" || f.Referer() != "https://agyreset.com/" {
		t.Fatalf("antigravity feed = %q %q %q %q", f.StatusURL(), f.ReactionURL(), f.Origin(), f.Referer())
	}
}

// agyFeed is an Antigravity-shaped feed: the source cites another account
// and the scope carries fields the Codex feed does not publish.
func agyFeed(handle, urlHandle, rationale string) string {
	return `{
  "schemaVersion": 1,
  "generatedAt": "2026-11-26T12:00:00.000Z",
  "lastSuccessfulCheckAt": "2026-11-26T12:00:00.000Z",
  "monitor": {"status": "ok", "errorCode": null},
  "events": [{
    "kind": "reset_completed",
    "resetType": "global",
    "announcedAt": "2026-11-26T07:19:24.000Z",
    "effectiveAt": null,
    "scope": {"plans": ["all"], "windows": ["unknown"], "modelScope": "gemini", "modelIds": []},
    "source": {"handle": "` + handle + `", "postId": "2125851902009311306", "url": "https://x.com/` + urlHandle + `/status/2125851902009311306"},
    "confidence": 0.99,
    "rationale": "` + rationale + `",
    "text": "Resetting Gemini quota for all Antigravity users."
  }]
}`
}

func TestAntigravityFeedDecodes(t *testing.T) {
	now := at(t, "2026-11-26T12:00:00Z")
	data := []byte(agyFeed("_mohansolo", "_MohanSolo", "Explicit Antigravity quota reset announcement."))
	s, err := AntigravityFeed.Decode(data, now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if s.State() != StateYes || len(s.Events) != 1 || s.Feed.SiteURL != AntigravityFeed.SiteURL {
		t.Fatalf("snapshot = %+v", s)
	}
	if got := s.EvidenceURL(now, time.UTC, l10n.English); got != "https://agyreset.com/history/1795677564000" {
		t.Errorf("evidence url = %q", got)
	}
	if got := s.EvidenceURL(now, time.UTC, l10n.SimplifiedChinese); got != "https://agyreset.com/zh/history/1795677564000" {
		t.Errorf("zh evidence url = %q", got)
	}
	if _, err := Decode(data, now, time.UTC); err == nil {
		t.Error("the Codex feed must reject an Antigravity event")
	}
}

func TestAntigravityFeedRejectsForeignSources(t *testing.T) {
	now := at(t, "2026-11-26T12:00:00Z")
	for name, data := range map[string]string{
		"codex rationale": agyFeed("_mohansolo", "_mohansolo", "Explicit Codex quota reset announcement."),
		"url of another":  agyFeed("_mohansolo", "someone", "Explicit Antigravity quota reset announcement."),
		"bad handle":      agyFeed("not a handle", "not a handle", "Explicit Antigravity quota reset announcement."),
	} {
		if _, err := AntigravityFeed.Decode([]byte(data), now, time.UTC); err == nil {
			t.Errorf("%s: decoded", name)
		} else if strings.Contains(err.Error(), "Resetting") {
			t.Errorf("%s: error leaks feed text: %v", name, err)
		}
	}
}

func TestCodexEvidenceURLKeepsHTMLPage(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	s := newFeed(now, newEvent("reset_completed", "2026-07-28T11:55:00.000Z", post("123"))).mustDecode(t)
	if got := s.EvidenceURL(now, time.UTC, l10n.English); !strings.HasPrefix(got, SiteURL+"history/") || !strings.HasSuffix(got, ".html") {
		t.Errorf("evidence url = %q", got)
	}
}
