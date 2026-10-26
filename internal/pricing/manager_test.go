package pricing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

const sample = `{"openai":{"name":"OpenAI","models":{"fresh":{"id":"fresh","name":"Fresh","cost":{"input":2,"output":0,"cache_read":null,"tiers":[{"input":4,"output":1,"tier":{"type":"context","size":272000}}],"context_over_200k":{"input":4,"output":1}},"env":["IGNORED_SECRET"],"api":"not executed"}}},"other":{"models":{"fresh":{"cost":{"input":100,"output":200}}}}}`

func TestParseOptionalZeroIdentityAndCopies(t *testing.T) {
	s, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := s.Lookup("openai", "fresh")
	if !ok || m.Rate.Output == nil || !m.Rate.Output.IsZero() || m.Rate.CacheRead != nil {
		t.Fatalf("model %#v", m)
	}
	if len(m.Tiers) != 1 || m.Tiers[0].Size != 272000 || m.Tiers[0].Rate.Input.String() != "4" {
		t.Fatalf("tier %#v", m.Tiers)
	}
	*m.Rate.Input = decimal.FromInt(99)
	m.Tiers[0].Size = 1
	again, _ := s.Lookup("openai", "fresh")
	if again.Rate.Input.String() != "2" || again.Tiers[0].Size != 272000 {
		t.Fatal("mutable snapshot")
	}
	if _, ok := s.Lookup("missing", "fresh"); ok {
		t.Fatal("cross-provider match")
	}
	for _, data := range []string{`{}`, `null`, `{"p":{"models":{"x":{"cost":{"input":-1}}}}}`, `{"p":{"models":{"x":{"cost":{"input":"2"}}}}}`, `{"p":{"models":{"x":{"cost":{"input":1e999}}}}}`} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
func TestManagerCache304SameContentAndFailure(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), CacheFileName)
	response := Response{200, []byte(sample), "one"}
	var calls int
	var fail error
	fetch := func(_ context.Context, etag string) (Response, error) {
		calls++
		if calls > 1 && etag == "" {
			t.Fatal("missing validator")
		}
		return response, fail
	}
	m := NewManager(Config{CachePath: path, Fetch: fetch, Now: func() time.Time { return now }})
	if calls != 0 {
		t.Fatal("network on load")
	}
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := m.Status()
	if first.LastChanged != now || first.ETag != "one" {
		t.Fatalf("status %+v", first)
	}
	if err := platform.VerifyOwnerOnly(path, 0o600); err != nil {
		t.Fatalf("cache permissions: %v", err)
	}
	now = now.Add(time.Hour)
	response = Response{304, nil, ""}
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Status().Version != first.Version || m.Status().LastChanged != first.LastChanged || m.Status().LastSuccess != now {
		t.Fatal("304 changed version")
	}
	now = now.Add(time.Hour)
	response = Response{200, []byte(strings.ReplaceAll(sample, "Fresh", "Display rename")), "two"}
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Status().Version != first.Version || m.Status().ETag != "two" {
		t.Fatal("metadata changed price version")
	}
	restored := NewManager(Config{CachePath: path})
	if restored.Snapshot().Version() != first.Version {
		t.Fatal("cache not restored")
	}
	response = Response{200, []byte(`{"broken":`), "bad"}
	if err := m.Refresh(context.Background()); err == nil {
		t.Fatal("accepted invalid")
	}
	if m.Status().Version != first.Version || m.Status().ETag != "two" || m.Status().Error == "" {
		t.Fatal("failed fetch replaced good state")
	}
	response = Response{200, make([]byte, MaxResponseBytes+1), "big"}
	if !errors.Is(m.Refresh(context.Background()), ErrResponseTooLarge) {
		t.Fatal("oversize accepted")
	}
	fail = context.DeadlineExceeded
	if err := m.Refresh(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestSaveFailureDoesNotPublish(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dir")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	m := NewManager(Config{CachePath: path, Fetch: func(context.Context, string) (Response, error) { return Response{200, []byte(sample), "x"}, nil }})
	if m.Refresh(context.Background()) == nil || m.Snapshot().Version() != "" || m.Status().LastSuccess.IsZero() == false {
		t.Fatal("published unsaved catalog")
	}
}
func TestSingleFlightAndClose(t *testing.T) {
	var calls atomic.Int64
	started := make(chan struct{})
	m := NewManager(Config{Fetch: func(ctx context.Context, _ string) (Response, error) {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		return Response{}, ctx.Err()
	}})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = m.Refresh(context.Background()) }()
	<-started
	if !m.Status().Running {
		t.Fatal("not running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(m.Refresh(ctx), context.Canceled) {
		t.Fatal("waiter ignores cancellation")
	}
	m.Close()
	wg.Wait()
	if calls.Load() != 1 || m.Status().Running || m.Refresh(context.Background()) == nil {
		t.Fatal("close/flight failed")
	}
}
func TestFetchHonorsBlockedProxy(t *testing.T) {
	netx.Block(errors.New("test blocked proxy"))
	defer netx.Configure(nil)
	if _, err := Fetch(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "test blocked proxy") {
		t.Fatalf("proxy ignored: %v", err)
	}
}
func TestProviderVersionIndependent(t *testing.T) {
	a, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse([]byte(strings.Replace(sample, `"input":100`, `"input":101`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if a.Version() == b.Version() || a.ProviderVersion("openai") != b.ProviderVersion("openai") {
		t.Fatal("provider hash not isolated")
	}
}

func TestMissingAllPricesDoesNotReplaceSuccessfulSnapshot(t *testing.T) {
	body := []byte(sample)
	m := NewManager(Config{Fetch: func(context.Context, string) (Response, error) { return Response{200, body, "etag"}, nil }})
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := m.Snapshot().Version()
	body = []byte(`{"p":{"models":{"missing":{"cost":{"input":null}}}}}`)
	if err := m.Refresh(context.Background()); err == nil || m.Snapshot().Version() != before {
		t.Fatalf("all-missing catalog published %v", err)
	}
}

func TestDirectoryRevisionChangesWithoutPriceVersion(t *testing.T) {
	a, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse([]byte(strings.ReplaceAll(sample, "Fresh", "Rename")))
	if err != nil {
		t.Fatal(err)
	}
	if a.Revision() == b.Revision() || a.Version() != b.Version() {
		t.Fatal("display metadata revision mixed with price version")
	}
	models := a.Models()
	models = append(models, Model{ProviderID: "new-provider", ID: "no-prices"})
	c, err := NewSnapshot(models)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version() == a.Version() || c.ProviderVersion("new-provider") == "" || c.Revision() == a.Revision() {
		t.Fatal("model lookup presence missing from effective-price version")
	}
}

func TestFetchResolvesChangedProxyForEveryRequest(t *testing.T) {
	defer netx.Configure(nil)
	netx.Block(errors.New("first proxy configuration"))
	if _, err := Fetch(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "first proxy configuration") {
		t.Fatal(err)
	}
	netx.Block(errors.New("second proxy configuration"))
	if _, err := Fetch(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "second proxy configuration") {
		t.Fatal("stale network configuration", err)
	}
}

func TestMissingRateRuleStructureChangesProviderVersion(t *testing.T) {
	a, err := Parse([]byte(`{"other":{"models":{"priced":{"cost":{"input":1}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse([]byte(`{"other":{"models":{"priced":{"cost":{"input":1}}}},"openai":{"models":{"gpt-5":{"cost":{"context_over_200k":{}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if a.Version() == b.Version() || a.ProviderVersion("openai") == b.ProviderVersion("openai") {
		t.Fatal("missing prices changed rule lookup without changing version")
	}
}
