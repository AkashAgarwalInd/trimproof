package bench

import (
	"fmt"
	"io"
	"math/rand/v2"
	"slices"

	"github.com/AkashAgarwalInd/trimproof/pkg/eval"
	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
)

// ReplayConfig holds the registered settings of the promotion replay.
type ReplayConfig struct {
	Codec            string  // the codec arm, e.g. "toonx"
	OutputPriceRatio float64 // k in the measured savings
	MinNetSavings    float64
	Thresholds       eval.Thresholds
	Seed             uint64 // order in which questions arrive as traffic
}

// DefaultReplay is the configuration Amendment 8 registers: the gateway's
// default thresholds and minimum savings, output priced at 4×.
func DefaultReplay() ReplayConfig {
	return ReplayConfig{Codec: "toonx", OutputPriceRatio: 4, MinNetSavings: 0.15, Thresholds: eval.DefaultThresholds(), Seed: 1}
}

// ReplayLook is the promotion test's state at one scheduled look.
type ReplayLook struct {
	Stats  eval.Stats
	State  policy.PromotionState
	Reason string
}

// replayPairs turns every question a model answered with json-compact (A),
// the codec (B) and json-compact-2 (C) into the gateway's three-arm shadow
// sample. Agreement is the gateway's own comparator on the replies, and
// Tier 1 is "scored correct". Questions are shuffled with a fixed seed, as
// they would arrive as traffic.
func replayPairs(g *group, cfg ReplayConfig) []eval.EvaluationPair {
	ids := slices.Sorted(slices.Values(g.qs))
	rand.New(rand.NewPCG(cfg.Seed, 0)).Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	var cmp eval.DefaultComparator
	resp := func(r Record) *ir.Response { return &ir.Response{Text: r.Reply} }
	usage := func(r Record) ir.Usage { return ir.Usage{InputTokens: r.InputTokens, OutputTokens: r.OutputTokens} }
	var out []eval.EvaluationPair
	for _, q := range ids {
		a, okA := g.byQ[q]["json-compact"]
		b, okB := g.byQ[q][cfg.Codec]
		c, okC := g.byQ[q]["json-compact-2"]
		if !okA || !okB || !okC {
			continue
		}
		ab, ac, cb := cmp.Compare(resp(a), resp(b)), cmp.Compare(resp(a), resp(c)), cmp.Compare(resp(c), resp(b))
		out = append(out, eval.EvaluationPair{ID: q, Kind: eval.Paired, Model: a.Model, Codec: cfg.Codec,
			Agreement: ab, ControlAgreement: &ac, AgreementCB: &cb,
			UsageA: usage(a), UsageB: usage(b), UsageC: usage(c),
			LatencyA: a.LatencyMS, LatencyB: b.LatencyMS, LatencyC: c.LatencyMS,
			Tier1A: &a.Correct, Tier1B: &b.Correct, Tier1C: &c.Correct})
	}
	return out
}

// replay runs the SHADOW route's promotion test over ps at each scheduled
// look, stopping at the first change of state. If no look is reached, the
// one entry holds every sample and the "collecting" reason.
func replay(ps []eval.EvaluationPair, cfg ReplayConfig) []ReplayLook {
	t := cfg.Thresholds
	var looks []ReplayLook
	for n := t.NextLook(0); n <= len(ps); n = t.NextLook(n) {
		s := eval.ComputeStats(ps[:n], 0, cfg.OutputPriceRatio)
		st, why := eval.NextState(policy.Shadow, cfg.MinNetSavings, s, s, t)
		looks = append(looks, ReplayLook{Stats: s, State: st, Reason: why})
		if st != policy.Shadow {
			return looks
		}
	}
	if len(looks) == 0 {
		s := eval.ComputeStats(ps, 0, cfg.OutputPriceRatio)
		_, why := eval.NextState(policy.Shadow, cfg.MinNetSavings, s, s, t)
		looks = append(looks, ReplayLook{Stats: s, State: policy.Shadow, Reason: why})
	}
	return looks
}

// PromotionReplay writes, per model, the decision the gateway's promotion
// test would take on a SHADOW route carrying recs as traffic.
func PromotionReplay(w io.Writer, recs []Record, cfg ReplayConfig) {
	t := cfg.Thresholds
	fmt.Fprintf(w, "Each question answered by json-compact, %s and json-compact-2 is one three-arm shadow sample. ", cfg.Codec)
	fmt.Fprintf(w, "Agreement uses the gateway's comparator on the replies; Tier 1 is \"scored correct\". ")
	fmt.Fprintf(w, "Thresholds: first look at %d samples, then every %d, z=%.1f, ε=%.2f; minimum net savings %.0f%% with output ×%g. ",
		t.NMin, t.LookEvery, t.Z, t.Epsilon, 100*cfg.MinNetSavings, cfg.OutputPriceRatio)
	fmt.Fprintf(w, "Questions arrive in a fixed shuffled order (seed %d).\n\n", cfg.Seed)
	fmt.Fprintln(w, "| model | samples | look at | codec agreement | noise floor | bounds of difference | Tier 1 b / c | input saved | net saved | state | reason |")
	fmt.Fprintln(w, "|---|---:|---:|---:|---:|---|---|---:|---:|---|---|")
	for _, g := range groupRecords(recs) {
		ps := replayPairs(g, cfg)
		if len(ps) == 0 {
			continue
		}
		for _, l := range replay(ps, cfg) {
			s := l.Stats
			lo, hi := s.Bounds(t.Z)
			fmt.Fprintf(w, "| %s | %d | %d | %.3f | %.3f | [%+.3f, %+.3f] | %d / %d | %.1f%% | %.1f%% | %s | %s |\n",
				g.name, len(ps), s.N, s.AgreeTreatment, s.AgreeControl, lo, hi, s.B, s.C,
				100*s.InputSavings, 100*s.MeasuredSavings, l.State, l.Reason)
		}
	}
	fmt.Fprintln(w)
}
