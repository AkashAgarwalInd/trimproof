package bench

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
)

// replayRecords makes n questions for one model. The JSON arms always give
// the gold answer; the codec arm uses 40% fewer input tokens and, on every
// wrongEvery-th question (0: never), gives a wrong answer, or the right
// one written differently when reworded is set.
func replayRecords(model string, n, wrongEvery int, reworded bool) []Record {
	var recs []Record
	for i := range n {
		q := fmt.Sprintf("q%03d", i)
		for _, f := range []string{"json-compact", "json-compact-2", "toonx"} {
			r := Record{Provider: "nim", Model: model, QuestionID: q, Format: f, Answer: "7", Reply: "7", Correct: true,
				InputTokens: 1000, OutputTokens: 100}
			if f == "toonx" {
				r.InputTokens = 600
				if wrongEvery > 0 && i%wrongEvery == 0 {
					r.Reply, r.Correct = "8", false
					if reworded {
						r.Reply, r.Correct = "7.0", true
					}
				}
			}
			recs = append(recs, r)
		}
	}
	return recs
}

func TestReplayDecisions(t *testing.T) {
	cfg := DefaultReplay()
	for _, tc := range []struct {
		model             string
		n, wrongEvery     int
		reworded          bool
		want              policy.PromotionState
		wantLooks, wantAt int
	}{
		{"same", 250, 0, false, policy.Enabled, 1, 200},  // identical answers, 32% cheaper at k=4
		{"differs", 400, 5, true, policy.Off, 1, 200},    // 20% of codec answers differ, all correct
		{"wrong", 400, 5, false, policy.Off, 1, 200},     // 20% wrong
		{"few", 150, 0, false, policy.Shadow, 1, 150},    // no look before 200 samples
		{"close", 450, 25, false, policy.Shadow, 3, 400}, // 4% wrong: held
	} {
		g := groupRecords(replayRecords(tc.model, tc.n, tc.wrongEvery, tc.reworded))[0]
		ps := replayPairs(g, cfg)
		if len(ps) != tc.n {
			t.Fatalf("%s: %d samples, want %d", tc.model, len(ps), tc.n)
		}
		looks := replay(ps, cfg)
		last := looks[len(looks)-1]
		if last.State != tc.want || len(looks) != tc.wantLooks || last.Stats.N != tc.wantAt {
			t.Errorf("%s: %d looks, last at %d: %s (%s); want %d looks, last at %d: %s",
				tc.model, len(looks), last.Stats.N, last.State, last.Reason, tc.wantLooks, tc.wantAt, tc.want)
		}
	}
}

// A question missing one of the three arms (a failed call) is no sample.
func TestReplaySkipsIncomplete(t *testing.T) {
	recs := replayRecords("m", 3, 0, false)
	recs[4].Error = "timeout" // q001 json-compact-2
	ps := replayPairs(groupRecords(recs)[0], DefaultReplay())
	if len(ps) != 2 {
		t.Fatalf("%d samples, want 2", len(ps))
	}
	var buf bytes.Buffer
	PromotionReplay(&buf, recs, DefaultReplay())
	if !strings.Contains(buf.String(), "| nim:m | 2 | 2 |") || !strings.Contains(buf.String(), "collecting: 2/200") {
		t.Fatalf("report:\n%s", buf.String())
	}
}
