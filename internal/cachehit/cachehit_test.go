package cachehit

import (
	"math"
	"testing"
)

func TestTotalsPercentIsTokenWeighted(t *testing.T) {
	var totals Totals
	if _, ok := totals.Percent(); ok {
		t.Fatal("empty totals have a percentage")
	}
	if err := totals.AddSample(100, 50); err != nil {
		t.Fatal(err)
	}
	if err := totals.AddSample(900, 90); err != nil {
		t.Fatal(err)
	}
	if err := totals.AddMissing(); err != nil {
		t.Fatal(err)
	}
	got, ok := totals.Percent()
	if !ok || math.Abs(got-14) > 1e-12 {
		t.Fatalf("percent = %v, %v", got, ok)
	}
	if totals.Samples != 2 || totals.MissingSamples != 1 {
		t.Fatalf("totals = %+v", totals)
	}
}

func TestZeroInputHasNoPercentage(t *testing.T) {
	var totals Totals
	if err := totals.AddSample(0, 0); err != nil {
		t.Fatal(err)
	}
	if totals.Samples != 1 {
		t.Fatalf("totals = %+v", totals)
	}
	if _, ok := totals.Percent(); ok {
		t.Fatal("zero input has a percentage")
	}
}

func TestReportCloneOwnsSlices(t *testing.T) {
	r := Report{Models: []Row{{ID: "a"}}, Sessions: []Row{{ID: "s"}}}
	c := r.Clone()
	c.Models[0].ID = "b"
	c.Sessions[0].ID = "t"
	if r.Models[0].ID != "a" || r.Sessions[0].ID != "s" {
		t.Fatalf("clone changed source: %+v", r)
	}
}
