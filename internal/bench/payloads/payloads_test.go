package payloads

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEvaluate(t *testing.T) {
	est := tokens.NewCalibrated(nil)
	cases := []struct {
		file, codec string
		eligible    bool
		gate        string // "" = not checked
		reason      string // substring
	}{
		{"uniform.json", "toon", true, "none", ""},
		{"uniform.json", "tabular", true, "none", ""},
		{"nested.json", "tabular", false, "structural", "nested container"},
		{"nonuniform.json", "toon", false, "structural", "missing key"},
		{"nonuniform.json", "tabular", false, "structural", ""},
		{"wrapper.json", "toon", false, "structural", "top-level value is not an array"},
		{"wrapper.json", "tabular", false, "structural", "top-level value is not an array"},
		{"scalars.json", "toon", false, "structural", "row 0 is not an object"},
	}
	for _, c := range cases {
		o, err := Evaluate(fixture(t, c.file), c.codec, est)
		if err != nil {
			t.Fatalf("%s/%s: %v", c.file, c.codec, err)
		}
		if o.Eligible != c.eligible || o.Gate != c.gate || !strings.Contains(o.Reason, c.reason) {
			t.Errorf("%s/%s: eligible=%v gate=%s reason=%q, want %v %s %q", c.file, c.codec, o.Eligible, o.Gate, o.Reason, c.eligible, c.gate, c.reason)
		}
		if o.Eligible && (o.EncTokens >= o.JSONTokens || o.Savings < 0.15 || o.PrettyTokens <= o.JSONTokens) {
			t.Errorf("%s/%s: implausible counts %+v", c.file, c.codec, o)
		}
		if !o.Eligible && o.Gate == "structural" && !math.IsNaN(o.Savings) {
			t.Errorf("%s/%s: savings %v for a structural rejection", c.file, c.codec, o.Savings)
		}
	}
	// TOON allows nested objects, so nesting alone is not a structural
	// rejection for it.
	if o, _ := Evaluate(fixture(t, "nested.json"), "toon", est); o.Gate == "structural" {
		t.Errorf("nested/toon rejected structurally: %s", o.Reason)
	}
}

func TestInnerArray(t *testing.T) {
	in, ok := InnerArray(fixture(t, "wrapper.json"))
	if !ok || in.Path != "items" || in.Rows != 30 {
		t.Fatalf("got %+v %v", in, ok)
	}
	// The unwrapped array is judged like a direct array.
	o, err := Evaluate(in.JSON, "toon", tokens.NewCalibrated(nil))
	if err != nil || !o.Eligible {
		t.Fatalf("inner array not eligible: %+v %v", o, err)
	}
	for _, f := range []string{"uniform.json", "scalars.json"} {
		if _, ok := InnerArray(fixture(t, f)); ok {
			t.Errorf("%s: top-level array reported as wrapper", f)
		}
	}
	// Deeper and larger arrays win; arrays of scalars are skipped.
	in, ok = InnerArray([]byte(`{"ids":[1,2,3,4,5,6,7,8,9],"message":{"items":[{"a":1},{"a":2}]},"x":[{"b":1}]}`))
	if !ok || in.Path != "message.items" || in.Rows != 2 {
		t.Fatalf("got %+v %v", in, ok)
	}
	// Keys containing dots are quoted so the path stays unambiguous.
	in, ok = InnerArray([]byte(`{"releases":{"2.23.0":[{"a":1}]}}`))
	if !ok || in.Path != `releases["2.23.0"]` {
		t.Fatalf("got %q %v", in.Path, ok)
	}
}

func TestAnalyze(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{FetchedAt: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)}
	for _, f := range []string{"uniform.json", "wrapper.json", "scalars.json"} {
		if err := os.WriteFile(filepath.Join(dir, f), fixture(t, f), 0o644); err != nil {
			t.Fatal(err)
		}
		m.Entries = append(m.Entries, Entry{API: "fixture", Name: strings.TrimSuffix(f, ".json"), File: f, Status: 200})
	}
	m.Entries = append(m.Entries, Entry{API: "fixture", Name: "down", Error: "HTTP 503"})
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Analyze(dir, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		"4 public, keyless endpoints; 3 stored, 1 failed: fixture/down (HTTP 503)",
		"| payloads eligible as sent | 1 / 3 (33%) | 1 / 3 (33%) |",
		"| eligible as sent or via inner array | 2 / 3 (67%) | 2 / 3 (67%) |",
		"| fixture | wrapper | `items` | 30 | **yes** |",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("report lacks %q\n%s", want, s)
		}
	}
}
