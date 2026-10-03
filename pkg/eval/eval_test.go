package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/tabular"
	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/server"
	"github.com/AkashAgarwalInd/trimproof/pkg/validator"
)

func resp(text string, calls ...ir.ToolCall) *ir.Response {
	return &ir.Response{Text: text, ToolCalls: calls}
}

func tc(name, args string) ir.ToolCall { return ir.ToolCall{Name: name, Args: json.RawMessage(args)} }

func TestComparator(t *testing.T) {
	c := DefaultComparator{}
	a := resp("", tc("refund", `{"order_id":3,"amount":"5"}`), tc("notify", `{}`))
	b := resp("", tc("notify", ` { } `), tc("refund", `{ "amount":"5", "order_id":3 }`))
	if ag := c.Compare(a, b); !ag.Agree || !ag.ToolCallsEqual || !ag.ExactMatch || ag.FieldAgreement != 1 {
		t.Fatalf("order/whitespace should not matter: %+v", ag)
	}
	d := resp("", tc("refund", `{"order_id":4,"amount":"5"}`), tc("notify", `{}`))
	ag := c.Compare(a, d)
	if ag.Agree || ag.ToolCallsEqual {
		t.Fatalf("different args must disagree: %+v", ag)
	}
	if math.Abs(ag.FieldAgreement-1.0/3) > 1e-9 { // shared: amount; differing: order_id x2
		t.Fatalf("field agreement %v", ag.FieldAgreement)
	}
	if ag := c.Compare(resp("Eight "), resp("eight")); !ag.Agree {
		t.Fatal("text should be normalized")
	}
	s1 := &ir.Response{Structured: json.RawMessage(`{"a":1,"b":[1,2]}`), Text: `{"a":1,"b":[1,2]}`}
	s2 := &ir.Response{Structured: json.RawMessage(`{"b":[1,2],"a":1}`), Text: `{"b":[1,2],"a":1}`}
	if ag := c.Compare(s1, s2); !ag.Agree {
		t.Fatal("structured outputs compare canonically")
	}
}

// sample builds a valid Paired sample. ab, cb: the codec arm agrees with
// JSON arms A and C; ac: the JSON arms agree with each other.
func sample(ab, cb, ac bool, inJSON, inCodec, outJSON, outCodec int) EvaluationPair {
	ctl, agCB := Agreement{Agree: ac}, Agreement{Agree: cb}
	return EvaluationPair{Kind: Paired, Agreement: Agreement{Agree: ab}, ControlAgreement: &ctl, AgreementCB: &agCB,
		UsageA: ir.Usage{InputTokens: inJSON, OutputTokens: outJSON}, UsageC: ir.Usage{InputTokens: inJSON, OutputTokens: outJSON},
		UsageB: ir.Usage{InputTokens: inCodec, OutputTokens: outCodec}}
}

// samples returns n samples where the first worse have the codec arm
// disagreeing with both JSON arms and the rest all agree.
func samples(n, worse int, inCodec, outCodec int) []EvaluationPair {
	out := make([]EvaluationPair, n)
	for i := range out {
		ok := i >= worse
		out[i] = sample(ok, ok, true, 1000, inCodec, 100, outCodec)
	}
	return out
}

func withTier1(ps []EvaluationPair, t1 [][2]bool) []EvaluationPair {
	for i := range t1 {
		a, b := t1[i][0], t1[i][1]
		ps[i].Tier1A, ps[i].Tier1B = &a, &b
	}
	return ps
}

func TestComputeStats(t *testing.T) {
	ps := []EvaluationPair{
		sample(true, true, true, 1000, 700, 100, 130),
		sample(false, true, true, 1000, 700, 100, 130),
		sample(true, true, false, 1000, 700, 100, 130),
		{Kind: Treatment, Agreement: Agreement{Agree: false}}, // legacy layout: ignored
		{Kind: Paired, ErrC: "upstream HTTP 500"},             // invalid: ignored
	}
	ps[0].LatencyA, ps[0].LatencyB, ps[0].LatencyC = 1000, 1500, 1000
	st := ComputeStats(ps, 0, 4)
	if st.N != 3 {
		t.Fatalf("N=%d", st.N)
	}
	// d per sample: 0, −0.5, +1.
	if math.Abs(st.Diff-0.5/3) > 1e-12 || math.Abs(st.AgreeTreatment-2.5/3) > 1e-12 || math.Abs(st.AgreeControl-2.0/3) > 1e-12 {
		t.Fatalf("agreement %+v", st)
	}
	if math.Abs(st.InputSavings-0.3) > 1e-12 || math.Abs(st.OutputChange-0.3) > 1e-12 {
		t.Fatalf("savings %+v", st)
	}
	if want := 1 - (700+4*130)/(1000+4*100.0); math.Abs(st.MeasuredSavings-want) > 1e-12 {
		t.Fatalf("measured savings %v, want %v", st.MeasuredSavings, want)
	}
	if st.LatencyRatio != 1.5 {
		t.Fatalf("latency ratio %v", st.LatencyRatio)
	}
	if st := ComputeStats(samples(50, 0, 700, 100), 0, 4); st.SE != 1.0/50 {
		t.Fatalf("all-agree SE should be floored at 1/n: %v", st.SE)
	}
	if st := ComputeStats(samples(50, 0, 700, 100), 20, 4); st.N != 20 {
		t.Fatalf("window: N=%d", st.N)
	}
}

func TestNextState(t *testing.T) {
	th := Thresholds{NMin: 100, LookEvery: 50, Z: 2.5, Epsilon: 0.02, Alpha: 0.05, Window: 200, MaxSamples: 400}
	cases := []struct {
		name string
		all  []EvaluationPair
		want policy.PromotionState
		why  string
	}{
		{"too few", samples(50, 0, 700, 100), policy.Shadow, "collecting"},
		{"good", samples(200, 0, 700, 100), policy.Enabled, "promoted"},
		{"straddles", samples(200, 2, 700, 100), policy.Shadow, "straddle"},
		{"worse", samples(200, 40, 700, 100), policy.Off, "rejected"},
		{"inconclusive", samples(400, 8, 700, 100), policy.Off, "inconclusive"},
		{"no input savings", samples(200, 0, 950, 100), policy.Shadow, "measured savings"},
		// 30% fewer input tokens, but the model writes 2.5× more: at 4×
		// output pricing the codec costs more than JSON.
		{"output eats savings", samples(200, 0, 700, 250), policy.Shadow, "measured savings"},
		{"tier1 regression", withTier1(samples(200, 0, 700, 100), repeat([2]bool{true, false}, 12)), policy.Shadow, "McNemar"},
		// Confidently worse and a Tier 1 regression: rejected, not held.
		{"worse with tier1 regression", withTier1(samples(200, 40, 700, 100), repeat([2]bool{true, false}, 40)), policy.Off, "rejected"},
	}
	for _, c := range cases {
		got, why := NextState(policy.Shadow, 0.15, ComputeStats(c.all, 0, 4), ComputeStats(c.all, th.Window, 4), th)
		if got != c.want || !strings.Contains(why, c.why) {
			t.Errorf("%s: got %v (%s)", c.name, got, why)
		}
	}
	// ENABLED demotes when the rolling window is confidently worse, and
	// stays on noise.
	bad := samples(200, 40, 700, 100)
	if got, why := NextState(policy.Enabled, 0.15, ComputeStats(bad, 0, 4), ComputeStats(bad, th.Window, 4), th); got != policy.Shadow || !strings.Contains(why, "demoted") {
		t.Errorf("expected demotion, got %v (%s)", got, why)
	}
	noisy := samples(200, 2, 700, 100)
	if got, _ := NextState(policy.Enabled, 0.15, ComputeStats(noisy, 0, 4), ComputeStats(noisy, th.Window, 4), th); got != policy.Enabled {
		t.Error("demoted on noise")
	}
}

func shadowRoute(t *testing.T) *server.Registry {
	t.Helper()
	reg := server.NewRegistry()
	p := policy.Defaults()
	p.TenantID, p.RouteID, p.Version, p.Codec, p.State = "t", "r", "v1", "tabular", policy.Shadow
	if err := reg.Put(p, nil, false); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestPromoterLooks checks that decisions are taken only at scheduled
// looks: this route first satisfies the promotion bound at 277 samples but
// is promoted at the next look, 280.
func TestPromoterLooks(t *testing.T) {
	reg := shadowRoute(t)
	store, _ := NewMemoryStore("")
	prom := &Promoter{Registry: reg, Store: store, T: Thresholds{NMin: 20, LookEvery: 10, Z: 2.5, Epsilon: 0.02, Alpha: 0.05, Window: 1000}}
	var tr *Transition
	for i, s := range samples(400, 2, 700, 100) {
		s.TenantID, s.RouteID, s.Time = "t", "r", time.Unix(int64(i+1), 0)
		store.Add(s)
		if tr = prom.Evaluate("t", "r"); tr != nil {
			break
		}
	}
	if tr == nil || tr.To != policy.Enabled || tr.Stats.N != 280 {
		t.Fatalf("transition %+v", tr)
	}
	if prom.T.lookIndex(19) != 0 || prom.T.lookIndex(20) != 1 || prom.T.lookIndex(29) != 1 || prom.T.lookIndex(30) != 2 {
		t.Fatal("look schedule")
	}
}

// TestPromoterEvidenceSinceTransition checks that a route needs fresh
// evidence after a transition, also across a restart.
func TestPromoterEvidenceSinceTransition(t *testing.T) {
	reg := shadowRoute(t)
	store, _ := NewMemoryStore("")
	th := Thresholds{NMin: 20, LookEvery: 10, Z: 0.3, Epsilon: 0.02, Alpha: 0.05, Window: 1000}
	prom := &Promoter{Registry: reg, Store: store, T: th}
	add := func(from, n int) {
		for i, s := range samples(n, 0, 700, 100) {
			s.TenantID, s.RouteID, s.Time = "t", "r", time.Unix(int64(from+i), 0)
			store.Add(s)
		}
	}
	add(1, 30) // good evidence, then a demotion at t=100 (e.g. the production circuit breaker)
	reg.SetState("t", "r", policy.Enabled)
	demoted := Transition{Time: time.Unix(100, 0), TenantID: "t", RouteID: "r", PolicyVersion: "v1", From: policy.Enabled, To: policy.Shadow}
	reg.SetState("t", "r", policy.Shadow)
	prom.Resume([]Transition{demoted})
	add(101, 10)
	if tr := prom.Evaluate("t", "r"); tr != nil {
		t.Fatalf("re-promoted on samples from before the demotion: %+v", tr)
	}
	// Without Resume (a restart that forgot the transition) the old
	// samples would count.
	fresh := &Promoter{Registry: reg, Store: store, T: th}
	if tr := fresh.Evaluate("t", "r"); tr == nil || tr.To != policy.Enabled {
		t.Fatal("control: old samples should promote without Resume")
	}
	reg.SetState("t", "r", policy.Shadow)
	add(111, 10)
	if tr := prom.Evaluate("t", "r"); tr == nil || tr.To != policy.Enabled || tr.Stats.N != 20 {
		t.Fatalf("expected promotion on 20 fresh samples, got %+v", tr)
	}
}

func repeat[T any](v T, n int) []T {
	out := make([]T, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func TestStats(t *testing.T) {
	if p := McNemarExact(10, 0); math.Abs(p-0.001953125) > 1e-12 {
		t.Fatal(p)
	}
	if p := BinomialUpperTail(10, 0, 0.3); p != 1 {
		t.Fatal(p)
	}
	if p := BinomialUpperTail(10, 10, 0.5); math.Abs(p-1.0/1024) > 1e-12 {
		t.Fatal(p)
	}
}

// --- end-to-end lifecycle through the real gateway ---

type upstream struct {
	// encodedTenant is the tenant the fake model puts in its tool call when
	// it receives the tabular encoding; JSON requests always get "t1".
	encodedTenant atomic.Value
	calls         atomic.Int64
	encodedCalls  atomic.Int64
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	u.calls.Add(1)
	tenant, in := "t1", 1000
	if strings.Contains(string(b), `[\"amount\"`) {
		u.encodedCalls.Add(1)
		tenant, in = u.encodedTenant.Load().(string), 700
	}
	fmt.Fprintf(w, `{"content":[{"type":"tool_use","id":"x","name":"refund","input":{"order_id":3,"tenant_id":%q}}],"usage":{"input_tokens":%d,"output_tokens":5}}`, tenant, in)
}

func rows(n int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"amount":"%d.5","status":"refunded"}`, i, i)
	}
	b.WriteString("]")
	return b.String()
}

func TestLifecycle(t *testing.T) {
	up := &upstream{}
	up.encodedTenant.Store("t1")
	ups := httptest.NewServer(up)
	defer ups.Close()

	reg := server.NewRegistry()
	p := policy.Defaults()
	p.TenantID, p.RouteID, p.Codec, p.State = "t1", "r", "tabular", policy.Shadow
	p.MinPayloadTokens, p.MinNetSavings, p.ShadowSampleRate, p.AllowFallbackRetry = 50, 0.1, 1, true
	p.ToolSchemas = map[string][]byte{"refund": []byte(`{"type":"object","required":["order_id","tenant_id"]}`)}
	if err := reg.Put(p, nil, true); err != nil {
		t.Fatal(err)
	}
	store, _ := NewMemoryStore("")
	// A small Z keeps the lifecycle short; error rates are covered by
	// TestPromotionErrorRates.
	th := Thresholds{NMin: 12, LookEvery: 4, Z: 0.2, Epsilon: 0.02, Alpha: 0.05, Window: 12, MaxSamples: 1000, ProdMin: 10}
	var transitions []Transition
	prom := &Promoter{Registry: reg, Store: store, T: th, OnTransition: func(tr Transition) { transitions = append(transitions, tr) }}
	ev := NewEvaluator(Config{Store: store, Promoter: prom, RPS: 1000, Burst: 1000,
		Rand: func() float64 { return 0 }}) // deterministic: sample every request
	key := []byte("k")
	gw := httptest.NewServer(server.New(server.Config{Registry: reg, AnthropicBase: ups.URL,
		IdentityMode: server.IdentityJWT, IdentityKey: key, Observers: []server.Observer{prom, ev}}))
	defer gw.Close()

	tok := server.SignHS256(map[string]any{"tenant_id": "t1", "scope": "action:refund", "exp": time.Now().Add(time.Hour).Unix()}, key)
	tr, _ := json.Marshal(rows(30))
	body := fmt.Sprintf(`{"model":"m","max_tokens":10,"tools":[{"name":"refund","input_schema":{"type":"object"}}],
	 "messages":[{"role":"user","content":"q"},{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"lookup","input":{}}]},
	 {"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":%s}]}]}`, tr)
	send := func() *http.Response {
		req, _ := http.NewRequest("POST", gw.URL+"/anthropic/v1/messages", strings.NewReader(body))
		req.Header.Set(server.HeaderRoute, "r")
		req.Header.Set(server.HeaderIdentity, tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	state := func() policy.PromotionState { return reg.Lookup("t1", "r").Policy.State }

	// 1. SHADOW: production stays JSON; samples accumulate; route promotes.
	for i := 0; i < 40 && state() == policy.Shadow; i++ {
		if r := send(); r.Header.Get("X-Trimproof-Representation") != "json" {
			t.Fatalf("SHADOW served %q", r.Header.Get("X-Trimproof-Representation"))
		}
		ev.Drain(context.Background())
	}
	if state() != policy.Enabled {
		t.Fatalf("not promoted; pairs=%d transitions=%+v", len(store.Pairs("t1", "r")), transitions)
	}
	st := ComputeStats(store.Pairs("t1", "r"), 0, p.OutputPriceRatio)
	if st.N != 12 || math.Abs(st.InputSavings-0.3) > 1e-9 || math.Abs(st.MeasuredSavings-(1-720.0/1020)) > 1e-9 {
		t.Fatalf("stats %+v", st)
	}
	// Each sample ran three arms; exactly one of them was encoded.
	if prod := int64(up.calls.Load()) - 3*int64(st.N); prod != int64(st.N) || up.encodedCalls.Load() != int64(st.N) {
		t.Fatalf("upstream calls %d (encoded %d) for %d samples", up.calls.Load(), up.encodedCalls.Load(), st.N)
	}

	// 2. ENABLED: production is encoded.
	if r := send(); !strings.HasPrefix(r.Header.Get("X-Trimproof-Representation"), "tabular") {
		t.Fatalf("ENABLED served %q", r.Header.Get("X-Trimproof-Representation"))
	}
	ev.Drain(context.Background())

	// 3. The model starts misbehaving on the encoding only. Production Tier 1
	// failures on encoded requests trip the circuit breaker (fallback keeps
	// callers whole), and/or shadow pairs show the regression.
	up.encodedTenant.Store("evil")
	for i := 0; i < 40 && state() == policy.Enabled; i++ {
		if r := send(); r.StatusCode != 200 {
			t.Fatalf("caller saw %d; fallback should keep responses valid", r.StatusCode)
		}
		ev.Drain(context.Background())
	}
	if state() != policy.Shadow {
		t.Fatalf("not demoted: %+v", transitions)
	}
	last := transitions[len(transitions)-1]
	if last.From != policy.Enabled || !strings.Contains(last.Reason, "demoted") {
		t.Fatalf("last transition %+v", last)
	}
	t.Logf("transitions: %d; last: %s", len(transitions), last.Reason)
}

func TestProductionCircuitBreaker(t *testing.T) {
	reg := server.NewRegistry()
	p := policy.Defaults()
	p.TenantID, p.RouteID, p.Codec, p.State = "t1", "r", "tabular", policy.Enabled
	if err := reg.Put(p, nil, false); err != nil {
		t.Fatal(err)
	}
	store, _ := NewMemoryStore("")
	var trs []Transition
	prom := &Promoter{Registry: reg, Store: store, T: Thresholds{Epsilon: 0.02, Alpha: 0.05, ProdMin: 20},
		OnTransition: func(tr Transition) { trs = append(trs, tr) }}
	orig := &ir.Request{Model: "m"}
	dec := &policy.Decision{Codec: "tabular"}
	ex := func(sent string, ok, fellBack bool) *server.Exchange {
		return &server.Exchange{Route: reg.Lookup("t1", "r"), Original: orig, Decision: dec, Sent: sent, FellBack: fellBack,
			Tier1: &validator.Result{OK: ok}}
	}
	// Baseline: 1 failure in 100 canonical JSON requests.
	for i := 0; i < 100; i++ {
		prom.Observe(context.Background(), ex("original", i != 0, false))
	}
	// Encoded: failures at 5% stay below significance with n=20.
	for i := 0; i < 20; i++ {
		prom.Observe(context.Background(), ex("encoded", i != 0, false))
	}
	if len(trs) != 0 {
		t.Fatalf("demoted on noise: %+v", trs)
	}
	// Encoded requests that needed a fallback count as first-attempt failures.
	for i := 0; i < 10 && len(trs) == 0; i++ {
		prom.Observe(context.Background(), ex("original", true, true))
	}
	if len(trs) != 1 || trs[0].To != policy.Shadow || !strings.Contains(trs[0].Reason, "production Tier 1") {
		t.Fatalf("transitions %+v", trs)
	}
}

func TestReplayTransitions(t *testing.T) {
	reg := server.NewRegistry()
	for _, r := range []struct {
		id, version string
		state       policy.PromotionState
	}{{"a", "v1", policy.Shadow}, {"b", "v2", policy.Shadow}, {"c", "v1", policy.Manual}} {
		p := policy.Defaults()
		p.TenantID, p.RouteID, p.Version, p.Codec, p.State = "t", r.id, r.version, "tabular", r.state
		if err := reg.Put(p, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	path := t.TempDir() + "/tr.jsonl"
	var buf strings.Builder
	for _, tr := range []Transition{
		{TenantID: "t", RouteID: "a", PolicyVersion: "v1", From: policy.Shadow, To: policy.Enabled},
		{TenantID: "t", RouteID: "b", PolicyVersion: "v1", From: policy.Shadow, To: policy.Enabled}, // stale version
		{TenantID: "t", RouteID: "c", PolicyVersion: "v1", From: policy.Shadow, To: policy.Enabled}, // MANUAL wins
	} {
		b, _ := json.Marshal(tr)
		buf.Write(append(b, '\n'))
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	applied, err := ReplayTransitions(path, reg)
	if err != nil || len(applied) != 1 || applied[0].RouteID != "a" {
		t.Fatalf("restored %+v, err %v", applied, err)
	}
	if reg.Lookup("t", "a").Policy.State != policy.Enabled || reg.Lookup("t", "b").Policy.State != policy.Shadow || reg.Lookup("t", "c").Policy.State != policy.Manual {
		t.Fatal("wrong states after replay")
	}
}

// TestPromotionErrorRates simulates routes under DefaultThresholds and
// checks the promotion rule's error rates. Each sampled question has a
// stability q (75% of questions q=1, 25% q=0.663, giving the 0.86 noise
// floor seen in the end-to-end run). Every arm gives the modal answer with
// probability q (the codec arm q·(1−δ')), and only modal answers agree.
// δ is the codec's true agreement drop.
func TestPromotionErrorRates(t *testing.T) {
	th := DefaultThresholds()
	const routes = 400
	rng := rand.New(rand.NewPCG(1, 2))
	run := func(delta float64) (promoted, rejected int, medianN int) {
		dPrime := delta / 0.86 // E[q²] = 0.86
		var ns []int
		for range routes {
			var acc accumulator
			for n := 1; n <= th.MaxSamples; n++ {
				q := 1.0
				if rng.Float64() < 0.25 {
					q = 0.663
				}
				a, c := rng.Float64() < q, rng.Float64() < q
				b := rng.Float64() < q*(1-dPrime)
				acc.add(sample(a && b, c && b, a && c, 1000, 700, 100, 100))
				if th.lookIndex(n) == th.lookIndex(n-1) {
					continue
				}
				st := acc.stats(4)
				next, why := NextState(policy.Shadow, 0.15, st, st, th)
				switch {
				case next == policy.Enabled:
					promoted++
					ns = append(ns, n)
				case strings.HasPrefix(why, "rejected"):
					rejected++
				}
				if next != policy.Shadow {
					break
				}
			}
		}
		if len(ns) > 0 {
			slices.Sort(ns)
			medianN = ns[len(ns)/2]
		}
		return
	}
	for _, c := range []struct {
		delta                float64
		minPromoted, maxProm float64
		minRejected          float64
	}{
		{0, 0.90, 1, 0},
		{th.Epsilon, 0, 0.08, 0},
		{0.05, 0, 0, 0.95},
		{0.10, 0, 0, 0.99},
	} {
		p, r, med := run(c.delta)
		fp, fr := float64(p)/routes, float64(r)/routes
		t.Logf("true drop %.2f: promoted %.3f (median %d samples), rejected %.3f", c.delta, fp, med, fr)
		if fp < c.minPromoted || fp > c.maxProm || fr < c.minRejected {
			t.Errorf("drop %.2f: promoted %.3f rejected %.3f outside [%v, %v], ≥%v", c.delta, fp, fr, c.minPromoted, c.maxProm, c.minRejected)
		}
	}
}
