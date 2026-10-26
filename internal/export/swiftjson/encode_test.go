package swiftjson

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type scalars struct {
	Doubles  []float64 `json:"doubles"`
	Strings  []string  `json:"strings"`
	Decimals []Decimal `json:"decimals"`
	Date     time.Time `json:"date"`
}

func swiftScalars() scalars {
	a, b := 0.1, 0.2
	negZero := math.Copysign(0, -1)
	texts := []string{
		"0", "1", "12.5", "0.00132", "0.132", "0.0264", "123456789012345678901234567890", "1000", "0.216048", "4.25", "100.10", "-3.75",
		"0.1", "99999999.99999999",
	}
	decimals := make([]Decimal, len(texts))
	for i, s := range texts {
		decimals[i] = MustDecimal(s)
	}
	return scalars{
		Doubles: []float64{
			0, negZero, 1, 12, 0.04, 0.5, 100, 1234.5678, 33.333333333333336, a + b, 3379.6, 112662.5,
			1e15, 9007199254740992, 9007199254740994, 1e16, 1e20, 1e21, 1.2345678901234568e17,
			0.0001, 0.00012345, 0.00001, 1e-7, 1.5e-10, 5e-324, 1.7976931348623157e308, -2.5, -1e-5, -123456.789,
			float64(math.MaxInt64), 4503599627370496.5, 0.30000000000000004, 2.2250738585072014e-308, 123456789.123456789,
		},
		Strings: []string{
			"", "plain", "a/b", "a\\b", "quote\"inside", "<tag>&amp;", "tab\tnew\nline\rret", "\b\f\x00\x01\x1f",
			"del\x7fend", "café", "  ", "emoji \U0001F600 \U0001F468‍\U0001F469", "中文/路径\\x", "https://x.com/a/b?c=d&e=f",
		},
		Decimals: decimals,
		// Swift fixture: 1_800_000_000.987 seconds, fraction dropped.
		Date: time.Unix(1_800_000_000, 987_000_000),
	}
}

func TestScalarsMatchSwiftEncoder(t *testing.T) {
	for name, pretty := range map[string]bool{"swift_scalars.json": false, "swift_scalars_pretty.json": true} {
		got, err := Marshal(swiftScalars(), pretty)
		if err != nil {
			t.Fatal(err)
		}
		if want := fixture(t, name); !bytes.Equal(got, want) {
			t.Fatalf("%s differs\n got: %s\nwant: %s", name, got, want)
		}
	}
}

func TestEmptiesAndNilsMatchSwift(t *testing.T) {
	type empties struct {
		A []int          `json:"a"`
		B map[string]int `json:"b"`
		C *string        `json:"c"`
		D [][]int        `json:"d"`
		E int            `json:"e"`
	}
	v := empties{D: [][]int{{}, {1}}, E: 1}
	for name, pretty := range map[string]bool{"swift_empties.json": false, "swift_empties_pretty.json": true} {
		got, err := Marshal(v, pretty)
		if err != nil {
			t.Fatal(err)
		}
		if want := fixture(t, name); !bytes.Equal(got, want) {
			t.Fatalf("%s differs\n got: %q\nwant: %q", name, got, want)
		}
	}
}

func TestOmitNil(t *testing.T) {
	type v struct {
		A []int `json:"a,omitnil"`
		B []int `json:"b,omitnil"`
		C []int `json:"c"`
	}
	got, err := Marshal(v{B: []int{}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"b":[],"c":[]}` {
		t.Fatalf("got %s", got)
	}
}

func TestDecimalParsing(t *testing.T) {
	if _, err := ParseDecimal("abc"); err == nil {
		t.Fatal("garbage must not parse")
	}
	d := MustDecimal("1").Add(MustDecimal("0.5"))
	if d.String() != "1.5" || d.Cmp(MustDecimal("1.5")) != 0 {
		t.Fatalf("got %s", d)
	}
	var back Decimal
	if err := back.UnmarshalJSON([]byte("12.50")); err != nil || back.String() != "12.5" {
		t.Fatalf("unmarshal: %v %s", err, back)
	}
	if err := back.UnmarshalJSON([]byte(`"1"`)); err == nil {
		t.Fatal("a string is not a Swift Decimal number")
	}
}

func TestPointerToTimeDropsFraction(t *testing.T) {
	at := time.Unix(1_800_000_000, 987_000_000)
	type v struct {
		P *time.Time `json:"p"`
		V time.Time  `json:"v"`
		D *Decimal   `json:"d"`
	}
	d := MustDecimal("1.50")
	got, err := Marshal(v{&at, at, &d}, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"d":1.5,"p":"2027-01-15T08:00:00Z","v":"2027-01-15T08:00:00Z"}` {
		t.Fatalf("got %s", got)
	}
}

func TestCompatibilityOptionsPreserveCodexLayout(t *testing.T) {
	value := map[string]any{"b": []any{}, "A": "x/\u2028", "a": json.Number("1.00")}
	got, err := MarshalWithOptions(value, Options{
		Pretty: true, WithoutEscapingSlashes: true, CompactEmptyContainers: true,
		EscapeLineSeparators: true,
		KeyLess: func(a, b string) bool {
			if x, y := strings.ToLower(a), strings.ToLower(b); x != y {
				return x < y
			}
			return a < b
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"A\" : \"x/\\u2028\",\n  \"a\" : 1.00,\n  \"b\" : []\n}"
	if string(got) != want {
		t.Fatalf("unexpected layout: %q", got)
	}
}

func TestJSONSerializationNumberNormalization(t *testing.T) {
	value := []json.Number{"1.0", "0.1", "1e3", "18446744073709551616", "0.123456789012345678901234"}
	got, err := MarshalWithOptions(value, Options{NormalizeNumbers: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[1,0.10000000000000001,1000,18446744073709551616,0.123456789012345678901234]` {
		t.Fatalf("unexpected normalized numbers: %s", got)
	}
}

func TestInvalidNumberAndUnsupportedValuesFail(t *testing.T) {
	for _, v := range []any{json.Number("null"), json.Number("01"), json.Number("1 2"), make(chan int)} {
		if _, err := MarshalWithOptions(v, Options{}); err == nil {
			t.Fatal("invalid JSON values must fail explicitly")
		}
	}
}

func TestOptInZeroOmissionRetainsLegacyFields(t *testing.T) {
	type row struct {
		Legacy  int   `json:"legacy,omitempty"`
		Count   int64 `json:"count,omitempty,omitzero"`
		Partial bool  `json:"partial,omitempty,omitzero"`
	}
	for _, tc := range []struct {
		in   row
		want string
	}{{row{}, `{"legacy":0}`}, {row{Count: 4, Partial: true}, `{"count":4,"legacy":0,"partial":true}`}} {
		got, err := Marshal(tc.in, false)
		if err != nil || string(got) != tc.want {
			t.Fatalf("%s %v want %s", got, err, tc.want)
		}
	}
}
