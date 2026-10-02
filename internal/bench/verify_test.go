package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSeeds(t *testing.T) {
	names, sizes := []string{"orders", "logs", "search", "employees"}, []int{30, 120}
	if id := Generate(names, sizes, PhaseZeroSeed)[0].Questions[0].ID; id != "orders-30-q1" {
		t.Fatalf("Phase 0 question ID changed: %s", id)
	}
	ds := GenerateSeeds(names, sizes, 1000, 5)
	ids := map[string]bool{}
	n := 0
	for _, d := range ds {
		for _, q := range d.Questions {
			if ids[q.ID] {
				t.Fatalf("duplicate question ID %s", q.ID)
			}
			ids[q.ID] = true
			n++
		}
	}
	if n != 230 {
		t.Fatalf("%d questions from 5 seeds, want 230", n)
	}
	if ds[0].Key() == ds[8].Key() || ds[0].Name != ds[8].Name || string(ds[0].JSON()) == string(ds[8].JSON()) {
		t.Fatal("seeds 1000 and 1001 should give different data under different keys")
	}
	got, kept := LimitQuestions(ds, 50), 0
	for _, d := range got {
		kept += len(d.Questions)
	}
	if kept != 50 || len(got) >= len(ds) || got[0].Questions[0].ID != ds[0].Questions[0].ID {
		t.Fatalf("LimitQuestions kept %d questions in %d datasets", kept, len(got))
	}
}

func TestRunRefusesOverCap(t *testing.T) {
	ds := Generate([]string{"orders"}, []int{30}, 1000)
	out := filepath.Join(t.TempDir(), "r.jsonl")
	called := false
	err := Run(context.Background(), RunConfig{
		Targets:  []Target{{Provider: "nim", Model: "m", Client: clientFunc(func(Call) { called = true })}},
		Datasets: ds, Formats: []string{"json-compact", "toon"}, Out: out, MaxCalls: 3, RPM: 6000, Concurrency: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "-max-calls 3") || called {
		t.Fatalf("err %v, called %v: want refusal before any call", err, called)
	}
}

type clientFunc func(Call)

func (f clientFunc) Do(_ context.Context, c Call) (*Result, error) {
	f(c)
	return &Result{Text: "x"}, nil
}

// fixture: 100 questions; JSON always right with 1000 in / 100 out tokens;
// toon wrong once with 750 in / 110 out; tabular wrong 20 times; the JSON
// resend wrong twice; one failed toon call.
func fixture() []Record {
	var recs []Record
	for i := 0; i < 100; i++ {
		base := Record{Provider: "nim", Model: "m", Dataset: "orders", Rows: 30, Kind: "count", QuestionID: fmt.Sprintf("q%d", i),
			Question: "how | many?", Answer: "7"}
		add := func(fm string, ok bool, in, out, rsn int) {
			r := base
			r.Format, r.Correct, r.InputTokens, r.OutputTokens, r.ReasoningTokens, r.LatencyMS = fm, ok, in, out, rsn, 1000
			r.Reply = "7"
			recs = append(recs, r)
		}
		add("json-compact", true, 1000, 100, 80)
		add("json-compact-2", i >= 2, 1000, 100, 80)
		add("toon", i != 5, 750, 110, 90)
		add("tabular", i >= 20, 700, 100, 80)
	}
	recs = append(recs, Record{Provider: "nim", Model: "m", QuestionID: "q0", Format: "toon", Error: "HTTP 500"})
	return recs
}

func TestVerifyReport(t *testing.T) {
	var buf bytes.Buffer
	VerifyReport(&buf, fixture(), DefaultVerify())
	out := buf.String()
	for _, want := range []string{
		"| nim:m | json-compact-2 | 100 | 100.0% | 98.0% |",
		"| 2.0% | noise floor |",
		"| nim:m | toon | 100 | 100.0% | 99.0% | -1.0pp [-3.0, +0.0] | 1.0% | inconclusive |",
		"| nim:m | tabular | 100 | 100.0% | 80.0% | -20.0pp [",
		"| worse than JSON |",
		// input −25%, output +10%, reasoning +12.5%, answer 20→20, net at
		// k=4: 1 − (750+440)/(1000+400) = 15%; at k=1: 1 − 860/1100.
		"| nim:m | toon | 100 | -25.0% [-25.0, -25.0] | +10.0% [+10.0, +10.0] | +12.5% | +0.0% | +15.0% [+15.0, +15.0] | +21.8% [+21.8, +21.8] | ×1.00 |",
		"| nim:m | toon | orders | 30 | 100 | -25.0% [-25.0, -25.0] |",
		"| nim:m | toon | 101 | 1 | 0 | – |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q\n%s", want, out)
		}
	}
	// Deterministic: the same records give the same report.
	var again bytes.Buffer
	VerifyReport(&again, fixture(), DefaultVerify())
	if again.String() != out {
		t.Fatal("report is not deterministic")
	}
}

func TestVerdict(t *testing.T) {
	cfg := DefaultVerify()
	for _, c := range []struct {
		lo, hi float64
		want   string
	}{
		{-0.02, 0.01, "non-inferior within 3pp"},
		{-0.08, -0.04, "worse than JSON"},
		{-0.05, 0.01, "inconclusive"},
	} {
		if got := cfg.verdict(interval{lo: c.lo, hi: c.hi}); got != c.want {
			t.Errorf("verdict(%v, %v) = %q, want %q", c.lo, c.hi, got, c.want)
		}
	}
}

func TestDatapointsReport(t *testing.T) {
	var buf bytes.Buffer
	DatapointsReport(&buf, fixture())
	out := buf.String()
	for _, want := range []string{
		"| ⚠ | q5 | orders/30 | count | how \\| many? | 7 |",
		"✓ · 1000 · 100 (80) · 1.0s · 7",
		"| q0 | toon | HTTP 500 |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("datapoints lack %q", want)
		}
	}
	if strings.Contains(out, "| ⚠ | q50 |") {
		t.Fatal("agreeing question marked discordant")
	}
}

func TestDumpData(t *testing.T) {
	dir := t.TempDir()
	ds := Generate([]string{"orders"}, []int{30}, 1000)
	if err := DumpData(dir, ds); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ds[0].Key()+".json"))
	if err != nil {
		t.Fatal(err)
	}
	text, _, _ := Render("json-compact", ds[0])
	if string(b) != text+"\n" {
		t.Fatal("dumped data differs from what the JSON arm sends")
	}
	var qs []map[string]any
	b, _ = os.ReadFile(filepath.Join(dir, "questions.json"))
	if err := json.Unmarshal(b, &qs); err != nil || len(qs) != len(ds[0].Questions) || qs[0]["answer"] == "" {
		t.Fatalf("questions.json: %v, %d entries", err, len(qs))
	}
}
