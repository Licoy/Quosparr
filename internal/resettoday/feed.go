package resettoday

import "strings"

// Feed is one hosted reset monitor. Both monitors publish the same schema
// version 1 feed and reaction service; they differ in the product named by
// the derived rationales, the X accounts a source may cite and the site.
// The zero Feed is CodexFeed.
type Feed struct {
	// Product is the product name inside the derived rationales.
	Product string
	// Handle is the only X account a source may cite; "" accepts any valid
	// handle, as long as the post URL names the same account.
	Handle string
	// SiteURL is the home page, with a trailing slash.
	SiteURL string
	// HistoryExt is appended to the history page path ("" for none).
	HistoryExt string
}

// Hosted monitors.
var (
	CodexFeed = Feed{Product: "Codex", Handle: "thsottiaux", SiteURL: "https://didcodexreset.com/", HistoryExt: ".html"}
	// AntigravityFeed follows the Gemini quota of Google Antigravity; the
	// accounts it monitors are configured by the service.
	AntigravityFeed = Feed{Product: "Antigravity", SiteURL: "https://agyreset.com/"}
)

func (f Feed) orCodex() Feed {
	if f.SiteURL == "" {
		return CodexFeed
	}
	return f
}

// StatusURL is the status feed endpoint.
func (f Feed) StatusURL() string { return f.orCodex().SiteURL + "api/status.json" }

// ReactionURL is the reaction service endpoint.
func (f Feed) ReactionURL() string { return f.orCodex().SiteURL + "api/reaction" }

// Origin is the Origin header the reaction service expects on a POST.
func (f Feed) Origin() string { return strings.TrimSuffix(f.orCodex().SiteURL, "/") }

// Referer is the Referer header of a reaction POST.
func (f Feed) Referer() string { return f.orCodex().SiteURL }

// isXHandle is the X username alphabet: 1 to 15 letters, digits or "_".
func isXHandle(s string) bool {
	if len(s) == 0 || len(s) > 15 {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}
