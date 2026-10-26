package grok

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

type sessionSpec struct {
	Name   string            `json:"name"`
	TZ     string            `json:"tz"`
	Files  map[string]string `json:"files"`
	Mtimes map[string]int64  `json:"mtimes"`
	Opts   struct {
		Recent int       `json:"recent"`
		Limit  int       `json:"limit"`
		Window *[2]int64 `json:"window"`
		Now    int64     `json:"now"`
	} `json:"opts"`
}

func sessionSec(t time.Time) string {
	return fmt.Sprintf("%.3f", float64(t.Unix())+float64(t.Nanosecond())/1e9)
}

func sessionDec(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return d.String()
}

func sessionTotalsRow(t SessionUsageTotals) []any {
	return []any{t.TotalTokens, t.InputTokens, t.OutputTokens, t.CachedReadTokens, t.ReasoningTokens, t.Turns, t.ModelCalls, sessionDec(t.EstimatedUSD)}
}

func sessionAPIRow(t usage.Totals) []any {
	return []any{t.TotalTokens, t.UncachedInputTokens, t.CachedInputTokens, t.OutputTokens, t.Turns, t.Threads}
}

func sessionBreakdown(rows []usage.BreakdownRow) []any {
	out := []any{}
	for _, r := range rows {
		out = append(out, []any{r.Name, sessionAPIRow(r.Totals), sessionDec(r.EstimatedUSD)})
	}
	return out
}

func sessionOptStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// sessionOracleShape renders a scan result in the shape the Swift oracle prints.
func sessionOracleShape(r SessionUsageSummary) map[string]any {
	c := r.CostSummary
	models := []any{}
	for _, m := range r.ModelRows {
		models = append(models, []any{m.Model, sessionTotalsRow(m.Totals)})
	}
	items := []any{}
	for _, it := range r.RecentItems {
		items = append(items, []any{it.ID, it.Title, it.ProjectName, sessionOptStr(it.CWD), sessionOptStr(it.Model),
			sessionSec(it.UpdatedAt), it.MessageCount, sessionTotalsRow(it.Totals)})
	}
	daily := []any{}
	for _, d := range c.DailyRows {
		daily = append(daily, []any{d.Date, sessionAPIRow(d.Totals), sessionDec(d.EstimatedUSD), d.RawCredits})
	}
	warnings := c.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	dailyTokens := map[string]any{}
	for k, v := range r.DailyTokens {
		dailyTokens[k] = v
	}
	return map[string]any{
		"totals": sessionTotalsRow(r.Totals), "count": r.SessionCount, "models": models, "items": items,
		"daily": dailyTokens, "label": r.SourceLabel, "calc": sessionSec(r.CalculatedAt),
		"cost": map[string]any{
			"source": string(c.Source), "confidence": string(c.Confidence),
			"window": []any{sessionSec(c.Window.Start), sessionSec(c.Window.End)},
			"usd":    sessionDec(c.EstimatedUSD), "totals": sessionAPIRow(c.Totals), "dailyRows": daily,
			"modelRows": sessionBreakdown(c.ModelRows), "projectRows": sessionBreakdown(c.ProjectRows),
			"clientRows": sessionBreakdown(c.ClientRows), "raw": c.RawCredits, "warnings": warnings,
			"pv": c.PricingVersion, "calc": sessionSec(c.CalculatedAt),
		},
	}
}

func sessionNormalize(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// sessionFirstDiff names the first path where two decoded JSON values differ.
func sessionFirstDiff(path string, a, b any) string {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return path
		}
		for k := range x {
			if _, ok := y[k]; !ok {
				return path + "/" + k + " (missing in Swift)"
			}
			if d := sessionFirstDiff(path+"/"+k, x[k], y[k]); d != "" {
				return d
			}
		}
		for k := range y {
			if _, ok := x[k]; !ok {
				return path + "/" + k + " (missing in Go)"
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return fmt.Sprintf("%s len %d vs %v", path, len(x), b)
		}
		for i := range x {
			if d := sessionFirstDiff(fmt.Sprintf("%s[%d]", path, i), x[i], y[i]); d != "" {
				return d
			}
		}
	default:
		if !reflect.DeepEqual(a, b) {
			return fmt.Sprintf("%s: go %v, Swift %v", path, a, b)
		}
	}
	return ""
}

func sessionLoadSpecs(t *testing.T) []sessionSpec {
	t.Helper()
	f, err := os.Open("testdata/billing_sessions_spec.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	var specs []sessionSpec
	if err := json.Unmarshal(data, &specs); err != nil {
		t.Fatal(err)
	}
	return specs
}

// TestSessionScanMatchesSwift materializes every scenario (files, mtimes,
// time zone, window) in a temp directory and compares the scan with the
// output of GrokSessionScanner.
func TestSessionScanMatchesSwift(t *testing.T) {
	specs := sessionLoadSpecs(t)
	var gold []json.RawMessage
	readJSON(t, "testdata/billing_sessions_gold.json", &gold)
	if len(specs) != len(gold) {
		t.Fatalf("%d scenarios, %d goldens", len(specs), len(gold))
	}
	failures := 0
	for i, sp := range specs {
		home := t.TempDir()
		for rel, content := range sp.Files {
			p := filepath.Join(home, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for rel, sec := range sp.Mtimes {
			mt := time.Unix(sec, 0)
			if err := os.Chtimes(filepath.Join(home, filepath.FromSlash(rel)), mt, mt); err != nil {
				t.Fatal(err)
			}
		}
		loc, err := time.LoadLocation(sp.TZ)
		if err != nil {
			t.Fatal(err)
		}
		opts := SessionScanOptions{RecentLimit: sp.Opts.Recent, SessionLimit: sp.Opts.Limit, Now: time.Unix(sp.Opts.Now, 0)}
		if sp.Opts.Window != nil {
			opts.CostWindow = usage.Window{Start: time.Unix(sp.Opts.Window[0], 0), End: time.Unix(sp.Opts.Window[1], 0)}
		}
		got, err := SessionScanner{GrokHome: home, Location: loc}.Scan(context.Background(), opts)
		if err != nil {
			t.Errorf("%d %s: %v", i, sp.Name, err)
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(gold[i]))
		dec.UseNumber()
		var want any
		if err := dec.Decode(&want); err != nil {
			t.Fatal(err)
		}
		// Frozen Swift output predates partial pricing and warned even for
		// unknown rows with no billable tokens; migrate only those expectations
		expected := want.(map[string]any)["cost"].(map[string]any)
		unpriced := int64(0)
		zeroUnknown := map[string]bool{}
		for _, row := range want.(map[string]any)["models"].([]any) {
			model := row.([]any)
			totals := model[1].([]any)
			if totals[7] != nil {
				continue
			}
			number := func(i int) int64 { v, _ := totals[i].(json.Number).Int64(); return v }
			tokens := max(0, number(1)-number(3)) + number(3) + number(2)
			unpriced += tokens
			if tokens == 0 {
				zeroUnknown["unknown-model:"+model[0].(string)] = true
			}
		}
		if expected["confidence"] == "priced" && unpriced > 0 {
			expected["confidence"] = "partiallyPriced"
		}
		filtered := []any{}
		for _, warning := range expected["warnings"].([]any) {
			if !zeroUnknown[warning.(string)] {
				filtered = append(filtered, warning)
			}
		}
		if len(want.(map[string]any)["models"].([]any)) == 0 && func() bool {
			totals := want.(map[string]any)["totals"].([]any)
			number := func(i int) int64 { v, _ := totals[i].(json.Number).Int64(); return v }
			return max(0, number(1)-number(3))+number(3)+number(2) > 0
		}() {
			filtered = append(filtered, "unknown-model:unknown")
		}
		expected["warnings"] = filtered
		if g := sessionNormalize(t, sessionOracleShape(got)); !reflect.DeepEqual(g, want) {
			failures++
			if failures <= 12 {
				t.Errorf("%d %s (%s): scan differs from Swift at %s", i, sp.Name, sp.TZ, sessionFirstDiff("", g, want))
			}
		}
	}
	if failures > 0 {
		t.Errorf("%d of %d scenarios differ", failures, len(specs))
	}
}
