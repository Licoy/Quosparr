package runway

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/store"
)

type fakeCursor struct{ count atomic.Int32 }

func (f *fakeCursor) State(time.Time) CursorState         { return CursorState{Availability: CursorReady} }
func (f *fakeCursor) SetOnChange(func())                  {}
func (f *fakeCursor) Refresh(context.Context, bool) error { f.count.Add(1); return nil }

func TestCursorRefreshRequiresOptInAndVisibleSurface(t *testing.T) {
	c := &fakeCursor{}
	s, _ := newService(t, newFake(t), func(o *Options) { o.Cursor = c })
	refreshAll(t, s)
	<-s.RefreshCursor(true)
	if c.count.Load() != 0 {
		t.Fatal("disabled Cursor refreshed")
	}
	p := s.Preferences()
	p.CursorMonitoringEnabled = true
	s.SetPreferences(p)
	settle(t, s)
	if c.count.Load() != 1 {
		t.Fatal("enabling visible provider did not refresh")
	}
	p.PanelProviders = []store.Provider{store.ProviderCodex}
	s.SetPreferences(p)
	settle(t, s)
	s.SetWidgetRequirements(export.RequireProviderQuota)
	refreshAll(t, s)
	if c.count.Load() != 1 {
		t.Fatal("unrelated widget refreshed hidden Cursor")
	}
	<-s.RefreshCursor(false)
	if c.count.Load() != 1 {
		t.Fatal("background refresh fetched hidden Cursor")
	}
	<-s.RefreshCursor(true)
	if c.count.Load() != 2 {
		t.Fatal("explicit refresh from account settings was ignored")
	}
}

func TestCursorPanelDoesNotReadCodexLocalData(t *testing.T) {
	s, f, g, c, reports := localService(t)
	p := s.Preferences()
	p.SelectedProvider = store.ProviderCursor
	s.SetPreferences(p)
	s.SetPanelOpen(true)
	settle(t, s)
	if got := localCounts(f, reports); got != [4]int{} || g.localCount() != 0 || c.localCount() != 0 {
		t.Fatal("Cursor opening scanned another provider")
	}
}

func TestCursorStatusAndFloatingKeepPoolNamesAndHideStaleMeters(t *testing.T) {
	p := store.DefaultPreferences()
	p.SelectedProvider = store.ProviderCursor
	in := StatusInputs{Prefs: p, Provider: store.ProviderCursor, Cursor: CursorState{Availability: CursorReady, Meters: []Meter{{Title: "Auto pool", RemainingPercent: 75, UsedPercent: 25, PreserveTitle: true}}}}
	if got := SelectedStatusText(zh, in); got != "75%" {
		t.Fatal("Cursor status fell through")
	}
	f := BuildFloating(zh, in)
	if len(f.Providers) != 1 || f.Providers[0].Meters[0].Label != "Auto pool" {
		t.Fatal("pool name was replaced with a window")
	}
	in.Cursor.Stale = true
	if len(SelectedMeters(zh, in)) != 0 || len(BuildFloating(zh, in).Providers[0].Meters) != 0 {
		t.Fatal("stale quota advertised as live")
	}
}
