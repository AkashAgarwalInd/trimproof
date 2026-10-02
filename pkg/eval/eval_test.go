package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/context-mesh/context-mesh/pkg/codec/tabular"
	"github.com/context-mesh/context-mesh/pkg/ir"
	"github.com/context-mesh/context-mesh/pkg/policy"
	"github.com/context-mesh/context-mesh/pkg/server"
	"github.com/context-mesh/context-mesh/pkg/validator"
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

func pairs(kind PairKind, n, agree int, inA, inB int, t1 [][2]bool) []EvaluationPair {
	var out []EvaluationPair
	for i := 0; i < n; i++ {
		p := EvaluationPair{Kind: kind, Agreement: Agreement{Agree: i < agree},
			UsageA: ir.Usage{InputTokens: inA}, UsageB: ir.Usage{InputTokens: inB}}
		if i < len(t1) {
			a, b := t1[i][0], t1[i][1]
			p.Tier1A, p.Tier1B = &a, &b
		}
		out = append(out, p)
	}
	return out
}

func TestNextState(t *testing.T) {
	th := Thresholds{NMin: 100, NControlMin: 20, Epsilon: 0.02, Alpha: 0.05, Window: 100}
	ctrl := pairs(Control, 40, 36, 1000, 1000, nil) // noise floor: 90% agreement
	cases := []struct {
		name  string
		treat []EvaluationPair
		want  policy.PromotionState
		why   string
	}{
		{"too few", pairs(Treatment, 50, 50, 1000, 700, nil), policy.Shadow, "collecting"},
		{"good", pairs(Treatment, 100, 89, 1000, 700, nil), policy.Enabled, "promoted"},
		{"below noise floor", pairs(Treatment, 100, 80, 1000, 700, nil), policy.Shadow, "noise floor"},
		{"no savings", pairs(Treatment, 100, 95, 1000, 950, nil), policy.Shadow, "savings"},
		{"tier1 regression", pairs(Treatment, 100, 95, 1000, 700, repeat([2]bool{true, false}, 12)), policy.Shadow, "McNemar"},
	}
	for _, c := range cases {
		all := append(append([]EvaluationPair{}, ctrl...), c.treat...)
		got, why := NextState(policy.Shadow, 0.15, ComputeStats(all, 0), ComputeStats(all, th.Window), th)
		if got != c.want || !strings.Contains(why, c.why) {
			t.Errorf("%s: got %v (%s)", c.name, got, why)
		}
	}
	// ENABLED demotes on rolling regression.
	all := append(append([]EvaluationPair{}, ctrl...), pairs(Treatment, 100, 70, 1000, 700, nil)...)
	if got, _ := NextState(policy.Enabled, 0.15, ComputeStats(all, 0), ComputeStats(all, th.Window), th); got != policy.Shadow {
		t.Error("expected demotion")
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
	th := Thresholds{NMin: 12, NControlMin: 4, Epsilon: 0.02, Alpha: 0.05, Window: 12, ProdMin: 10}
	var transitions []Transition
	prom := &Promoter{Registry: reg, Store: store, T: th, OnTransition: func(tr Transition) { transitions = append(transitions, tr) }}
	n := 0
	ev := NewEvaluator(Config{Store: store, Promoter: prom, RPS: 1000, Burst: 1000, ControlFraction: 0.25,
		Rand: func() float64 { n++; return float64(n%4) / 4 }}) // deterministic: samples always (rate 1), every 4th is a control pair
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

	// 1. SHADOW: production stays JSON; pairs accumulate; route promotes.
	for i := 0; i < 40 && state() == policy.Shadow; i++ {
		if r := send(); r.Header.Get("X-Context-Mesh-Representation") != "json" {
			t.Fatalf("SHADOW served %q", r.Header.Get("X-Context-Mesh-Representation"))
		}
		ev.Drain(context.Background())
	}
	if state() != policy.Enabled {
		t.Fatalf("not promoted; pairs=%d transitions=%+v", len(store.Pairs("t1", "r")), transitions)
	}
	st := ComputeStats(store.Pairs("t1", "r"), 0)
	if st.NControl == 0 || math.Abs(st.MeasuredSavings-0.3) > 1e-9 {
		t.Fatalf("stats %+v", st)
	}

	// 2. ENABLED: production is encoded.
	if r := send(); !strings.HasPrefix(r.Header.Get("X-Context-Mesh-Representation"), "tabular") {
		t.Fatalf("ENABLED served %q", r.Header.Get("X-Context-Mesh-Representation"))
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
	n, err := ReplayTransitions(path, reg)
	if err != nil || n != 1 {
		t.Fatalf("restored %d, err %v", n, err)
	}
	if reg.Lookup("t", "a").Policy.State != policy.Enabled || reg.Lookup("t", "b").Policy.State != policy.Shadow || reg.Lookup("t", "c").Policy.State != policy.Manual {
		t.Fatal("wrong states after replay")
	}
}
