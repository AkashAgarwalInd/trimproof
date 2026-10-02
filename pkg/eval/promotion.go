package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/context-mesh/context-mesh/pkg/policy"
	"github.com/context-mesh/context-mesh/pkg/server"
)

// Thresholds parameterize the promotion state machine (spec §8.4).
// Defaults should be revisited with Phase 0 data (spec §14).
type Thresholds struct {
	NMin        int     // minimum valid treatment pairs before promotion
	NControlMin int     // minimum valid control pairs (noise floor estimate)
	Epsilon     float64 // tolerated agreement shortfall vs the noise floor
	Alpha       float64 // significance level for McNemar / production tests
	Window      int     // rolling window (pairs) used for demotion while ENABLED
	ProdMin     int     // minimum transformed production requests before the production test
}

// DefaultThresholds are conservative starting values.
func DefaultThresholds() Thresholds {
	return Thresholds{NMin: 200, NControlMin: 50, Epsilon: 0.02, Alpha: 0.05, Window: 200, ProdMin: 50}
}

// Stats summarizes a set of pairs.
type Stats struct {
	NTreatment, NControl int
	AgreeTreatment       float64 // fraction of treatment pairs that agree
	AgreeControl         float64 // noise floor: fraction of control pairs that agree
	// Tier 1 discordance over treatment pairs: B = JSON passed and codec
	// failed; C = codec passed and JSON failed.
	B, C            int
	McNemarP        float64
	MeasuredSavings float64 // 1 − Σ codec input tokens / Σ JSON input tokens
}

// ComputeStats summarizes valid pairs, using at most the last window pairs
// of each kind (window <= 0 means all).
func ComputeStats(pairs []EvaluationPair, window int) Stats {
	var treat, ctrl []EvaluationPair
	for _, p := range pairs {
		if !p.Valid() {
			continue
		}
		if p.Kind == Control {
			ctrl = append(ctrl, p)
		} else {
			treat = append(treat, p)
		}
	}
	if window > 0 {
		treat, ctrl = tail(treat, window), tail(ctrl, window)
	}
	s := Stats{NTreatment: len(treat), NControl: len(ctrl)}
	s.AgreeTreatment = agreeRate(treat)
	s.AgreeControl = agreeRate(ctrl)
	var inA, inB int
	for _, p := range treat {
		inA += p.UsageA.InputTokens
		inB += p.UsageB.InputTokens
		if p.Tier1A != nil && p.Tier1B != nil {
			if *p.Tier1A && !*p.Tier1B {
				s.B++
			}
			if !*p.Tier1A && *p.Tier1B {
				s.C++
			}
		}
	}
	if inA > 0 {
		s.MeasuredSavings = 1 - float64(inB)/float64(inA)
	}
	s.McNemarP = McNemarExact(s.B, s.C)
	return s
}

func tail[T any](xs []T, n int) []T {
	if len(xs) > n {
		return xs[len(xs)-n:]
	}
	return xs
}

func agreeRate(ps []EvaluationPair) float64 {
	if len(ps) == 0 {
		return 0
	}
	n := 0
	for _, p := range ps {
		if p.Agreement.Agree {
			n++
		}
	}
	return float64(n) / float64(len(ps))
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

// NextState is the pure promotion rule. It returns the next state and a
// reason; next == cur means no change.
func NextState(cur policy.PromotionState, minNetSavings float64, all, recent Stats, t Thresholds) (policy.PromotionState, string) {
	switch cur {
	case policy.Shadow:
		switch {
		case all.NTreatment < t.NMin:
			return cur, fmt.Sprintf("collecting: %d/%d treatment pairs", all.NTreatment, t.NMin)
		case all.NControl < t.NControlMin:
			return cur, fmt.Sprintf("collecting: %d/%d control pairs", all.NControl, t.NControlMin)
		case all.AgreeTreatment < all.AgreeControl-t.Epsilon:
			return cur, fmt.Sprintf("agreement %.3f below noise floor %.3f − ε", all.AgreeTreatment, all.AgreeControl)
		case all.McNemarP <= t.Alpha && all.B > all.C:
			return cur, fmt.Sprintf("Tier 1 regression (McNemar p=%.4f, b=%d, c=%d)", all.McNemarP, all.B, all.C)
		case all.MeasuredSavings < minNetSavings:
			return cur, fmt.Sprintf("measured savings %.3f < %.3f", all.MeasuredSavings, minNetSavings)
		}
		return policy.Enabled, fmt.Sprintf("promoted: n=%d, agreement %.3f vs noise floor %.3f, McNemar p=%.3f, savings %.3f",
			all.NTreatment, all.AgreeTreatment, all.AgreeControl, all.McNemarP, all.MeasuredSavings)
	case policy.Enabled:
		if recent.NTreatment >= t.Window/2 && recent.NControl > 0 && recent.AgreeTreatment < recent.AgreeControl-t.Epsilon {
			return policy.Shadow, fmt.Sprintf("demoted: rolling agreement %.3f below noise floor %.3f − ε", recent.AgreeTreatment, recent.AgreeControl)
		}
		if recent.McNemarP <= t.Alpha && recent.B > recent.C {
			return policy.Shadow, fmt.Sprintf("demoted: Tier 1 regression in shadow pairs (McNemar p=%.4f)", recent.McNemarP)
		}
	}
	return cur, ""
}

// Promoter applies NextState to routes in a registry and watches
// production Tier 1 outcomes as a per-route representation circuit breaker.
type Promoter struct {
	Registry     *server.Registry
	Store        Store
	T            Thresholds
	OnTransition func(Transition)

	mu   sync.Mutex
	prod map[string]*prodCounts
}

type prodCounts struct {
	encN, encFail   int // transformed first attempts
	baseN, baseFail int // canonical JSON requests on the same route
}

// Evaluate re-checks one route after new evidence.
func (p *Promoter) Evaluate(tenant, route string) *Transition {
	rt := p.Registry.Lookup(tenant, route)
	cur := rt.Policy.State
	if cur != policy.Shadow && cur != policy.Enabled {
		return nil // OFF and MANUAL are operator-controlled
	}
	pairs := p.Store.Pairs(tenant, route)
	all, recent := ComputeStats(pairs, 0), ComputeStats(pairs, p.T.Window)
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
	if p.OnTransition != nil {
		p.OnTransition(*tr)
	}
	return tr
}

// Observe implements server.Observer: it tracks first-attempt Tier 1
// failures of transformed requests against the route's canonical-JSON
// baseline and demotes an ENABLED route at once on a significant excess
// (spec §8.4, demotion signals).
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
// win. It returns the number of routes restored.
func ReplayTransitions(path string, reg *server.Registry) (int, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
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
	n := 0
	for k, tr := range last {
		cur := reg.Lookup(k[0], k[1]).Policy
		if cur.TenantID != k[0] || cur.Version != tr.PolicyVersion {
			continue
		}
		if cur.State != policy.Shadow && cur.State != policy.Enabled {
			continue
		}
		if reg.SetState(k[0], k[1], tr.To) {
			n++
		}
	}
	return n, sc.Err()
}
