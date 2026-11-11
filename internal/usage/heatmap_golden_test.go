package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"
)

func fnv64(s string) uint64 {
	h := uint64(1469598103934665603)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

func goldenTok(key, salt string) int64 {
	h := fnv64(key + salt)
	if h%5 == 0 {
		return 0
	}
	return int64(h % 100000)
}

func goldenMaps() (all, local map[string]int64) {
	all, local = map[string]int64{}, map[string]int64{}
	for d := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC); d.Year() < 2028; d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		all[k], local[k] = goldenTok(k, "A"), goldenTok(k, "L")
	}
	return
}

func heatDump(h HeatmapSnapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%d|%d|%d|%v|%v|%v\n", h.Mode, h.TotalTokens, h.TotalAllDevices, h.TotalLocal, h.HasAllDevicesData, h.HasLocalData, h.HasUsage)
	for i, w := range h.Weeks {
		for _, c := range w {
			fmt.Fprintf(&b, "%d|%s|%d|%d|%d|%d|%d|%v\n", i, c.DayKey, c.Date.Unix(), c.Tokens, c.AllDevicesTokens, c.LocalTokens, c.Level, c.InRange)
		}
	}
	for _, l := range h.MonthLabels {
		fmt.Fprintf(&b, "M%d|%d\n", l.WeekIndex, l.Month)
	}
	return b.String()
}

func seriesDump(s ChartSeries) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%d|%d|%d|%v|%v|%v\n", s.Mode, s.TotalTokens, s.TotalAllDevices, s.TotalLocal, s.HasAllDevicesData, s.HasLocalData, s.HasUsage)
	for _, p := range s.Points {
		fmt.Fprintf(&b, "%s|%d|%d|%d|%d\n", p.DayKey, p.Date.Unix(), p.Tokens, p.AllDevicesTokens, p.LocalTokens)
	}
	return b.String()
}

func hex64(s string) string { return strconv.FormatUint(fnv64(s), 16) }

func loadZone(t *testing.T, name string) *time.Location {
	t.Helper()
	// The zone rules are part of the Swift golden input, not the runner's OS.
	data, err := os.ReadFile(filepath.Join("testdata", "timezones", filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocationFromTZData(name, data)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// TestHeatmapMatchesSwiftGolden replays 3420 (zone, now, first weekday, mode)
// scenarios, including zones whose midnight does not exist or repeats, and
// compares a digest of every cell and chart point with the Swift builder's.
func TestHeatmapMatchesSwiftGolden(t *testing.T) {
	if raceEnabled {
		// Pure single-goroutine math: the detector finds nothing here and
		// makes the 3,420 scenarios ten times slower; non-race runs cover it.
		t.Skip("covered by non-race runs")
	}
	all, local := goldenMaps()
	locs := map[string]*time.Location{}
	bad := 0
	for _, line := range readLines(t, "heatmap_golden.txt") {
		f := strings.Fields(line)
		if len(f) != 8 {
			t.Fatalf("bad golden line %q", line)
		}
		loc := locs[f[0]]
		if loc == nil {
			loc = loadZone(t, f[0])
			locs[f[0]] = loc
		}
		n, _ := strconv.ParseInt(f[1], 10, 64)
		fw, _ := strconv.Atoi(f[2])
		in := HeatmapInput{AllDevices: all, Local: local, Mode: HeatmapMode(f[3]), Now: time.Unix(n, 0), FirstWeekday: fw, Loc: loc}
		h, s := heatDump(BuildHeatmap(in)), seriesDump(BuildChartSeries(in))
		if hex64(h) != f[4] || hex64(s) != f[5] {
			bad++
			if bad <= 5 {
				t.Errorf("%s %s fw=%d %s: heatmap %v series %v", f[0], f[1], fw, f[3], hex64(h) == f[4], hex64(s) == f[5])
			}
		}
	}
	if bad > 0 {
		t.Fatalf("%d scenarios differ", bad)
	}
}

func TestYearToDateWindowMatchesSwiftGolden(t *testing.T) {
	for _, line := range readLines(t, "ytd_golden.txt") {
		f := strings.Fields(line)
		n, _ := strconv.ParseInt(f[1], 10, 64)
		w := YearToDateWindow(time.Unix(n, 0), loadZone(t, f[0]))
		if got := fmt.Sprintf("%d %d", w.Start.Unix(), w.End.Unix()); got != f[2]+" "+f[3] {
			t.Errorf("%s: got %s want %s %s", line, got, f[2], f[3])
		}
	}
}

func TestCostRangeMatchesSwiftGolden(t *testing.T) {
	n := 0
	for _, line := range readLines(t, "costrange_golden.txt") {
		f := strings.Fields(line)
		zone, sec, kind := f[0], f[1], f[2]
		loc := loadZone(t, zone)
		now, _ := strconv.ParseInt(sec, 10, 64)
		nowT := time.Unix(now, 0)
		sod := newDayCalendar(loc, 1).startOfDay(nowT)
		cal := newDayCalendar(loc, 1)
		win := Window{Start: cal.addDays(sod, -7), End: sod}
		win2 := Window{Start: nowT.Add(-(5*86400 + 777) * time.Second), End: nowT}
		var r CostRange
		ok := true
		switch kind {
		case "month":
			r = ThisMonth(nowT, loc)
		case "today":
			r = Today(nowT, loc)
		case "custom":
			r, ok = CustomRange(nowT.Add(-(3*86400+1234)*time.Second), nowT, loc)
		case "customrev":
			r, ok = CustomRange(nowT, nowT.Add(-2*86400*time.Second), loc)
		case "range":
			r = RangeForWindow(win, loc)
		case "prev":
			r = PreviousCycle(win, loc)
		case "range2":
			r = RangeForWindow(win2, loc)
		case "prev2":
			r = PreviousCycle(win2, loc)
		case "zero":
			r = RangeForWindow(Window{Start: nowT, End: nowT}, loc)
		default:
			t.Fatalf("unknown kind %s", kind)
		}
		want := strings.Join(f[3:], " ")
		got := "nil"
		if ok {
			got = fmt.Sprintf("%.3f %.3f %s %s", float64(r.Window.Start.UnixNano())/1e9, float64(r.Window.End.UnixNano())/1e9, r.APIStartDate, r.APIEndDate)
		}
		if got != want {
			t.Errorf("%s %s %s: got %q want %q", zone, sec, kind, got, want)
		}
		n++
	}
	if n < 3000 {
		t.Fatalf("only %d rows", n)
	}
}
