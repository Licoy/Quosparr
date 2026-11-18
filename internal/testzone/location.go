// Package testzone loads time zone rules for frozen Swift comparison fixtures.
package testzone

import (
	_ "embed"
	"time"
)

// The fixtures expect Havana to turn back its clocks in November 2026.
// IANA 2026f removed that transition. Keep the rules from IANA 2026e
// (northamerica, compiled with zic) alongside the frozen expectations.
//
//go:embed testdata/America_Havana.tzif
var havana []byte

// LoadLocation uses the frozen rules for Havana and the system rules otherwise.
func LoadLocation(name string) (*time.Location, error) {
	if name == "America/Havana" {
		return time.LoadLocationFromTZData(name, havana)
	}
	return time.LoadLocation(name)
}
