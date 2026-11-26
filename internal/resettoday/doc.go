// Package resettoday is the Go port of Quosparr's "did Codex reset today"
// feature: the hosted status feed (RateLimitResetToday*.swift), its strict
// decoder, the verdict and next-scheduled-reset rules, the optional reaction
// service, and the controllers that poll and refresh them.
//
// Monitors. The same feed and reaction service are hosted for Codex
// (CodexFeed, didcodexreset.com) and for the Gemini quota of Google
// Antigravity (AntigravityFeed, agyreset.com). A Feed fixes the product
// named by the derived rationales and the X accounts a source may cite:
// Codex sources must be @thsottiaux posts, Antigravity sources any account
// the post URL names. Fields a monitor adds beyond the Codex schema are
// ignored. The package-level Decode, Client and ReactionClient default to
// CodexFeed.
//
// Time rules. Feed timestamps are absolute RFC 3339 instants. "Today" is the
// viewer's local Gregorian day, passed to every method as a *time.Location
// (nil means time.Local); it is never the UTC date prefix of a feed string.
// Date-precision schedules are published in the source's calendar day
// (America/Los_Angeles), which is why this package embeds the time zone
// database. Midnights that do not exist (DST gaps) resolve to the first valid
// instant of the day, as Foundation does.
//
// Text. Verdict copy goes through internal/l10n. The few strings that need
// internal/runway's duration and schedule formatting take a Formatter, so this
// package does not depend on the coordinator that consumes it.
//
// Privacy. The reaction visitor ID is stored by export.ReactionStore and is
// never logged; nothing here reads credentials.
package resettoday
