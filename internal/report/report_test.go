package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/audit"
	"github.com/AkashAgarwalInd/trimproof/pkg/eval"
	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
)

func writeJSONL(t *testing.T, path string, vs ...any) {
	t.Helper()
	var b bytes.Buffer
	for _, v := range vs {
		line, _ := json.Marshal(v)
		b.Write(append(line, '\n'))
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func sample(at time.Time, agree bool) eval.EvaluationPair {
	yes := eval.Agreement{Agree: true}
	ab := eval.Agreement{Agree: agree}
	return eval.EvaluationPair{Time: at, Kind: eval.Paired, TenantID: "*", RouteID: "orders",
		Agreement: ab, AgreementCB: &ab, ControlAgreement: &yes,
		UsageA: ir.Usage{InputTokens: 1000, OutputTokens: 50}, UsageC: ir.Usage{InputTokens: 1000, OutputTokens: 50},
		UsageB: ir.Usage{InputTokens: 700, OutputTokens: 50}, LatencyA: 100, LatencyB: 110, LatencyC: 100}
}

func TestBuild(t *testing.T) {
	dir := t.TempDir()
	f := Files{Policies: filepath.Join(dir, "p.json"), Pairs: filepath.Join(dir, "pairs.jsonl"),
		Transitions: filepath.Join(dir, "tr.jsonl"), Audit: filepath.Join(dir, "audit.jsonl")}
	os.WriteFile(f.Policies, []byte(`{"routes":[
	 {"route_id":"orders","version":"v1","codec":"toon","state":"SHADOW"},
	 {"route_id":"idle","codec":"toon","state":"OFF"}]}`), 0o600)

	t0 := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	// An earlier promotion and demotion: only samples after the demotion
	// count, as in the gateway.
	writeJSONL(t, f.Transitions,
		eval.Transition{Time: t0.Add(time.Hour), TenantID: "*", RouteID: "orders", PolicyVersion: "v1", From: policy.Shadow, To: policy.Enabled, Reason: "promoted"},
		eval.Transition{Time: t0.Add(2 * time.Hour), TenantID: "*", RouteID: "orders", PolicyVersion: "v1", From: policy.Enabled, To: policy.Shadow, Reason: "demoted"})
	var pairs []any
	for i := 0; i < 50; i++ { // before the demotion: all disagree
		pairs = append(pairs, sample(t0.Add(90*time.Minute), false))
	}
	for i := 0; i < 250; i++ {
		pairs = append(pairs, sample(t0.Add(3*time.Hour), true))
	}
	pairs = append(pairs, eval.EvaluationPair{Time: t0.Add(3 * time.Hour), Kind: eval.Treatment, TenantID: "*", RouteID: "orders"})
	writeJSONL(t, f.Pairs, pairs...)
	writeJSONL(t, f.Audit,
		audit.Entry{TenantID: "*", RouteID: "orders", Status: 200, Sent: "original", Usage: &ir.Usage{InputTokens: 900},
			Gates: []audit.BlockGate{{FailedGate: "min-size"}, {FailedGate: "net-savings"}}},
		audit.Entry{TenantID: "*", RouteID: "orders", Status: 502, Sent: "original"},
		audit.Entry{TenantID: "t9", RouteID: "gone", Status: 200, Sent: "original"})

	r, err := Build(f, eval.DefaultThresholds())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Routes) != 3 {
		t.Fatalf("%d routes", len(r.Routes))
	}
	o := r.Routes[1]
	if o.RouteID != "orders" || o.State != "SHADOW" || o.Since == nil || !o.Since.Equal(t0.Add(2*time.Hour)) {
		t.Fatalf("orders route %+v", o)
	}
	if o.Samples != 251 || o.Legacy != 1 || o.Stats.N != 250 || o.Stats.AgreeTreatment != 1 {
		t.Fatalf("evidence: samples %d legacy %d stats %+v", o.Samples, o.Legacy, o.Stats)
	}
	if o.NextLook != 300 || !strings.HasPrefix(o.Outlook, "SHADOW → ENABLED: promoted: n=250") {
		t.Fatalf("next look %d, outlook %q", o.NextLook, o.Outlook)
	}
	if a := o.Audit; a.Requests != 2 || a.Errors != 1 || a.GateRejections["min-size"] != 1 || a.InputTokens["original"] != 900 {
		t.Fatalf("audit %+v", a)
	}
	if g := r.Routes[2]; g.TenantID != "t9" || g.State != "not in policy file" {
		t.Fatalf("unconfigured route %+v", g)
	}

	var text, js bytes.Buffer
	r.WriteText(&text)
	for _, want := range []string{
		"*/orders  state SHADOW  codec toon  policy v1",
		"250 valid of 251 (1 two-arm samples from the earlier rule, not counted); next look at 300",
		"savings         25.0% net (input 30.0%, output +0.0%), codec latency ×1.10",
		"ENABLED → SHADOW  demoted",
		"gate rejections min-size 1, net-savings 1",
		"*/idle  state OFF",
	} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("text lacks %q:\n%s", want, text.String())
		}
	}
	if err := r.WriteJSON(&js); err != nil || !json.Valid(js.Bytes()) {
		t.Fatalf("json: %v", err)
	}
}

func TestBuildMissingFiles(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.json")
	os.WriteFile(p, []byte(`{"routes":[{"route_id":"a","codec":"toon","state":"SHADOW"}]}`), 0o600)
	r, err := Build(Files{Policies: p, Pairs: filepath.Join(dir, "x"), Transitions: filepath.Join(dir, "y"), Audit: filepath.Join(dir, "z")}, eval.DefaultThresholds())
	if err != nil || len(r.Routes) != 1 {
		t.Fatalf("%v %+v", err, r)
	}
	var b bytes.Buffer
	r.WriteText(&b)
	if !strings.Contains(b.String(), "samples         none yet") {
		t.Fatal(b.String())
	}
}
