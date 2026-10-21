package runway

import (
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// Section is a part of the Codex panel that refreshes on its own.
type Section string

// Section values (Swift RunwayRefreshSection).
const (
	SectionQuota          Section = "quota"
	SectionResetCredits   Section = "resetCredits"
	SectionResetToday     Section = "rateLimitResetToday"
	SectionQuotaEstimate  Section = "quotaEstimate"
	SectionAPICost        Section = "apiCost"
	SectionTokenHeatmap   Section = "tokenHeatmap"
	SectionSpeed          Section = "speedAnalysis"
	SectionSessionRepair  Section = "sessionRepair"
	SectionRecentSessions Section = "recentSessions"
)

// Snapshot is an immutable view of the whole coordinator state. The Service
// builds a new one after every change; the slices, maps and pointers inside
// are never modified afterwards, so a UI may keep and read it from any
// goroutine. Pointers are nil when the datum is not available.
type Snapshot struct {
	// Seq increases with every published snapshot.
	Seq         uint64
	GeneratedAt time.Time
	Language    l10n.Language
	// Provider is the platform the panel shows.
	Provider store.Provider

	Accounts   AccountsView
	Codex      CodexView
	Grok       GrokState
	Claude     ClaudeState
	Cursor     CursorState
	ResetToday ResetTodayState
	Refresh    RefreshView
	StatusBar  StatusBarView
	Floating   FloatingView
}

// AccountsView is the managed account list and the signed-in identity.
type AccountsView struct {
	// Sidebar is the account list in sidebar order (active first).
	Sidebar  []codex.ManagedAccount
	ActiveID string
	// Display is the identity of the current credential; Authenticated is
	// false after an authentication failure.
	Display codex.AccountDisplay
	// Switching is true while a switch runs; Message is the operation status.
	Switching bool
	Message   string
	// RefreshingIDs are the accounts whose quota is being polled.
	RefreshingIDs []string
}

// CodexView is the Codex panel state.
type CodexView struct {
	// AccountID is the managed account the data belongs to ("" when unknown)
	// and Generation the account-state generation it was fetched in. Both
	// change when the account switches or its state is cleared.
	AccountID  string
	Generation uint64

	Authenticated bool
	// APIKeyMode is true while the Codex login is an API key: there is no
	// ChatGPT quota, reset credits or estimate then, only the local usage.
	// APIBaseURL is the relay site of that key ("" for a plain OpenAI key) and
	// APIBalance the cached balance reading of the site (nil without one).
	// Meters then holds the balance meter, if any.
	APIKeyMode bool
	APIBaseURL string
	APIBalance *apiquota.Quota
	// StatusText is the status item text; the *Text fields are one-line
	// summaries and the *Lines fields the detail rows of each section.
	StatusText         string
	QuotaText          string
	ResetCreditsText   string
	CostText           string
	CostSubtitle       string
	SessionText        string
	QuotaLines         []DetailLine
	ResetCreditLines   []DetailLine
	CostLines          []DetailLine
	SessionLines       []DetailLine
	RecentSessionLines []DetailLine

	Meters             []Meter
	Quota              *codexapi.QuotaSnapshot
	ResetCredits       *codexapi.ResetCreditsSnapshot
	ResetCreditSummary *ResetCreditSummary
	CreditDetails      []CreditDetail

	Estimate      *estimate.Snapshot
	EstimateError string

	// Cost is the current-cycle summary; DisplayedCost the one for
	// DisplayedRange (what the cost card shows).
	Cost           *usage.Summary
	DisplayedCost  *usage.Summary
	DisplayedRange store.CostRange
	CostScanNote   string

	Heatmap HeatmapData
	// Speed is the model speed of the recent local sessions; SpeedError the
	// user-facing error of its last scan.
	Speed          *speed.Report
	SpeedError     string
	RecentSessions []sessions.Item
	SessionReport  *sessions.Report

	// LastError is the user-facing error of the last refresh, "" when fine.
	LastError string
}

// HeatmapData is the raw data of the token usage chart; build a grid with
// Grid. Maps are shared between snapshots and must not be modified.
type HeatmapData struct {
	// AllDevices is the account's official profile series, Local this Mac's
	// session logs; both are keyed by UTC "yyyy-MM-dd".
	AllDevices          map[string]int64
	Local               map[string]int64
	CalculatedAt        time.Time
	OfficialStatsAsOf   string
	OfficialGeneratedAt time.Time
}

// Grid builds the heatmap/chart grid for a mode. It uses UTC days like the
// profile statistics.
func (h HeatmapData) Grid(mode usage.HeatmapMode, firstWeekday int, now time.Time) usage.HeatmapSnapshot {
	at := h.CalculatedAt
	if at.IsZero() {
		at = now
	}
	return usage.BuildHeatmap(usage.HeatmapInput{
		AllDevices: h.AllDevices, Local: h.Local, Mode: mode, Now: at,
		FirstWeekday: firstWeekday, Loc: time.UTC,
	})
}

// RefreshView says what is refreshing right now.
type RefreshView struct {
	// All is true during a full refresh; Sections lists single-section runs
	// (and the sections of a full refresh as they run).
	All           bool
	Sections      []Section
	AccountQuotas bool
	CostProgress  usage.Progress
}

// Refreshing reports whether anything is refreshing.
func (r RefreshView) Refreshing() bool { return r.All || len(r.Sections) > 0 }

// Has reports whether a section is refreshing.
func (r RefreshView) Has(s Section) bool {
	for _, x := range r.Sections {
		if x == s {
			return true
		}
	}
	return false
}

// EventKind classifies a service event.
type EventKind int

// EventKind values.
const (
	// EventAlerts: new, deduplicated alerts to show as OS notifications.
	EventAlerts EventKind = iota
	// EventFullRefreshDone: a full refresh finished (or was cancelled).
	EventFullRefreshDone
)

// Event is something the UI layer should act on besides re-rendering.
type Event struct {
	Kind   EventKind
	Alerts []alerts.Alert
}
