package app

import (
	"reflect"
	"sync"

	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// livePreferenceWork serializes UI requests that can wait for service state
// or disk. Bursts retain the newest preferences and one refresh per section.
// Service.SetPreferences keeps its synchronous contract for core callers.
type livePreferenceWork struct {
	mu              sync.Mutex
	idle            sync.WaitGroup
	running, closed bool
	pending         *store.Preferences
	provider        *store.Provider
	refresh         bool
	repair          bool
	sections        map[runway.Section]bool
}

type livePreferenceRequest struct {
	prefs           *store.Preferences
	provider        *store.Provider
	refresh, repair bool
	section         runway.Section
}

// RequestLiveRefresh is the UI entry point for a full refresh; taking service
// locks and applying any pending UI preferences happens on the runtime worker.
func RequestLiveRefresh() {
	if rt := currentRuntime(); rt != nil {
		rt.requestRefresh(nil, "")
	}
}

// RequestLiveSessionRepair schedules the backed-up session-index repair on
// the same worker and lifecycle as panel refreshes.
func RequestLiveSessionRepair() {
	if rt := currentRuntime(); rt != nil {
		rt.queuePreferences(livePreferenceRequest{repair: true})
	}
}

func (rt *liveApp) enqueuePreferences(p store.Preferences) {
	rt.queuePreferences(livePreferenceRequest{prefs: &p})
}

func (rt *liveApp) requestRefresh(s *Settings, section runway.Section) {
	var p *store.Preferences
	if s != nil {
		current := s.Prefs()
		p = &current
	}
	rt.queuePreferences(livePreferenceRequest{prefs: p, refresh: section == "", section: section})
}

func (rt *liveApp) queuePreferences(req livePreferenceRequest) {
	p, provider, refresh, section := req.prefs, req.provider, req.refresh, req.section
	w := &rt.preferenceWork
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if p != nil {
		copy := p.Clone()
		w.pending = &copy
		w.provider = nil
	}
	if provider != nil {
		copy := *provider
		w.provider = &copy
	}
	w.refresh = w.refresh || refresh
	w.repair = w.repair || req.repair
	if section != "" {
		if w.sections == nil {
			w.sections = map[runway.Section]bool{}
		}
		w.sections[section] = true
	}
	if !w.running {
		w.running = true
		w.idle.Add(1)
		go rt.drainPreferences()
	}
}

func (rt *liveApp) drainPreferences() {
	w := &rt.preferenceWork
	defer w.idle.Done()
	for {
		w.mu.Lock()
		p, provider, refresh, repair, sections := w.pending, w.provider, w.refresh, w.repair, w.sections
		w.pending, w.provider, w.refresh, w.repair, w.sections = nil, nil, false, false, nil
		if p == nil && provider == nil && !refresh && !repair && len(sections) == 0 {
			w.running = false
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
		if p != nil {
			rt.applyPreferences(*p)
		}
		if provider != nil {
			rt.svc.SelectProvider(*provider)
		}
		w.mu.Lock()
		newer := w.pending != nil || w.provider != nil
		if newer {
			w.refresh = w.refresh || refresh
			w.repair = w.repair || repair
			for section := range sections {
				if w.sections == nil {
					w.sections = map[runway.Section]bool{}
				}
				w.sections[section] = true
			}
		}
		closing := w.closed
		w.mu.Unlock()
		if newer || closing {
			continue
		}
		if repair {
			rt.svc.RepairSessions()
		}
		if refresh {
			rt.svc.Refresh(usage.RefreshForce)
			// The public reset feed has its own refresh cycle; a full account
			// refresh must not consume a queued panel-open/manual feed request.
			if sections[runway.SectionResetToday] {
				refreshSection(rt.svc, runway.SectionResetToday)
			}
		} else {
			for section := range sections {
				refreshSection(rt.svc, section)
			}
		}
	}
}

func (rt *liveApp) applyPreferences(p store.Preferences) {
	if reflect.DeepEqual(rt.svc.Preferences(), p) {
		return
	}
	rt.grok.setPreferences(p, rt.langs)
	if rt.claudeB != nil {
		rt.claudeB.setPreferences(p, rt.langs)
	}
	if rt.cursorB != nil {
		rt.cursorB.setPreferences(p, rt.langs)
	}
	rt.reset.setPreferences(p, rt.langs)
	rt.svc.SetPreferences(p)
}

func (rt *liveApp) closePreferences() {
	w := &rt.preferenceWork
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	w.idle.Wait()
}
