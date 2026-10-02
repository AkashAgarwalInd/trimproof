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
		// toon-go writes a table nested in a list item in a form its own
		// decoder rejects.
		{"nonuniform.json", "toon", false, "structural", "round trip failed"},
		{"nonuniform.json", "tabular", false, "structural", ""},
		{"wrapper.json", "toon", true, "none", ""},
		{"wrapper.json", "tabular", false, "structural", "top-level value is not an array"},
		{"scalars.json", "toon", false, "net-savings", "not smaller"},
		{"uniform.json", "toonx", true, "none", ""},
		{"nested.json", "toonx", true, "none", ""},
		// Encoded, but this small fixture lands just under the 15% gate.
		{"nonuniform.json", "toonx", false, "net-savings", "net savings 14.9%"},
		{"wrapper.json", "toonx", true, "none", ""},
		{"scalars.json", "toonx", false, "net-savings", "not smaller"},
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
		"| | toonx | toon | tabular |",
		"| payloads eligible | 2 / 3 (67%) | 2 / 3 (67%) | 1 / 3 (33%) |",
		"| fixture | 3 | 2 | 2 | 1 |",
		"| fixture | wrapper | 0.0 | 1306 | **",
		"| fixture | scalars | ",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("report lacks %q\n%s", want, s)
		}
	}
}
