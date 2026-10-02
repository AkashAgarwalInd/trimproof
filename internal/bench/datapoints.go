package bench

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DatapointsReport writes every measured question: for each model, one row
// per question with each format's result side by side, discordant
// questions marked, then every failed call.
func DatapointsReport(w io.Writer, recs []Record) {
	fmt.Fprintln(w, "Each cell: ✓/✗ correct · input tokens · output tokens (reasoning) · latency · start of the reply. ⚠ marks questions where the arms disagree on correctness. Full replies are in the JSONL records.")
	fmt.Fprintln(w)
	for _, g := range groupRecords(recs) {
		var fms []string
		for _, fm := range append([]string{"json-compact"}, formatOrder...) {
			if slices.Contains(g.format, fm) {
				fms = append(fms, fm)
			}
		}
		fmt.Fprintf(w, "## %s\n\n", g.name)
		fmt.Fprintf(w, "| | question ID | data | kind | question | gold | %s |\n", strings.Join(fms, " | "))
		fmt.Fprintf(w, "|---|---|---|---|---|---|%s\n", strings.Repeat("---|", len(fms)))
		for _, q := range g.qs {
			arms := g.byQ[q]
			var first Record
			cells := make([]string, len(fms))
			seen := map[bool]bool{}
			for i, fm := range fms {
				r, ok := arms[fm]
				if !ok {
					cells[i] = "–"
					continue
				}
				first = r
				seen[r.Correct] = true
				mark := "✗"
				if r.Correct {
					mark = "✓"
				}
				rsn := ""
				if r.ReasoningTokens > 0 {
					rsn = fmt.Sprintf(" (%d)", r.ReasoningTokens)
				}
				cells[i] = fmt.Sprintf("%s · %d · %d%s · %.1fs · %s", mark, r.InputTokens, r.OutputTokens, rsn,
					float64(r.LatencyMS)/1000, cell(Clean(r.Reply), 40))
			}
			flag := ""
			if len(seen) > 1 {
				flag = "⚠"
			}
			data := fmt.Sprintf("%s/%d", first.Dataset, first.Rows)
			if first.Seed != 0 {
				data += fmt.Sprintf("/s%d", first.Seed)
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n", flag, q, data, first.Kind, cell(first.Question, 90),
				cell(first.Answer, 40), strings.Join(cells, " | "))
		}
		fmt.Fprintln(w)
		var failed []Record
		for _, r := range recs {
			if r.Error != "" && r.Provider+":"+r.Model == g.name {
				failed = append(failed, r)
			}
		}
		if len(failed) > 0 {
			fmt.Fprintf(w, "Failed calls (%d, excluded):\n\n| question ID | format | error |\n|---|---|---|\n", len(failed))
			for _, r := range failed {
				fmt.Fprintf(w, "| %s | %s | %s |\n", r.QuestionID, r.Format, cell(r.Error, 120))
			}
			fmt.Fprintln(w)
		}
	}
}

// cell makes s safe for a markdown table cell, cut to n runes.
func cell(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	if r := []rune(s); len(r) > n {
		s = string(r[:n]) + "…"
	}
	return s
}

// DumpData writes each dataset's tool result exactly as the JSON arm sent
// it (dir/<key>.json) and every question with its gold answer
// (dir/questions.json), so the benchmark's inputs can be audited.
func DumpData(dir string, ds []*Dataset) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	type q struct {
		Question
		Dataset string `json:"dataset"`
		File    string `json:"file"`
	}
	var qs []q
	for _, d := range ds {
		text, _, err := Render("json-compact", d)
		if err != nil {
			return err
		}
		file := d.Key() + ".json"
		if err := os.WriteFile(filepath.Join(dir, file), []byte(text+"\n"), 0o644); err != nil {
			return err
		}
		for _, x := range d.Questions {
			qs = append(qs, q{x, d.Key(), file})
		}
	}
	b, err := json.MarshalIndent(qs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "questions.json"), append(b, '\n'), 0o644)
}
