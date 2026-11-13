// Package gemini manages Gemini accounts signed in through the Google
// Antigravity clients (desktop app 2.x, IDE and the agy CLI) and reads their
// Cloud Code quota.
//
// # Official login
//
// The three clients share one credential: a JSON document
//
//	{"token":{"access_token","token_type","refresh_token","expiry"},"auth_method","id_token"?}
//
// stored in the system keychain under service "gemini", account
// "antigravity". On macOS the item value is "go-keyring-base64:" followed by
// the base64 of that JSON and is read and written through /usr/bin/security
// with the secret on stdin only; on Windows (target "gemini:antigravity") and
// Linux (Secret Service) the raw JSON goes through go-keyring so both sides
// read each other's items. Two files may mirror it:
// ~/.gemini/jetski-standalone-oauth-token and
// ~/.gemini/antigravity-cli/antigravity-oauth-token.
//
// Reading tries the keychain first, then the files in that order. Writing
// always writes the keychain and writes a mirror file only when it already
// exists; every target is read back. A failure at any step restores every
// target to its previous bytes (or absence) and verifies the restore; when
// that is not possible the error is a *PartialWriteError. Unknown JSON fields
// of the document are kept when tokens are rewritten. The pre-2.0 state
// database is not supported.
//
// # Library
//
// Managed accounts live in <Quosparr data>/accounts, shared with the other
// providers: gemini-index.json plus one gemini-<hash> directory per account
// holding credential.json (0600, atomic writes).
// The index never holds tokens. An account is the Google subject ("sub");
// when no subject is known, the case-folded email. Ids are "gemini-" plus the
// stable hash of that key. An official login whose refresh token equals a
// managed copy is that account without any network call; otherwise the
// identity comes from the id_token claims, else from the userinfo endpoint.
//
// The official login is written only by an explicit switch, by bootstrapping
// the first account of an empty library, and by refreshing the current
// account's token. Refreshing any other account changes its managed copy
// only. Pasted credentials bootstrap an empty library the same way and
// re-pasting the current account also updates the official login; packs
// (shared transfer format, provider "gemini") and previewed files only ever
// write the library. The current account cannot be removed. All module
// operations are serialized. A switch is refused while Antigravity or agy
// runs unless the caller allows it; restarting those clients is the
// caller's job.
//
// # Tokens and quota
//
// Access tokens are renewed RefreshSkew (15 minutes) before expiry with the
// refresh_token grant of the Antigravity OAuth client. A response without a
// new refresh token or id_token keeps the old ones. invalid_grant means the
// user has to sign in again.
//
// A quota reading is loadCodeAssist (onboardUser with polling when no
// project is assigned; the placeholder project "aicode-consumers" counts as
// none), then retrieveUserQuotaSummary and fetchAvailableModels. Requests
// go to the daily Cloud Code host unless the account uses the GCP terms of
// service with a real project, which uses the production host; the
// QUOSPARR_DEV_GEMINI_CLOUDCODE_URL environment variable overrides both. A
// 401 is retried once after a forced token refresh; a 403 carrying
// VALIDATION_REQUIRED, SUBSCRIPTION_REQUIRED or a validation_url is its own
// error, and the link is kept in the cached snapshot; a 429 backs off per
// account (Retry-After, else 5 minutes doubling to an hour). The tier is
// paidTier.id, else currentTier.id: "ultra" in it is Ultra, "pro" is Pro,
// anything else Free, nothing Unknown.
//
// # Local sessions
//
// SessionScanner reads the conversations of the desktop App
// (~/.gemini/antigravity) and of agy (~/.gemini/antigravity-cli): titles
// and times from conversation_summaries.db, token usage from each
// conversations/<id>.db, always from a private temporary copy so the live
// databases and their WAL are never opened. Encrypted .pb conversations are
// counted as unreadable. DeleteManaged is the only writer: it runs only
// while every touched client is stopped, backs up each summaries database
// it edits first, moves the conversation files to the system Trash, and
// removes the rows of the conversations whose files all moved.
// RunningClients tells the App (an "Antigravity" executable) from agy, so
// the caller can say which client owns a data root (SessionClientOf).
// ScanCache weighs each call's whole prompt against its cache reads.
//
// # MCP
//
// The App and agy share <GeminiHome>/config/mcp_config.json; the Store
// methods read and write it through package mcp, serialized among
// themselves, with backups in <Quosparr data>/backups.
package gemini
