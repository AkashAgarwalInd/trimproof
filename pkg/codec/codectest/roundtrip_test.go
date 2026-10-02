// Package codectest holds cross-codec contract tests.
package codectest

import (
	"errors"
	"testing"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/tabular"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toon"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toonx"
)

var seeds = []string{
	`[{"a":1,"b":"x"},{"a":2,"b":"y"}]`,
	`[{"id":9007199254740993,"neg":-0,"f":88.0,"big":1e400,"d":0.1000,"h":123456789012345678901234567890}]`,
	`[{"s":"comma, colon: quote\" backslash\\ newline\n tab\t"},{"s":"<html>&amp;"}]`,
	`[{"s":"123"},{"s":"true"},{"s":"null"},{"s":""},{"s":" padded "},{"s":"-dash"}]`,
	`[{"n":null,"b":true},{"n":3,"b":false}]`,
	`[{"nested":{"x":[1,2,3]},"k":"v"},{"nested":{"x":[4]},"k":"w"}]`,
	// Wide enough for toonx to split into parts.
	`[{"a":1,"b":2,"c":3,"d":4,"e":5,"u":{"f":6,"g":"https://x.io/u/1","h":{"i":7}},"j":8,"k":"z"},{"a":2,"b":3,"c":4,"d":5,"e":6,"u":{"f":7,"g":"https://x.io/u/2","h":{"i":8}},"j":9,"k":"y"},{"a":3,"b":4,"c":5,"d":6,"e":7,"u":{"f":8,"g":"https://x.io/u/3","h":{"i":9}},"j":10}]`,
	`[{"ctl":"\u0001"}]`,
	`[{"é":"ünïcode","日本":"語"}]`,
	`[{"a":1},{"b":2}]`,
	`[{},{}]`,
	`[]`,
	`{"a":1}`,
	`[{"a":1.5e3},{"a":-2.25}]`,
	`[{"a":[]}]`,
}

func codecs(t testing.TB) []codec.Codec {
	var out []codec.Codec
	for _, n := range codec.Names() {
		c, _ := codec.Get(n)
		out = append(out, c)
	}
	if len(out) < 2 {
		t.Fatal("expected toon and tabular to be registered")
	}
	return out
}

// checkRoundTrip asserts the codec contract for one input. It returns whether
// the input was eligible.
func checkRoundTrip(t testing.TB, c codec.Codec, input []byte) bool {
	v, err := canonical.Parse(input)
	if err != nil {
		return false
	}
	want, _ := canonical.Marshal(v)
	opts := codec.Options{Mode: codec.Strict}
	if err := c.Check(v, opts); err != nil {
		if !errors.Is(err, codec.ErrIneligible) {
			t.Fatalf("%s: Check returned non-ineligible error: %v", c.Name(), err)
		}
		if _, encErr := c.Encode(want, opts); encErr == nil {
			t.Fatalf("%s: Encode accepted input that Check rejected: %s", c.Name(), want)
		}
		return false
	}
	enc, err := c.Encode(want, opts)
	if ve, ok := c.(codec.ValueEncoder); ok {
		// The fast path the gates use must agree with Encode exactly.
		enc2, err2 := ve.EncodeValue(v, want, opts)
		if string(enc2) != string(enc) || (err == nil) != (err2 == nil) {
			t.Fatalf("%s: EncodeValue differs from Encode (%v vs %v)\n%s\n%s", c.Name(), err2, err, enc2, enc)
		}
	}
	if errors.Is(err, codec.ErrIneligible) {
		return false // Encode is authoritative; Check is a pre-filter.
	}
	if err != nil {
		t.Fatalf("%s: Encode failed on eligible input %s: %v", c.Name(), want, err)
	}
	got, err := c.Decode(enc)
	if err != nil {
		t.Fatalf("%s: Decode failed: %v\ninput: %s\nencoded:\n%s", c.Name(), err, want, enc)
	}
	if string(got) != string(want) {
		t.Fatalf("%s: round-trip mismatch\n in: %s\nout: %s\nencoded:\n%s", c.Name(), want, got, enc)
	}
	return true
}

func TestRoundTripSeeds(t *testing.T) {
	for _, c := range codecs(t) {
		for _, s := range seeds {
			checkRoundTrip(t, c, []byte(s))
		}
	}
}

func TestEligibility(t *testing.T) {
	cases := []struct {
		in            string
		tabular, toon bool
	}{
		{`[{"a":1,"b":"x"},{"a":2,"b":"y"}]`, true, true},
		{`[{"id":9007199254740993}]`, true, false},
		{`[{"f":88.0}]`, true, false},
		{`[{"big":1e400}]`, true, false},
		{`[{"z":-0}]`, true, false},
		{`[{"a":"k","nested":{"x":1}}]`, false, true},
		// toon-go bug: object list item whose first field is nested.
		{`[{"nested":{"x":1}}]`, false, false},
		// TOON writes rows with differing keys or kinds as a list.
		{`[{"a":1},{"b":2}]`, false, true},
		{`[{},{}]`, false, false},
		{`[]`, false, true},
		{`{"total":2,"items":[{"a":1},{"a":2}]}`, false, true},
		{`[{"a":1},{"a":"x"}]`, false, true},
		{`[{"a":null},{"a":"x"}]`, true, true},
		{`[{"ctl":"\u0001"}]`, true, false},
	}
	tab, _ := codec.Get("tabular")
	toon, _ := codec.Get("toon")
	for _, c := range cases {
		if got := checkRoundTrip(t, tab, []byte(c.in)); got != c.tabular {
			t.Errorf("tabular eligible(%s) = %v, want %v", c.in, got, c.tabular)
		}
		if got := checkRoundTrip(t, toon, []byte(c.in)); got != c.toon {
			t.Errorf("toon eligible(%s) = %v, want %v", c.in, got, c.toon)
		}
	}
}

// Byte-exact numeric golden test through the tabular codec.
func TestTabularNumericGolden(t *testing.T) {
	tab, _ := codec.Get("tabular")
	in := `[{"a":9007199254740993,"b":-0,"c":88.0,"d":1e400,"e":0.1000,"f":123456789012345678901234567890}]`
	enc, err := tab.Encode([]byte(in), codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	wantEnc := "[\"a\",\"b\",\"c\",\"d\",\"e\",\"f\"]\n[9007199254740993,-0,88.0,1e400,0.1000,123456789012345678901234567890]"
	if string(enc) != wantEnc {
		t.Fatalf("encoded:\n%s\nwant:\n%s", enc, wantEnc)
	}
	dec, err := tab.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec) != in {
		t.Fatalf("decoded %s, want %s", dec, in)
	}
}

func TestTabularDecodeRejects(t *testing.T) {
	tab, _ := codec.Get("tabular")
	for _, in := range []string{
		"[\"b\",\"a\"]\n[1,2]", // unsorted header
		"[\"a\",\"a\"]\n[1,2]", // duplicate header
		"[\"\"]\n[1]",          // empty header
		"[\"a\"]\n[1,2]",       // arity
		"[\"a\"]\n[[1]]",       // nested
		"[\"a\"]\n[1]\n",       // trailing newline = empty row
		"[\"a\"]\n[1] x",       // trailing garbage
		"[\"a\"]",              // no rows
		"[\"a\"]\n{\"a\":1}",   // row not an array
	} {
		if out, err := tab.Decode([]byte(in)); err == nil {
			t.Errorf("Decode(%q) accepted: %s", in, out)
		}
	}
	if out, err := tab.Decode([]byte("[\"a\"]\r\n[1]")); err != nil || string(out) != `[{"a":1}]` {
		t.Errorf("CRLF: %s %v", out, err)
	}
}

func TestUnionIsLossyAndFillsNull(t *testing.T) {
	tab, _ := codec.Get("tabular")
	if tab.Lossless(codec.Union) {
		t.Fatal("UNION must be declared lossy")
	}
	enc, err := tab.Encode([]byte(`[{"a":1},{"b":2}]`), codec.Options{Mode: codec.Union})
	if err != nil {
		t.Fatal(err)
	}
	if string(enc) != "[\"a\",\"b\"]\n[1,null]\n[null,2]" {
		t.Fatalf("got %q", enc)
	}
}

func FuzzRoundTrip(f *testing.F) {
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, c := range codecs(t) {
			checkRoundTrip(t, c, data)
		}
	})
}
