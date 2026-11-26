package claude

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseUsageRejectsUnusableLimits(t *testing.T) {
	for _, raw := range []string{
		`{"limits":[{"kind":"session"}],"five_hour":{"utilization":3}}`,
		`{"limits":[{"kind":"session","percent":"bad"}],"five_hour":{"utilization":3}}`,
	} {
		snap, err := ParseUsageSnapshot([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if len(snap.Windows) != 1 || snap.Windows[0].UsedPercent == nil || *snap.Windows[0].UsedPercent != 3 {
			t.Fatalf("unusable limit blocked valid flat quota: %+v", snap.Windows)
		}
	}

	for _, raw := range []string{
		`{"limits":[{"label":"new schema","value":9}]}`,
		`{"limits":{"metadata":{"request_id":"x"}}}`,
		`{"metadata":{"request_id":"x"},"limits":[{"kind":"session","percent":"bad"}]}`,
	} {
		if _, err := ParseUsageSnapshot([]byte(raw)); !errors.Is(err, ErrUsageParse) {
			t.Fatalf("invalid limits succeeded for %s: %v", raw, err)
		}
	}
}

func TestParseUsageValidLimitsShapesRemainAuthoritative(t *testing.T) {
	for _, raw := range []string{
		`{"limits":[{"kind":"session","percent":0},{"kind":"weekly_all","percent":null}],"five_hour":{"utilization":99}}`,
		`{"limits":{"session":{"percent":0},"weekly_all":{"percent":null}},"five_hour":{"utilization":99}}`,
	} {
		snap, err := ParseUsageSnapshot([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if len(snap.Windows) != 2 || snap.Windows[0].UsedPercent == nil || *snap.Windows[0].UsedPercent != 0 || !snap.Windows[1].Null {
			t.Fatalf("valid limits lost for %s: %+v", raw, snap.Windows)
		}
	}
}

func TestRefreshUsageParseFailureKeepsCachedQuota(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"limits":[{"kind":"session","percent":"bad"}],"metadata":{"request_id":"x"}}`))
	}))
	defer srv.Close()

	home := t.TempDir()
	store := NewStoreAt(home+"/accounts", home+"/.claude", home)
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	id := seedAccount(t, store, "acct-invalid", "org-invalid", false, now.Add(time.Hour))
	idx, err := store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	acc, _ := idx.Account(id)
	previousAt := now.Add(-time.Hour)
	if err := store.UpdateMetadata(acc.ApplyQuota([]byte(`{"five_hour":{"utilization":7}}`), previousAt)); err != nil {
		t.Fatal(err)
	}
	idx, err = store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	acc, _ = idx.Account(id)
	previousQuota := append([]byte(nil), acc.CachedQuota...)

	mod := NewModule(store)
	mod.Now = func() time.Time { return now }
	mod.Usage = &UsageClient{HTTP: srv.Client(), Endpoint: srv.URL, Now: mod.Now}
	if err := mod.Refresh(context.Background(), id, true); !errors.Is(err, ErrUsageParse) {
		t.Fatalf("refresh error = %v, want ErrUsageParse", err)
	}
	idx, err = store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	acc, _ = idx.Account(id)
	if acc.LastError != ErrorCodeUsageParse {
		t.Fatalf("last error = %q", acc.LastError)
	}
	if string(acc.CachedQuota) != string(previousQuota) || acc.LastQuotaAt == nil || !acc.LastQuotaAt.Equal(previousAt) {
		t.Fatalf("cached quota changed: quota=%s at=%v", acc.CachedQuota, acc.LastQuotaAt)
	}
}
