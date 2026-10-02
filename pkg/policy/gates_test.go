package policy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AkashAgarwalInd/context-mesh/pkg/codec"
	_ "github.com/AkashAgarwalInd/context-mesh/pkg/codec/tabular"
	_ "github.com/AkashAgarwalInd/context-mesh/pkg/codec/toon"
	"github.com/AkashAgarwalInd/context-mesh/pkg/tokens"
)

func rows(n int) []byte {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"customer-%d","status":"active","amount":"%d.50","region":"eu-west-1"}`, i, i, i*7)
	}
	b.WriteString("]")
	return []byte(b.String())
}

func pol(codecName string, state PromotionState) RoutePolicy {
	p := Defaults()
	p.Codec, p.State = codecName, state
	return p
}

var est = tokens.NewCalibrated(nil)

func TestGates(t *testing.T) {
	big, small := rows(60), rows(2)
	cases := []struct {
		name     string
		p        RoutePolicy
		blocks   [][]byte
		wantGate Gate // first block's failed gate, GateNone if transformed
		prod     bool
		shadow   bool
	}{
		{"no codec", pol("", Enabled), [][]byte{big}, GateOptIn, false, false},
		{"off", pol("toon", Off), [][]byte{big}, GateOptIn, false, false},
		{"not array", pol("toon", Enabled), [][]byte{[]byte(`{"a":1}`)}, GateStructural, false, false},
		{"not json", pol("toon", Enabled), [][]byte{[]byte(`hello`)}, GateStructural, false, false},
		{"small", pol("toon", Enabled), [][]byte{small}, GateMinSize, false, false},
		{"enabled", pol("toon", Enabled), [][]byte{big}, GateNone, true, false},
		{"manual", pol("tabular", Manual), [][]byte{big}, GateNone, true, false},
		{"shadow", pol("toon", Shadow), [][]byte{big}, GateNone, false, true},
		{"union needs opt-in", func() RoutePolicy { p := pol("tabular", Enabled); p.SchemaMode = codec.Union; return p }(), [][]byte{big}, GateOptIn, false, false},
		{"union opted in", func() RoutePolicy {
			p := pol("tabular", Enabled)
			p.SchemaMode, p.Lossy.AllowUnion = codec.Union, true
			return p
		}(), [][]byte{big}, GateNone, true, false},
		{"high bar", func() RoutePolicy { p := pol("toon", Enabled); p.MinNetSavings = 0.99; return p }(), [][]byte{big}, GateNetSavings, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := Decide(c.p, c.blocks, est, "gpt-test")
			if err != nil {
				t.Fatal(err)
			}
			b := d.Blocks[0]
			if b.FailedGate != c.wantGate {
				t.Fatalf("gate = %v (%s), want %v", b.FailedGate, b.Reason, c.wantGate)
			}
			if b.Transform != (c.wantGate == GateNone) {
				t.Fatalf("transform = %v", b.Transform)
			}
			if d.ApplyToProduction != c.prod || d.ShadowOnly != c.shadow {
				t.Fatalf("prod=%v shadow=%v", d.ApplyToProduction, d.ShadowOnly)
			}
		})
	}
}

func TestNetSavingsIncludesPrimer(t *testing.T) {
	d, err := Decide(pol("toon", Enabled), [][]byte{rows(60)}, est, "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	if d.PrimerTokens == 0 {
		t.Fatal("primer not counted")
	}
	want := float64(d.JSONTokens-d.EncTokens-d.PrimerTokens) / float64(d.JSONTokens)
	if d.NetSavings != want || d.NetSavings <= 0.15 {
		t.Fatalf("net savings %v (want %v, > 0.15)", d.NetSavings, want)
	}
}

func TestMixedBlocks(t *testing.T) {
	d, err := Decide(pol("tabular", Enabled), [][]byte{rows(60), []byte(`"text"`), rows(40)}, est, "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	got := []bool{d.Blocks[0].Transform, d.Blocks[1].Transform, d.Blocks[2].Transform}
	if !got[0] || got[1] || !got[2] {
		t.Fatalf("transforms = %v", got)
	}
}
