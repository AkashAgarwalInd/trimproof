package bench

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type fakeClient func(Call) string

func (f fakeClient) Do(_ context.Context, c Call) (*Result, error) {
	return &Result{Text: f(c), InputTokens: len(c.ToolResult), OutputTokens: 10}, nil
}

func freeFormFixture(t *testing.T) ([]*Dataset, []PayloadItem) {
	t.Helper()
	var ds []*Dataset
	var items []PayloadItem
	for i := range 4 {
		raw := fmt.Sprintf(`{"items":[{"id":1,"name":"a","meta":{"x":%d}},{"id":2,"name":"b"},{"id":3,"name":"c","meta":{"x":7}}]}`, i)
		ds = append(ds, &Dataset{ID: fmt.Sprintf("payload-api-p%d", i), Name: "payload", Rows: 3, Tool: "http_get",
			ToolArgs: `{"url":"u"}`, Raw: []byte(raw)})
		items = append(items, PayloadItem{Path: "items"})
	}
	return ds, items
}

func TestFreeFormDatasets(t *testing.T) {
	ds, items := freeFormFixture(t)
	a, ai := FreeFormDatasets(ds, items, 3000)
	b, _ := FreeFormDatasets(ds, items, 3000)
	kinds := map[string]bool{}
	for i, d := range a {
		q := d.Questions[0]
		if !q.FreeForm || len(d.Questions) != 1 || ai[i].Questions[0].ID != q.ID || b[i].Questions[0].Text != q.Text {
			t.Fatalf("task %d: %+v", i, q)
		}
		kinds[q.Kind] = true
	}
	if len(kinds) != len(FreeFormKinds) {
		t.Fatalf("kinds %v: want each of %v once", kinds, FreeFormKinds)
	}
	if len(ds[0].Questions) != 0 {
		t.Fatal("input datasets were changed")
	}
}

// TestFreeFormRunJudgeReport runs free-form tasks, judges them and reports,
// all against fake clients: toonx answers get an error on every task.
func TestFreeFormRunJudgeReport(t *testing.T) {
	ds, items := freeFormFixture(t)
	ds, _ = FreeFormDatasets(ds, items, 3000)
	dir := t.TempDir()
	out, judged := filepath.Join(dir, "a.jsonl"), filepath.Join(dir, "j.jsonl")
	model := fakeClient(func(c Call) string {
		if c.System != freeFormPrompt {
			t.Errorf("free-form call sent system prompt %q", c.System)
		}
		if strings.Contains(c.ToolResult, "items[3]") {
			return "toonx answer"
		}
		return "json answer"
	})
	arms := []string{"json-compact", "json-compact-2", "toonx"}
	if err := Run(context.Background(), RunConfig{Targets: []Target{{Provider: "nim", Model: "m", Client: model}},
		Datasets: ds, Formats: arms, Out: out, MaxCalls: 12, RPM: 60000, Concurrency: 2, MaxTokens: 100}); err != nil {
		t.Fatal(err)
	}
	recs, _ := ReadRecords(out)
	for _, r := range recs {
		if r.Correct || r.Error != "" {
			t.Fatalf("record %+v", r)
		}
	}
	answer := regexp.MustCompile(`Answer ([A-C]):\n<<<\n(\w+)`)
	judge := fakeClient(func(c Call) string {
		if c.System != judgePrompt || strings.Contains(c.ToolResult, "items[3]") {
			t.Errorf("judge did not get the data as JSON")
		}
		var parts []string
		for _, m := range answer.FindAllStringSubmatch(c.Question, -1) {
			errs := "[]"
			if m[2] == "toonx" {
				errs = `["wrong x"]`
			}
			parts = append(parts, fmt.Sprintf(`"%s":{"errors":%s,"complete":4}`, m[1], errs))
		}
		if len(parts) != 3 {
			t.Errorf("judge saw %d answers", len(parts))
		}
		return "<think>hmm</think>```json\n{" + strings.Join(parts, ",") + `,"same":{"AB":true,"AC":true,"BC":true}}` + "\n```"
	})
	cfg := JudgeConfig{Client: judge, Model: "j", Datasets: ds, Records: recs, Arms: arms, Out: judged, Seed: 3000,
		MaxCalls: 4, RPM: 60000, Concurrency: 2, MaxTokens: 100}
	if err := Judge(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := Judge(context.Background(), cfg); err != nil { // resumes: nothing left
		t.Fatal(err)
	}
	js, _ := ReadJudged(judged)
	if len(js) != 4 {
		t.Fatalf("%d judge records, want 4", len(js))
	}
	for _, j := range js {
		if j.Error != "" || len(j.Grades["toonx"].Errors) != 1 || len(j.Grades["json-compact"].Errors) != 0 || !j.Same["json-compact|toonx"] {
			t.Fatalf("judge record %+v", j)
		}
	}
	var buf bytes.Buffer
	FreeFormReport(&buf, recs, js, DefaultVerify())
	got := buf.String()
	for _, want := range []string{"| nim:m | 4 | 0% / 0% / 100% | +100.0pp", "a drop larger than the margin is not ruled out", "judge: 4 records, 0 not usable"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}

func TestParseJudgeRejects(t *testing.T) {
	rec := JudgeRecord{Labels: map[string]string{"json-compact": "A", "toonx": "B"}}
	for _, reply := range []string{
		"no json",
		`{"A":{"errors":[],"complete":5},"same":{"AB":true}}`,                                // B missing
		`{"A":{"errors":[],"complete":5},"B":{"errors":[],"complete":9},"same":{"AB":true}}`, // out of range
		`{"A":{"errors":[],"complete":5},"B":{"errors":[],"complete":5},"same":{}}`,          // no AB
	} {
		if err := parseJudge(&rec, reply); err == nil {
			t.Errorf("accepted %q", reply)
		}
	}
}

func TestWordF1(t *testing.T) {
	if f := wordF1("Top: react, 1.2M downloads.", "top react 1.2M downloads"); f != 1 {
		t.Fatalf("same words: %v", f)
	}
	if f := wordF1("a b", "c d"); f != 0 {
		t.Fatalf("no overlap: %v", f)
	}
}
