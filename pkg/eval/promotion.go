package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/server"
)

// Thresholds parameterize the promotion state machine.
//
// Promotion is a paired non-inferiority test on three-arm samples. For each
// sample, d = mean(agree(A,B), agree(C,B)) − agree(A,C): the codec's
// agreement with JSON minus JSON's agreement with itself on the same
// request. The route is promoted when the lower bound of mean(d) is above
// −Epsilon, and rejected when the upper bound is below it. Decisions are
// taken only at scheduled looks, and Z is widened for repeated looks.
// bench/results/PROMOTION.md has the simulated error rates.
type Thresholds struct {
	NMin      int     // samples before the first look
	LookEvery int     // samples between looks
	Z         float64 // bound width in standard errors, valid across repeated looks
	Epsilon   float64 // tolerated agreement shortfall vs the noise floor
	Alpha     float64 // significance level for McNemar / production tests
	Window    int     // rolling window (samples) used for demotion while ENABLED
	// MaxSamples ends an inconclusive evaluation (the route goes OFF).
	MaxSamples int
	ProdMin    int // minimum transformed production requests before the production test
}

// DefaultThresholds keep wrong promotions at ε to about 5%
// (TestPromotionErrorRates).
func DefaultThresholds() Thresholds {
	return Thresholds{NMin: 200, LookEvery: 100, Z: 2.5, Epsilon: 0.02, Alpha: 0.05, Window: 400, MaxSamples: 4000, ProdMin: 50}
}

// Stats summarizes valid Paired samples.
type Stats struct {
	N              int
	AgreeTreatment float64 // mean agreement of the codec arm with the JSON arms
	AgreeControl   float64 // noise floor: agreement of the two JSON arms
	Diff           float64 // mean per-sample difference (treatment − control)
	SE             float64 // standard error of Diff
	// Tier 1 discordance between JSON arm A and the codec arm: B = JSON
	// passed and codec failed; C = codec passed and JSON failed.
	B, C     int
	McNemarP float64
	// MeasuredSavings is the provider-reported cost saving, with output
	// tokens weighted by the route's output price ratio:
	// 1 − (in_codec + k·out_codec) / (in_json + k·out_json).
	MeasuredSavings float64
	InputSavings    float64 // 1 − in_codec / in_json
	OutputChange    float64 // out_codec / out_json − 1
	LatencyRatio    float64 // median codec-arm latency / JSON-arm latency; 0 if unknown
}

// Bounds returns Diff ∓ z·SE.
func (s Stats) Bounds(z float64) (lo, hi float64) { return s.Diff - z*s.SE, s.Diff + z*s.SE }

// ComputeStats summarizes valid Paired samples, using at most the last
// window samples (window <= 0 means all). outputPriceRatio weights output
// tokens in MeasuredSavings.
func ComputeStats(samples []EvaluationPair, window int, outputPriceRatio float64) Stats {
	var ps []EvaluationPair
	for _, p := range samples {
		if p.Kind == Paired && p.Valid() && p.ControlAgreement != nil && p.AgreementCB != nil {
			ps = append(ps, p)
		}
	}
	if window > 0 {
		ps = tail(ps, window)
	}
	var acc accumulator
	for _, p := range ps {
		acc.add(p)
	}
	return acc.stats(outputPriceRatio)
}

// accumulator builds Stats incrementally from valid Paired samples.
type accumulator struct {
	n                       int
	sumT, sumC, sumD, sumD2 float64
	inJSON, inCodec         float64
	outJSON, outCodec       float64
	latency                 []float64
	b, c                    int
}

func (a *accumulator) add(p EvaluationPair) {
	t := (b2f(p.Agreement.Agree) + b2f(p.AgreementCB.Agree)) / 2
	c := b2f(p.ControlAgreement.Agree)
	a.n++
	a.sumT, a.sumC, a.sumD, a.sumD2 = a.sumT+t, a.sumC+c, a.sumD+t-c, a.sumD2+(t-c)*(t-c)
	a.inJSON += float64(p.UsageA.InputTokens+p.UsageC.InputTokens) / 2
	a.outJSON += float64(p.UsageA.OutputTokens+p.UsageC.OutputTokens) / 2
	a.inCodec += float64(p.UsageB.InputTokens)
	a.outCodec += float64(p.UsageB.OutputTokens)
	if p.LatencyA > 0 && p.LatencyB > 0 && p.LatencyC > 0 {
		a.latency = append(a.latency, float64(p.LatencyB)/(float64(p.LatencyA+p.LatencyC)/2))
	}
	if p.Tier1A != nil && p.Tier1B != nil {
		if *p.Tier1A && !*p.Tier1B {
			a.b++
		}
		if !*p.Tier1A && *p.Tier1B {
			a.c++
		}
	}
}

func (a *accumulator) stats(outputPriceRatio float64) Stats {
	s := Stats{N: a.n, B: a.b, C: a.c, McNemarP: McNemarExact(a.b, a.c)}
	if a.n == 0 {
		return s
	}
	n := float64(a.n)
	s.AgreeTreatment, s.AgreeControl, s.Diff = a.sumT/n, a.sumC/n, a.sumD/n
	// Floor the standard error at 1/n so a run where every arm agrees is
	// not treated as certain.
	s.SE = max(math.Sqrt(max(a.sumD2/n-s.Diff*s.Diff, 0)/n), 1/n)
	if a.inJSON > 0 {
		s.InputSavings = 1 - a.inCodec/a.inJSON
	}
	if a.outJSON > 0 {
		s.OutputChange = a.outCodec/a.outJSON - 1
	}
	if den := a.inJSON + outputPriceRatio*a.outJSON; den > 0 {
		s.MeasuredSavings = 1 - (a.inCodec+outputPriceRatio*a.outCodec)/den
	}
	if len(a.latency) > 0 {
		lat := slices.Sorted(slices.Values(a.latency))
		s.LatencyRatio = lat[len(lat)/2]
	}
	return s
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func tail[T any](xs []T, n int) []T {
	if len(xs) > n {
		return xs[len(xs)-n:]
	}
	return xs
}

// Transition is a state change with its evidence.
type Transition struct {
	Time     time.Time `json:"time"`
	TenantID string    `json:"tenant_id"`
	RouteID  string    `json:"route_id"`
	// PolicyVersion guards replay: a transition only restores state for the
	// same policy version.
	PolicyVersion string                `json:"policy_version"`
	From          policy.PromotionState `json:"from"`
	To            policy.PromotionState `json:"to"`
	Reason        string                `json:"reason"`
	Stats         Stats                 `json:"stats"`
}

// NextState is the pure promotion rule, applied at a look. all covers the
// samples since the route's last transition, recent the last Window of
// them. It returns the next state and a reason; next == cur means no change.
func NextState(cur policy.PromotionState, minNetSavings float64, all, recent Stats, t Thresholds) (policy.PromotionState, string) {
	switch cur {
	case policy.Shadow:
		lo, hi := all.Bounds(t.Z)
		switch {
		case all.N < t.NMin:
			return cur, fmt.Sprintf("collecting: %d/%d samples", all.N, t.NMin)
		case all.McNemarP <= t.Alpha && all.B > all.C:
			return cur, fmt.Sprintf("Tier 1 regression (McNemar p=%.4f, b=%d, c=%d)", all.McNemarP, all.B, all.C)
		case hi < -t.Epsilon:
			return policy.Off, fmt.Sprintf("rejected: n=%d, agreement %.3f vs noise floor %.3f (upper bound %+.3f < −ε)",
				all.N, all.AgreeTreatment, all.AgreeControl, hi)
		case lo > -t.Epsilon && all.MeasuredSavings >= minNetSavings:
			return policy.Enabled, fmt.Sprintf("promoted: n=%d, agreement %.3f vs noise floor %.3f (lower bound %+.3f > −ε), %s, McNemar p=%.3f",
				all.N, all.AgreeTreatment, all.AgreeControl, lo, savings(all), all.McNemarP)
		case lo > -t.Epsilon:
			return cur, fmt.Sprintf("measured savings %.3f < %.3f (%s)", all.MeasuredSavings, minNetSavings, savings(all))
		case t.MaxSamples > 0 && all.N >= t.MaxSamples:
			return policy.Off, fmt.Sprintf("inconclusive after %d samples: agreement %.3f vs noise floor %.3f, bounds [%+.3f, %+.3f]",
				all.N, all.AgreeTreatment, all.AgreeControl, lo, hi)
		}
		return cur, fmt.Sprintf("collecting: n=%d, bounds [%+.3f, %+.3f] straddle −ε", all.N, lo, hi)
	case policy.Enabled:
		if _, hi := recent.Bounds(t.Z); recent.N >= t.NMin && hi < -t.Epsilon {
			return policy.Shadow, fmt.Sprintf("demoted: rolling agreement %.3f vs noise floor %.3f (upper bound %+.3f < −ε)",
				recent.AgreeTreatment, recent.AgreeControl, hi)
		}
		if recent.McNemarP <= t.Alpha && recent.B > recent.C {
			return policy.Shadow, fmt.Sprintf("demoted: Tier 1 regression in shadow samples (McNemar p=%.4f)", recent.McNemarP)
		}
	}
	return cur, ""
}

func savings(s Stats) string {
	out := fmt.Sprintf("savings %.3f (input %.3f, output %+.3f)", s.MeasuredSavings, s.InputSavings, s.OutputChange)
	if s.LatencyRatio > 0 {
		out += fmt.Sprintf(", latency ×%.2f", s.LatencyRatio)
	}
	return out
}

// lookIndex numbers the scheduled looks: 0 before NMin samples, then 1 at
// NMin, 2 at NMin+LookEvery, and so on.
func (t Thresholds) lookIndex(n int) int {
	if n < t.NMin {
		return 0
	}
	return (n-t.NMin)/max(t.LookEvery, 1) + 1
}

// Promoter applies NextState to routes in a registry and watches
// production Tier 1 outcomes as a per-route representation circuit breaker.
type Promoter struct {
	Registry     *server.Registry
	Store        Store
	T            Thresholds
	OnTransition func(Transition)

	mu    sync.Mutex
	prod  map[string]*prodCounts
	looks map[string]int       // last look taken per route
	since map[string]time.Time // last transition per route; older samples are not evidence
}

type prodCounts struct {
	encN, encFail   int // transformed first attempts
	baseN, baseFail int // canonical JSON requests on the same route
}

// Resume restores per-route evidence windows from replayed transitions, so
// samples from before a route's last transition are not reused.
func (p *Promoter) Resume(trs []Transition) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.since == nil {
		p.since = map[string]time.Time{}
	}
	for _, tr := range trs {
		p.since[tr.TenantID+"\x00"+tr.RouteID] = tr.Time
	}
}

// Evaluate re-checks one route after new evidence. A decision is taken only
// when the route reaches its next scheduled look.
func (p *Promoter) Evaluate(tenant, route string) *Transition {
	rt := p.Registry.Lookup(tenant, route)
	cur := rt.Policy.State
	if cur != policy.Shadow && cur != policy.Enabled {
		return nil // OFF and MANUAL are operator-controlled
	}
	k := tenant + "\x00" + route
	p.mu.Lock()
	since := p.since[k]
	p.mu.Unlock()
	var samples []EvaluationPair
	for _, s := range p.Store.Pairs(tenant, route) {
		if since.IsZero() || s.Time.After(since) {
			samples = append(samples, s)
		}
	}
	ratio := rt.Policy.OutputPriceRatio
	all, recent := ComputeStats(samples, 0, ratio), ComputeStats(samples, p.T.Window, ratio)
	look := p.T.lookIndex(all.N)
	p.mu.Lock()
	if p.looks == nil {
		p.looks = map[string]int{}
	}
	if look <= p.looks[k] {
		p.mu.Unlock()
		return nil
	}
	p.looks[k] = look
	p.mu.Unlock()
	next, reason := NextState(cur, rt.Policy.MinNetSavings, all, recent, p.T)
	if next == cur {
		return nil
	}
	stats := all
	if cur == policy.Enabled {
		stats = recent
	}
	return p.transition(tenant, route, cur, next, reason, stats)
}

func (p *Promoter) transition(tenant, route string, from, to policy.PromotionState, reason string, s Stats) *Transition {
	if !p.Registry.SetState(tenant, route, to) {
		return nil
	}
	tr := &Transition{Time: time.Now(), TenantID: tenant, RouteID: route, From: from, To: to, Reason: reason, Stats: s,
		PolicyVersion: p.Registry.Lookup(tenant, route).Policy.Version}
	p.mu.Lock()
	if p.since == nil {
		p.since = map[string]time.Time{}
	}
	k := tenant + "\x00" + route
	p.since[k] = tr.Time
	delete(p.looks, k)
	p.mu.Unlock()
	if p.OnTransition != nil {
		p.OnTransition(*tr)
	}
	return tr
}

// Observe implements server.Observer: it tracks first-attempt Tier 1
// failures of transformed requests against the route's canonical-JSON
// baseline and demotes an ENABLED route at once on a significant excess.
func (p *Promoter) Observe(_ context.Context, x *server.Exchange) {
	if x.Route == nil || x.Tier1 == nil || x.Decision == nil {
		return
	}
	pol := x.Route.Policy
	k := pol.TenantID + "\x00" + pol.RouteID + "\x00" + x.Original.Model + "\x00" + x.Decision.Codec
	p.mu.Lock()
	if p.prod == nil {
		p.prod = map[string]*prodCounts{}
	}
	c := p.prod[k]
	if c == nil {
		c = &prodCounts{}
		p.prod[k] = c
	}
	encoded := x.FellBack || x.Sent == "encoded"
	if encoded {
		c.encN++
		if x.FellBack || !x.Tier1.OK {
			c.encFail++
		}
	} else {
		c.baseN++
		if !x.Tier1.OK {
			c.baseFail++
		}
	}
	snapshot := *c
	p.mu.Unlock()

	if !encoded || pol.State != policy.Enabled || snapshot.encN < p.T.ProdMin {
		return
	}
	// Baseline failure rate with a Laplace prior so an empty baseline is
	// not treated as zero.
	p0 := float64(snapshot.baseFail+1) / float64(snapshot.baseN+2)
	rate := float64(snapshot.encFail) / float64(snapshot.encN)
	pv := BinomialUpperTail(snapshot.encN, snapshot.encFail, p0)
	if rate > p0+p.T.Epsilon && pv < p.T.Alpha {
		reason := fmt.Sprintf("demoted: production Tier 1 failure rate %.3f vs baseline %.3f (n=%d, p=%.4g)", rate, p0, snapshot.encN, pv)
		if p.transition(pol.TenantID, pol.RouteID, policy.Enabled, policy.Shadow, reason, Stats{}) != nil {
			p.mu.Lock()
			p.prod[k] = &prodCounts{baseN: snapshot.baseN, baseFail: snapshot.baseFail}
			p.mu.Unlock()
		}
	}
}

// McNemarExact is the two-sided exact McNemar p-value for discordant counts.
func McNemarExact(b, c int) float64 {
	n := b + c
	if n == 0 {
		return 1
	}
	return math.Min(1, 2*binomCDF(n, min(b, c), 0.5))
}

// BinomialUpperTail returns P(X >= k) for X ~ Bin(n, p).
func BinomialUpperTail(n, k int, p float64) float64 {
	if k <= 0 {
		return 1
	}
	return 1 - binomCDF(n, k-1, p)
}

func binomCDF(n, k int, p float64) float64 {
	if p <= 0 {
		return 1
	}
	if p >= 1 {
		if k >= n {
			return 1
		}
		return 0
	}
	sum := 0.0
	lp, lq := math.Log(p), math.Log1p(-p)
	for i := 0; i <= k; i++ {
		sum += math.Exp(lchoose(n, i) + float64(i)*lp + float64(n-i)*lq)
	}
	return math.Min(1, sum)
}

func lchoose(n, k int) float64 {
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

// ReplayTransitions restores promotion state after a restart from a JSONL
// transition log. A route's last recorded state is applied only when the
// route's policy version is unchanged and the policy file leaves it under
// automatic control (SHADOW or ENABLED); OFF and MANUAL in the file always
// win. It returns the transitions it applied (see Promoter.Resume).
func ReplayTransitions(path string, reg *server.Registry) ([]Transition, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	last := map[[2]string]Transition{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var tr Transition
		if json.Unmarshal(sc.Bytes(), &tr) != nil {
			continue
		}
		last[[2]string{tr.TenantID, tr.RouteID}] = tr
	}
	var applied []Transition
	for k, tr := range last {
		cur := reg.Lookup(k[0], k[1]).Policy
		if cur.TenantID != k[0] || cur.Version != tr.PolicyVersion {
			continue
		}
		if cur.State != policy.Shadow && cur.State != policy.Enabled {
			continue
		}
		if reg.SetState(k[0], k[1], tr.To) {
			applied = append(applied, tr)
		}
	}
	return applied, sc.Err()
}
