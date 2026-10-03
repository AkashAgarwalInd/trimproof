package bench

import (
	"fmt"
	"io"
	"math/rand/v2"
	"slices"
	"strings"
)

// VerifyConfig holds the pre-registered analysis settings (see
// bench/results/VERIFY-PLAN.md).
type VerifyConfig struct {
	Margin    float64   // non-inferiority margin on Δacc, e.g. 0.03
	Ks        []float64 // output price ratios for net savings
	Resamples int       // bootstrap resamples
	Seed      uint64    // bootstrap seed
	// MaxFailRate is the share of failed calls above which a model gets no
	// verdict.
	MaxFailRate float64
}

// DefaultVerify is the configuration VERIFY-PLAN.md registers.
func DefaultVerify() VerifyConfig {
	return VerifyConfig{Margin: 0.03, Ks: []float64{4, 1}, Resamples: 2000, Seed: 1, MaxFailRate: 0.10}
}

// pair is one question answered by the baseline (a) and a format (b).
type pair struct{ a, b Record }

// sums aggregates paired records; every verification metric is a function
// of these totals, so a bootstrap resample just re-adds them.
type sums struct {
	n, okA, okB, flips               int
	inA, inB, outA, outB, rsnA, rsnB float64
}

func (s *sums) add(p pair) {
	s.n++
	if p.a.Correct {
		s.okA++
	}
	if p.b.Correct {
		s.okB++
	}
	if p.a.Correct != p.b.Correct {
		s.flips++
	}
	s.inA += float64(p.a.InputTokens)
	s.inB += float64(p.b.InputTokens)
	s.outA += float64(p.a.OutputTokens)
	s.outB += float64(p.b.OutputTokens)
	s.rsnA += float64(p.a.ReasoningTokens)
	s.rsnB += float64(p.b.ReasoningTokens)
}

func (s sums) dAcc() float64     { return float64(s.okB-s.okA) / float64(s.n) }
func (s sums) inSaving() float64 { return 1 - s.inB/s.inA }
func (s sums) outChange() float64 {
	if s.outA == 0 {
		return 0
	}
	return s.outB/s.outA - 1
}
func (s sums) net(k float64) float64 { return 1 - (s.inB+k*s.outB)/(s.inA+k*s.outA) }

// ratioChange is b/a − 1, or 0 when a is 0.
func ratioChange(a, b float64) float64 {
	if a == 0 {
		return 0
	}
	return b/a - 1
}

// interval is a point estimate with a 95% percentile bootstrap interval.
type interval struct{ est, lo, hi float64 }

func (iv interval) pct() string {
	return fmt.Sprintf("%+.1f%% [%+.1f, %+.1f]", 100*iv.est, 100*iv.lo, 100*iv.hi)
}

func (iv interval) pp() string {
	return fmt.Sprintf("%+.1fpp [%+.1f, %+.1f]", 100*iv.est, 100*iv.lo, 100*iv.hi)
}

// bootstrap resamples pairs (questions) with replacement and returns a
// percentile interval for each metric.
func bootstrap(ps []pair, cfg VerifyConfig, metrics []func(sums) float64) []interval {
	var all sums
	for _, p := range ps {
		all.add(p)
	}
	out := make([]interval, len(metrics))
	draws := make([][]float64, len(metrics))
	r := rand.New(rand.NewPCG(cfg.Seed, 0))
	for i := 0; i < cfg.Resamples; i++ {
		var s sums
		for range ps {
			s.add(ps[r.IntN(len(ps))])
		}
		for m, f := range metrics {
			draws[m] = append(draws[m], f(s))
		}
	}
	for m, f := range metrics {
		slices.Sort(draws[m])
		q := func(p float64) float64 { return draws[m][int(p*float64(len(draws[m])-1)+0.5)] }
		out[m] = interval{est: f(all), lo: q(0.025), hi: q(0.975)}
	}
	return out
}

// verdict applies the registered claim rules to a Δacc interval.
func (cfg VerifyConfig) verdict(d interval) string {
	switch {
	case d.lo > -cfg.Margin:
		return fmt.Sprintf("non-inferior within %.0fpp", 100*cfg.Margin)
	case d.hi < -cfg.Margin:
		return "worse than JSON"
	}
	return "inconclusive"
}

// group is every record of one provider/model, keyed by question and format.
type group struct {
	name   string
	byQ    map[string]map[string]Record
	qs     []string // question IDs in first-seen order
	fails  map[string]int
	calls  map[string]int
	empty  map[string]int
	coded  map[string]int // gateway arm: replies the gateway served encoded
	format []string
}

func groupRecords(recs []Record) []*group {
	idx := map[string]*group{}
	var out []*group
	for _, r := range recs {
		name := r.Provider + ":" + r.Model
		g := idx[name]
		if g == nil {
			g = &group{name: name, byQ: map[string]map[string]Record{}, fails: map[string]int{},
				calls: map[string]int{}, empty: map[string]int{}, coded: map[string]int{}}
			idx[name] = g
			out = append(out, g)
		}
		if !slices.Contains(g.format, r.Format) {
			g.format = append(g.format, r.Format)
		}
		g.calls[r.Format]++
		if r.Error != "" {
			g.fails[r.Format]++
			continue
		}
		if r.Empty {
			g.empty[r.Format]++
		}
		if r.Representation != "" && !strings.HasPrefix(r.Representation, "json") {
			g.coded[r.Format]++
		}
		if g.byQ[r.QuestionID] == nil {
			g.byQ[r.QuestionID] = map[string]Record{}
			g.qs = append(g.qs, r.QuestionID)
		}
		// A resumed run may hold a failed and a later successful record;
		// keep the successful one.
		g.byQ[r.QuestionID][r.Format] = r
	}
	slices.SortFunc(out, func(a, b *group) int { return strings.Compare(a.name, b.name) })
	return out
}

// pairs returns the questions answered successfully by both formats.
func (g *group) pairs(a, b string) []pair {
	var ps []pair
	for _, q := range g.qs {
		ra, okA := g.byQ[q][a]
		rb, okB := g.byQ[q][b]
		if okA && okB {
			ps = append(ps, pair{ra, rb})
		}
	}
	return ps
}

// failRate is the share of failed calls among the group's calls.
func (g *group) failRate() float64 {
	calls, fails := 0, 0
	for f, n := range g.calls {
		calls += n
		fails += g.fails[f]
	}
	if calls == 0 {
		return 0
	}
	return float64(fails) / float64(calls)
}

// formatOrder lists the compared formats in report order.
var formatOrder = []string{"json-compact-2", "toonx", "toonx-p2", "toon", "tabular", "gateway", "csv"}

// VerifyReport writes the verification tables for recs: per model and
// format, accuracy with the registered verdict, input, output and net
// savings with 95% bootstrap intervals, and the noise floor.
func VerifyReport(w io.Writer, recs []Record, cfg VerifyConfig) {
	groups := groupRecords(recs)
	ks := cfg.Ks
	fmt.Fprintf(w, "Intervals are 95%% percentile bootstrap over questions (%d resamples, seed %d). ", cfg.Resamples, cfg.Seed)
	fmt.Fprintf(w, "Accuracy verdicts use a %.0fpp non-inferiority margin. Token figures are provider-reported usage, paired by question; only questions where both arms succeeded count.\n\n", 100*cfg.Margin)

	fmt.Fprintln(w, "### Accuracy")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| model | format | n | acc JSON | acc format | Δacc [95% CI] | answers that flip | verdict |")
	fmt.Fprintln(w, "|---|---|---:|---:|---:|---|---:|---|")
	for _, g := range groups {
		noVerdict := g.failRate() > cfg.MaxFailRate
		for _, fm := range formatOrder {
			ps := g.pairs("json-compact", fm)
			if len(ps) == 0 {
				continue
			}
			iv := bootstrap(ps, cfg, []func(sums) float64{sums.dAcc})[0]
			var s sums
			for _, p := range ps {
				s.add(p)
			}
			v := cfg.verdict(iv)
			switch {
			case fm == "json-compact-2":
				v = "noise floor"
			case noVerdict:
				v = fmt.Sprintf("no verdict: %.0f%% of calls failed", 100*g.failRate())
			}
			fmt.Fprintf(w, "| %s | %s | %d | %.1f%% | %.1f%% | %s | %.1f%% | %s |\n", g.name, fm, s.n,
				100*float64(s.okA)/float64(s.n), 100*float64(s.okB)/float64(s.n), iv.pp(), 100*float64(s.flips)/float64(s.n), v)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "### Tokens")
	fmt.Fprintln(w)
	head := "| model | format | n | input tokens | output tokens | reasoning tokens | answer tokens |"
	sep := "|---|---|---:|---|---|---:|---:|"
	for _, k := range ks {
		head += fmt.Sprintf(" net saving, output ×%g |", k)
		sep += "---|"
	}
	fmt.Fprintln(w, head+" latency ratio (median) |")
	fmt.Fprintln(w, sep+"---:|")
	for _, g := range groups {
		for _, fm := range formatOrder {
			if fm == "json-compact-2" {
				continue
			}
			ps := g.pairs("json-compact", fm)
			if len(ps) == 0 {
				continue
			}
			metrics := []func(sums) float64{
				func(s sums) float64 { return -s.inSaving() },
				sums.outChange,
			}
			for _, k := range ks {
				metrics = append(metrics, func(s sums) float64 { return s.net(k) })
			}
			ivs := bootstrap(ps, cfg, metrics)
			var s sums
			est := false
			lat := make([]float64, 0, len(ps))
			for _, p := range ps {
				s.add(p)
				est = est || p.a.ReasoningEstimated || p.b.ReasoningEstimated
				if p.a.LatencyMS > 0 {
					lat = append(lat, float64(p.b.LatencyMS)/float64(p.a.LatencyMS))
				}
			}
			rsn, ans := "–", "–"
			if s.rsnA+s.rsnB > 0 {
				rsn = fmt.Sprintf("%+.1f%%", 100*ratioChange(s.rsnA, s.rsnB))
				if est {
					rsn += " (est.)"
				}
				ans = fmt.Sprintf("%+.1f%%", 100*ratioChange(s.outA-s.rsnA, s.outB-s.rsnB))
			}
			row := fmt.Sprintf("| %s | %s | %d | %s | %s | %s | %s |", g.name, fm, s.n, ivs[0].pct(), ivs[1].pct(), rsn, ans)
			for i := range ks {
				row += " " + ivs[2+i].pct() + " |"
			}
			fmt.Fprintf(w, "%s ×%.2f |\n", row, median(lat))
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Input and output columns are the change against `json-compact` (negative input = saving). Net saving is the reduction in input + k·output tokens; k is the output/input price ratio.")
	fmt.Fprintln(w)

	multiTurn(w, groups, cfg)

	fmt.Fprintln(w, "### Input savings by data set")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| model | format | data set | rows | n | input tokens |")
	fmt.Fprintln(w, "|---|---|---|---:|---:|---|")
	for _, g := range groups {
		for _, fm := range []string{"toonx", "toonx-p2", "toon", "tabular", "gateway"} {
			by := map[string][]pair{}
			var keys []string
			for _, p := range g.pairs("json-compact", fm) {
				k := fmt.Sprintf("%s\x00%08d", p.a.Dataset, p.a.Rows)
				if by[k] == nil {
					keys = append(keys, k)
				}
				by[k] = append(by[k], p)
			}
			slices.Sort(keys)
			for _, k := range keys {
				ps := by[k]
				iv := bootstrap(ps, cfg, []func(sums) float64{func(s sums) float64 { return -s.inSaving() }})[0]
				fmt.Fprintf(w, "| %s | %s | %s | %d | %d | %s |\n", g.name, fm, ps[0].a.Dataset, ps[0].a.Rows, len(ps), iv.pct())
			}
		}
	}
	fmt.Fprintln(w)

	primerVariant(w, groups, cfg)

	fmt.Fprintln(w, "### Calls")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| model | format | calls | failed (excluded) | empty replies (scored incorrect) | served encoded by the gateway |")
	fmt.Fprintln(w, "|---|---|---:|---:|---:|---:|")
	for _, g := range groups {
		for _, fm := range g.format {
			coded := "–"
			if fm == "gateway" {
				coded = fmt.Sprint(g.coded[fm])
			}
			fmt.Fprintf(w, "| %s | %s | %d | %d | %d | %s |\n", g.name, fm, g.calls[fm], g.fails[fm], g.empty[fm], coded)
		}
	}
	fmt.Fprintln(w)
}

// multiTurn projects the net saving at k=4 when an agent resends the tool
// result on each of T turns while the output difference occurs once. The
// cached variant bills each resend at 0.1× input, as with prompt caching.
// It is computed from single-turn records, not measured.
func multiTurn(w io.Writer, groups []*group, cfg VerifyConfig) {
	const k = 4
	// Columns: turns T and the price of each resend relative to input.
	cols := []struct{ t, c float64 }{{1, 1}, {3, 1}, {10, 1}, {3, 0.1}, {10, 0.1}}
	fmt.Fprintln(w, "### Multi-turn projection (computed, not measured)")
	fmt.Fprintln(w)
	head, sep := "| model | format | n |", "|---|---|---:|"
	for _, col := range cols {
		label := fmt.Sprintf("%g turns", col.t)
		if col.t == 1 {
			label = "1 turn"
		} else if col.c != 1 {
			label += fmt.Sprintf(", resends at %g×", col.c)
		}
		head += " " + label + " |"
		sep += "---|"
	}
	fmt.Fprintln(w, head)
	fmt.Fprintln(w, sep)
	for _, g := range groups {
		for _, fm := range []string{"toonx", "gateway"} {
			ps := g.pairs("json-compact", fm)
			if len(ps) == 0 {
				continue
			}
			var metrics []func(sums) float64
			for _, col := range cols {
				m := 1 + col.c*(col.t-1)
				metrics = append(metrics, func(s sums) float64 { return 1 - (m*s.inB+k*s.outB)/(m*s.inA+k*s.outA) })
			}
			row := fmt.Sprintf("| %s | %s | %d |", g.name, fm, len(ps))
			for _, iv := range bootstrap(ps, cfg, metrics) {
				row += " " + iv.pct() + " |"
			}
			fmt.Fprintln(w, row)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Net saving with output ×%d when the same tool result is sent on each of T turns and the output difference is paid once. At 0.1×, every resend after the first is billed as cached input. These are projections from single-turn calls; no multi-turn conversation was run.\n\n", k)
}

// primerVariant compares the toonx-p2 arm with toonx directly and applies
// the registered adoption rule: cost at k=4 lower with the interval
// excluding 0 on at least 3 of 5 models, and no model's accuracy verdict
// against JSON worse than toonx's.
func primerVariant(w io.Writer, groups []*group, cfg VerifyConfig) {
	type row struct {
		name               string
		n                  int
		cost, out, dAcc    interval
		lower, accNotWorse bool
	}
	var rows []row
	rank := map[string]int{"worse than JSON": 0, "inconclusive": 1, fmt.Sprintf("non-inferior within %.0fpp", 100*cfg.Margin): 2}
	verdictOf := func(g *group, fm string) string {
		ps := g.pairs("json-compact", fm)
		if len(ps) == 0 {
			return ""
		}
		return cfg.verdict(bootstrap(ps, cfg, []func(sums) float64{sums.dAcc})[0])
	}
	for _, g := range groups {
		ps := g.pairs("toonx", "toonx-p2")
		if len(ps) == 0 {
			continue
		}
		iv := bootstrap(ps, cfg, []func(sums) float64{
			func(s sums) float64 { return -s.net(4) },
			sums.outChange,
			sums.dAcc,
		})
		r := row{name: g.name, n: len(ps), cost: iv[0], out: iv[1], dAcc: iv[2], lower: iv[0].hi < 0}
		r.accNotWorse = rank[verdictOf(g, "toonx-p2")] >= rank[verdictOf(g, "toonx")]
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(w, "### Primer variant: toonx-p2 against toonx")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| model | n | cost change at k=4 | output change | Δacc | cost lower (interval excludes 0) | accuracy verdict vs JSON not worse |")
	fmt.Fprintln(w, "|---|---:|---|---|---|---|---|")
	lower, worse := 0, 0
	for _, r := range rows {
		if r.lower {
			lower++
		}
		if !r.accNotWorse {
			worse++
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s | %s | %s | %s |\n", r.name, r.n, r.cost.pct(), r.out.pct(), r.dAcc.pp(),
			yesNo(r.lower), yesNo(r.accNotWorse))
	}
	met := lower >= 3 && worse == 0
	fmt.Fprintf(w, "\nAdoption rule (cost lower on at least 3 models, no accuracy verdict worse): cost lower on %d of %d, verdict worse on %d. Rule met: %s.\n\n",
		lower, len(rows), worse, yesNo(met))
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}
