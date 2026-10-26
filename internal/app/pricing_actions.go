package app

import (
	"context"
	"time"

	"github.com/licoy/quosparr/internal/usage/decimal"
)

// PricingActions exposes only in-memory catalog reads and a cancellable worker refresh.
// Snapshot must return immutable rows, reused while DataVersion has not changed.
type PricingActions struct {
	Snapshot func() PricingSnapshot
	Refresh  func(context.Context) error
}
type PricingSnapshot struct {
	Rows                                            []PricingRow
	LastAttempt, LastSuccess, LastChanged, NextSync time.Time
	Version, DataVersion, Error                     string
	Syncing                                         bool
}

// Prices are USD per million tokens; nil means unavailable, while zero is free.
// Details is localized by the business adapter, including applied-vs-catalog rules.
type PricingRow struct {
	ProviderID, ProviderName, ID, Name   string
	Input, Output, CacheRead, CacheWrite *decimal.Decimal
	Source, SourceURL, Version, Details  string
	Available, Builtin                   bool
}
