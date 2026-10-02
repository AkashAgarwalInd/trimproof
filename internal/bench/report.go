package bench

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/context-mesh/context-mesh/pkg/eval"
	"github.com/context-mesh/context-mesh/pkg/tokens"
)

// TokenReport writes the offline token table: o200k_base counts per format,
// with the primer included for formats that need one.
func TokenReport(w io.Writer, datasets []*Dataset) error {
	var bpe tokens.BPE
	fmt.Fprintln(w, "## Offline token counts (o200k_base, data block + primer)")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "| dataset | rows | %s | toon vs compact | tabular vs compact | csv vs compact | toon vs pretty |\n", strings.Join(Formats, " | "))
	fmt.Fprintf(w, "|---|---:|%s---:|---:|---:|---:|\n", strings.Repeat("---:|", len(Formats)))
	totals := map[string]int{}
	for _, d := range datasets {
		counts := map[string]int{}
		for _, fm := range Formats {
			text, primer, err := Render(fm, d)
			if err != nil {
				return err
			}
			counts[fm] = bpe.Count(text)
			if primer != "" {
				counts[fm] += bpe.Count(primer)
			}
			totals[fm] += counts[fm]
		}
		cells := make([]string, len(Formats))
		for i, fm := range Formats {
			cells[i] = fmt.Sprint(counts[fm])
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s | %s | %s | %s |\n", d.Name, d.Rows, strings.Join(cells, " | "),
			pct(counts["json-compact"], counts["toon"]), pct(counts["json-compact"], counts["tabular"]),
			pct(counts["json-compact"], counts["csv"]), pct(counts["json-pretty"], counts["toon"]))
	}
	cells := make([]string, len(Formats))
	for i, fm := range Formats {
		cells[i] = fmt.Sprint(totals[fm])
	}
	fmt.Fprintf(w, "| **all** | | %s | **%s** | **%s** | **%s** | %s |\n", strings.Join(cells, " | "),
		pct(totals["json-compact"], totals["toon"]), pct(totals["json-compact"], totals["tabular"]),
		pct(totals["json-compact"], totals["csv"]), pct(totals["json-pretty"], totals["toon"]))
	fmt.Fprintln(w)
	return nil
}

// pct formats the reduction from base to x as a negative percentage.
func pct(base, x int) string {
	if base == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%+.1f%%", 100*float64(x-base)/float64(base))
}

// GoCriteria are the Phase 0 thresholds (spec §12).
type GoCriteria struct {
	MinReduction float64 // e.g. 0.20 on the data-heavy prompt
	Alpha        float64 // McNemar significance level
}

// LiveReport writes accuracy, token reduction, noise floor and a go/no-go
// verdict per model and format.
func LiveReport(w io.Writer, recs []Record, gc GoCriteria) {
	type pairKey struct{ model, qid string }
	byKey := map[pairKey]map[string]Record{}
	var models []string
	for _, r := range recs {
		if r.Error != "" {
			continue
		}
		k := pairKey{r.Model, r.QuestionID}
		if byKey[k] == nil {
			byKey[k] = map[string]Record{}
		}
		byKey[k][r.Format] = r
		if !slices.Contains(models, r.Model) {
			models = append(models, r.Model)
		}
	}
	slices.Sort(models)

	fmt.Fprintln(w, "## Live results (provider-reported input tokens, exact-answer accuracy)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Each row compares a format with `json-compact` on the same questions (paired). `json-compact-2` is the control arm: identical input sent twice, so its disagreement rate is the model's own noise floor. Discordant = questions where exactly one arm was correct (b = only JSON right, c = only format right). McNemar uses the exact binomial test on b+c.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| model | format | n | acc JSON | acc format | Δ acc | b | c | McNemar p | input tokens vs JSON | verdict |")
	fmt.Fprintln(w, "|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---|")
	for _, m := range models {
		noise := 0.0
		for _, fm := range []string{"json-compact-2", "toon", "tabular", "csv"} {
			n, accA, accB, b, c := 0, 0, 0, 0, 0
			tokA, tokB := 0, 0
			for k, arms := range byKey {
				if k.model != m {
					continue
				}
				a, okA := arms["json-compact"]
				x, okB := arms[fm]
				if !okA || !okB {
					continue
				}
				n++
				tokA += a.InputTokens
				tokB += x.InputTokens
				if a.Correct {
					accA++
				}
				if x.Correct {
					accB++
				}
				if a.Correct && !x.Correct {
					b++
				}
				if !a.Correct && x.Correct {
					c++
				}
			}
			if n == 0 {
				continue
			}
			p := eval.McNemarExact(b, c)
			fa, fb := float64(accA)/float64(n), float64(accB)/float64(n)
			red := 1 - float64(tokB)/float64(tokA)
			verdict := ""
			if fm == "json-compact-2" {
				noise = float64(b+c) / float64(n)
				verdict = fmt.Sprintf("noise floor: %.1f%% of answers flip", 100*noise)
			} else {
				switch {
				case red < gc.MinReduction:
					verdict = fmt.Sprintf("NO-GO: reduction < %.0f%%", 100*gc.MinReduction)
				case p <= gc.Alpha && b > c:
					verdict = "NO-GO: significant accuracy loss"
				case fa-fb > noise+1e-9 && b > c:
					verdict = "HOLD: accuracy drop exceeds noise floor"
				default:
					verdict = "GO"
				}
			}
			fmt.Fprintf(w, "| %s | %s | %d | %.1f%% | %.1f%% | %+.1fpp | %d | %d | %.3f | %+.1f%% | %s |\n",
				m, fm, n, 100*fa, 100*fb, 100*(fb-fa), b, c, p, -100*red, verdict)
		}
	}
	fmt.Fprintln(w)

	// Accuracy by question kind, pooled over models.
	fmt.Fprintln(w, "### Accuracy by question kind (all models)")
	fmt.Fprintln(w)
	kinds := []string{}
	acc := map[string]map[string][2]int{}
	for _, r := range recs {
		if r.Error != "" {
			continue
		}
		if !slices.Contains(kinds, r.Kind) {
			kinds = append(kinds, r.Kind)
		}
		if acc[r.Kind] == nil {
			acc[r.Kind] = map[string][2]int{}
		}
		v := acc[r.Kind][r.Format]
		v[1]++
		if r.Correct {
			v[0]++
		}
		acc[r.Kind][r.Format] = v
	}
	slices.Sort(kinds)
	fmt.Fprintf(w, "| kind | %s |\n|---|%s\n", strings.Join(LiveFormats, " | "), strings.Repeat("---:|", len(LiveFormats)))
	for _, k := range kinds {
		cells := make([]string, len(LiveFormats))
		for i, fm := range LiveFormats {
			v := acc[k][fm]
			if v[1] == 0 {
				cells[i] = "–"
				continue
			}
			cells[i] = fmt.Sprintf("%.0f%% (%d/%d)", 100*float64(v[0])/float64(v[1]), v[0], v[1])
		}
		fmt.Fprintf(w, "| %s | %s |\n", k, strings.Join(cells, " | "))
	}
	fmt.Fprintln(w)

	errs, empties := 0, map[string]int{}
	for _, r := range recs {
		if r.Error != "" {
			errs++
		}
		if r.Empty {
			empties[r.Format]++
		}
	}
	if len(empties) > 0 {
		fmt.Fprint(w, "_Empty replies after 3 attempts (scored incorrect):")
		for _, fm := range LiveFormats {
			fmt.Fprintf(w, " %s=%d", fm, empties[fm])
		}
		fmt.Fprint(w, "_\n\n")
	}
	if errs > 0 {
		fmt.Fprintf(w, "_%d calls failed and are excluded (see results JSONL)._\n\n", errs)
	}
}
