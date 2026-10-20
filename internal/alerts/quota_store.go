package alerts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/licoy/quosparr/internal/platform"
)

// QuotaStateFileName is separate from the frozen Swift ID history. Legacy
// quota IDs have no account identity and cannot safely seed account progress;
// upgrading can therefore notify the current highest tier once per window.
const QuotaStateFileName = "quota-alert-state.json"

type quotaProgress struct {
	Cycle   int64 `json:"cycle"`
	Highest int   `json:"highest"`
}

// UnmarshalJSON distinguishes a valid zero tier/cycle from missing fields.
func (p *quotaProgress) UnmarshalJSON(data []byte) error {
	var raw struct {
		Cycle   *int64 `json:"cycle"`
		Highest *int   `json:"highest"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Cycle == nil || raw.Highest == nil {
		return fmt.Errorf("incomplete quota alert progress")
	}
	p.Cycle, p.Highest = *raw.Cycle, *raw.Highest
	return nil
}

type quotaState struct {
	Version int                      `json:"version"`
	Windows map[string]quotaProgress `json:"windows"`
}

// QuotaStore retains one latest cycle per account/window, rather than a capped
// list of threshold IDs. It remembers progress before notification dispatch,
// matching Store.Unseen's existing delivery semantics.
type QuotaStore struct{ Path string }

func QuotaPath(alertsPath string) string {
	return filepath.Join(filepath.Dir(alertsPath), QuotaStateFileName)
}

// Unseen returns only candidates above the highest already notified tier.
// Missing reset times use a stable unknown cycle, which never rolls over by
// wall clock. Learning a reset time binds existing unknown-cycle progress;
// temporarily absent reset times preserve that cycle. Older snapshots cannot
// replace a newer known cycle.
func (s QuotaStore) Unseen(account string, candidates []Alert) ([]Alert, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if account == "" {
		return nil, fmt.Errorf("quota alert account identity is missing")
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	state, err := s.load()
	if err != nil {
		return nil, err
	}
	accountHash := fmt.Sprintf("%x", sha256.Sum256([]byte(account)))
	var unseen []Alert
	changed := false
	for _, a := range candidates {
		if a.Kind != KindQuota || a.Threshold == nil || *a.Threshold < 0 || *a.Threshold > 100 {
			continue
		}
		key := accountHash + ":" + a.Name
		cycle := int64(0)
		if a.Date != nil {
			cycle = a.Date.Unix()
		}
		prev, exists := state.Windows[key]
		if exists && cycle == 0 {
			cycle = prev.Cycle
		}
		if exists && prev.Cycle == 0 && cycle > 0 {
			prev.Cycle = cycle
			state.Windows[key] = prev
			changed = true
		}
		if exists && (cycle < prev.Cycle || cycle == prev.Cycle && *a.Threshold <= prev.Highest) {
			continue
		}
		state.Windows[key] = quotaProgress{Cycle: cycle, Highest: *a.Threshold}
		changed = true
		a.ID = "quota-account:" + accountHash + ":" + a.ID
		unseen = append(unseen, a)
	}
	if !changed {
		return nil, nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	if err := platform.WriteFileAtomic(s.Path, data, 0o600); err != nil {
		return nil, err
	}
	return unseen, nil
}

func (s QuotaStore) load() (quotaState, error) {
	var state quotaState
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return quotaState{Version: 1, Windows: map[string]quotaProgress{}}, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("quota alert state: %w", err)
	}
	if state.Version != 1 || state.Windows == nil {
		return state, fmt.Errorf("invalid quota alert state")
	}
	for _, p := range state.Windows {
		if p.Cycle < 0 || p.Highest < 0 || p.Highest > 100 {
			return state, fmt.Errorf("invalid quota alert progress")
		}
	}
	return state, nil
}
