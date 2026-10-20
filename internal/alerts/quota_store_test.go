package alerts

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQuotaThresholdPrecisionAndZero(t *testing.T) {
	for _, tc := range []struct {
		used       float64
		thresholds []int
		want       int
	}{
		{0, []int{0, 100}, 0}, {99.6, []int{100}, -1}, {100, []int{100}, 100},
		{96, []int{95, 80}, 95}, {math.NaN(), []int{0}, -1}, {math.Inf(1), []int{0}, -1},
		{-1, []int{0}, -1}, {101, []int{100}, 100},
	} {
		got := QuotaAlerts(QuotaInput{Primary: Window{UsedPercent: tc.used}}, tc.thresholds)
		if tc.want < 0 {
			if len(got) != 0 {
				t.Fatalf("used %v: %+v", tc.used, got)
			}
			continue
		}
		if len(got) != 1 || *got[0].Threshold != tc.want {
			t.Fatalf("used %v: %+v", tc.used, got)
		}
	}
}

func TestQuotaProgressSurvivesRestartAccountsAndCycles(t *testing.T) {
	path := filepath.Join(t.TempDir(), QuotaStateFileName)
	cycle := time.Unix(2000000000, 0)
	send := func(account string, used float64, reset *time.Time, want int) {
		t.Helper()
		// Reconstruct the store on every reading to exercise restart persistence.
		got, err := (QuotaStore{path}).Unseen(account, QuotaAlerts(QuotaInput{Primary: Window{used, reset}}, []int{0, 80, 95, 100}))
		if err != nil || len(got) != want {
			t.Fatalf("%s used=%v reset=%v: %+v %v", account, used, reset, got, err)
		}
	}
	send("a", 45, &cycle, 1)
	send("a", 96, &cycle, 1)
	send("a", 90, &cycle, 0)
	send("a", 96, &cycle, 0)
	send("b", 96, &cycle, 1)
	for i := 0; i < 250; i++ {
		next := cycle.Add(time.Duration(i+1) * time.Hour)
		send("a", 96, &next, 1)
	}
	latest := cycle.Add(250 * time.Hour)
	send("a", 96, &latest, 0)
	send("a", 100, &latest, 1)
	send("a", 96, &cycle, 0) // old API snapshots must not undo progress
	send("unknown", 0, nil, 1)
	send("unknown", 0, nil, 0)
	send("unknown", 96, nil, 1)
	send("unknown", 90, nil, 0)
	send("unknown", 90, &cycle, 0) // learning reset binds existing progress
	send("unknown", 96, &cycle, 0)
	send("unknown", 100, nil, 1) // temporarily absent reset retains known cycle
	send("unknown", 100, &cycle, 0)
}

func TestQuotaLegacyStateIsNotAttributedToAnotherAccount(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, StateFileName)
	old := []byte(`["quota:5-hour:100:0","reset-credit:kept:1"]`)
	if err := os.WriteFile(legacy, old, 0600); err != nil {
		t.Fatal(err)
	}
	for _, account := range []string{"a", "b"} {
		got, err := (QuotaStore{QuotaPath(legacy)}).Unseen(account, QuotaAlerts(QuotaInput{Primary: Window{100, nil}}, legacyQuotaThresholds))
		if err != nil || len(got) != 1 {
			t.Fatalf("%s: %+v %v", account, got, err)
		}
	}
	got, err := os.ReadFile(legacy)
	if err != nil || string(got) != string(old) {
		t.Fatalf("legacy changed: %s %v", got, err)
	}
}

func TestQuotaCorruptStateFailsWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), QuotaStateFileName)
	for _, data := range []string{`bad`, `{}`, `null`, `{"version":1}`, `{"windows":{}}`, `{"version":1,"windows":{"x":null}}`, `{"version":1,"windows":{"x":{}}}`, `{"version":2,"windows":{}}`, `{"version":1,"windows":{"x":{"highest":-1}}}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := (QuotaStore{path}).Unseen("a", QuotaAlerts(QuotaInput{Primary: Window{100, nil}}, legacyQuotaThresholds))
		if err == nil {
			t.Fatalf("accepted %s", data)
		}
		after, _ := os.ReadFile(path)
		if string(after) != data {
			t.Fatalf("overwrote corrupt state")
		}
	}
}

func TestQuotaEmptyAccountFailsExplicitly(t *testing.T) {
	store := QuotaStore{filepath.Join(t.TempDir(), QuotaStateFileName)}
	if _, err := store.Unseen("", nil); err != nil {
		t.Fatal(err)
	}
	candidates := QuotaAlerts(QuotaInput{Primary: Window{100, nil}}, legacyQuotaThresholds)
	if _, err := store.Unseen("", candidates); err == nil {
		t.Fatal("empty account silently accepted")
	}
	if _, err := os.Stat(store.Path); !os.IsNotExist(err) {
		t.Fatalf("unexpected state write: %v", err)
	}
}
