package runwayfmt

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/resettoday"
	"github.com/licoy/quosparr/internal/testzone"
)

// The frozen fixture comes from the Swift releases' core; only
// the copy fields that need the runway formatters are read here.
type swiftDetail struct {
	Type    *string  `json:"type"`
	Label   string   `json:"label"`
	Percent *string  `json:"percent"`
	Time    *string  `json:"time"`
	Plain   string   `json:"plain"`
	Tokens  []string `json:"tokens"`
}

type swiftHint struct {
	Type string  `json:"type"`
	Text string  `json:"text"`
	Ago  *string `json:"ago"`
}

type swiftProbe struct {
	At         int64        `json:"at"`
	DetailEN   *swiftDetail `json:"detailEN"`
	DetailZH   *swiftDetail `json:"detailZH"`
	NoneHintEN *swiftHint   `json:"noneHintEN"`
	NoneHintZH *swiftHint   `json:"noneHintZH"`
}

type swiftCase struct {
	Name        string       `json:"name"`
	Feed        string       `json:"feed"`
	NowMs       int64        `json:"nowMs"`
	Zone        string       `json:"zone"`
	DecodeError bool         `json:"decodeError"`
	Probes      []swiftProbe `json:"probes"`
}

func loadCases(t *testing.T) []swiftCase {
	t.Helper()
	f, err := os.Open("../testdata/swift_cases.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var cases []swiftCase
	if err := json.NewDecoder(zr).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func tokenNames(tokens []resettoday.DetailToken) []string {
	out := make([]string, 0, len(tokens))
	for _, tk := range tokens {
		switch tk.Kind {
		case resettoday.TokenText:
			out = append(out, "text:"+tk.Text)
		case resettoday.TokenResetType:
			out = append(out, "type")
		case resettoday.TokenPercent:
			out = append(out, "percent")
		case resettoday.TokenTime:
			out = append(out, "time")
		}
	}
	return out
}

func compareDetail(t *testing.T, label string, got *resettoday.DetailPresentation, want *swiftDetail) {
	t.Helper()
	if got == nil || want == nil {
		if got != nil || want != nil {
			t.Errorf("%s: got %+v want %+v", label, got, want)
		}
		return
	}
	var gotType *string
	if got.ResetType != nil {
		s := string(*got.ResetType)
		gotType = &s
	}
	g := swiftDetail{gotType, got.TypeLabel, got.PercentText, got.TimeText, got.PlainText, tokenNames(got.Tokens)}
	if !reflect.DeepEqual(g, *want) {
		t.Errorf("%s:\n got %+v\nwant %+v", label, g, *want)
	}
}

func compareHint(t *testing.T, label string, got *resettoday.NoneHint, want *swiftHint) {
	t.Helper()
	if got == nil || want == nil {
		if got != nil || want != nil {
			t.Errorf("%s: got %+v want %+v", label, got, want)
		}
		return
	}
	if string(got.ResetType) != want.Type || got.Text != want.Text || (want.Ago != nil && got.Ago != *want.Ago) {
		t.Errorf("%s: got %+v want %+v", label, *got, *want)
	}
}

// TestDetailAndNoneHintMatchSwift compares the verdict detail line (token by
// token, with the Swift date/relative formatting) and the "none" hint for every
// generated case, in English and Simplified Chinese.
func TestDetailAndNoneHintMatchSwift(t *testing.T) {
	var checked int
	for _, c := range loadCases(t) {
		if c.DecodeError || len(c.Probes) == 0 {
			continue
		}
		loc, err := testzone.LoadLocation(c.Zone)
		if err != nil {
			t.Fatal(err)
		}
		p := c.Probes[0]
		now := time.UnixMilli(p.At).UTC()
		snap, err := resettoday.Decode([]byte(c.Feed), time.UnixMilli(c.NowMs).UTC(), loc)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		en, zh := l10n.New(l10n.English), l10n.New(l10n.SimplifiedChinese)
		name := c.Name + "/" + c.Zone
		compareDetail(t, name+" detailEN", snap.VerdictDetail(en, Formatter{}, now, loc), p.DetailEN)
		compareDetail(t, name+" detailZH", snap.VerdictDetail(zh, Formatter{}, now, loc), p.DetailZH)
		compareHint(t, name+" noneHintEN", snap.NoneHintLastReset(en, Formatter{}, now), p.NoneHintEN)
		compareHint(t, name+" noneHintZH", snap.NoneHintLastReset(zh, Formatter{}, now), p.NoneHintZH)
		checked++
	}
	if checked < 100 {
		t.Fatalf("only %d cases compared", checked)
	}
}
