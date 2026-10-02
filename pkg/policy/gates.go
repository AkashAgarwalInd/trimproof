package policy

import (
	"errors"
	"fmt"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

// Gate identifies which of the four ordered gates rejected a block.
type Gate int

const (
	GateNone Gate = iota
	GateStructural
	GateOptIn
	GateMinSize
	GateNetSavings
)

func (g Gate) String() string {
	return [...]string{"none", "structural", "opt-in", "min-size", "net-savings"}[g]
}

// BlockDecision is the outcome for one data block.
type BlockDecision struct {
	Index      int
	Transform  bool
	FailedGate Gate
	Reason     string
	Canonical  []byte // compact canonical JSON (the Gate 4 baseline)
	Encoded    []byte
	JSONTokens int
	EncTokens  int
}

// Decision is the outcome for a whole request.
type Decision struct {
	Codec        string
	Blocks       []BlockDecision
	PrimerTokens int
	JSONTokens   int     // Σ baseline tokens over transformed blocks
	EncTokens    int     // Σ encoded tokens over transformed blocks
	NetSavings   float64 // (JSON − Enc − Primer) / JSON
	// ApplyToProduction is true when the route is ENABLED or MANUAL.
	// ShadowOnly is true when the route is in SHADOW: the transform is used
	// only on the shadow arm.
	ApplyToProduction bool
	ShadowOnly        bool
}

// Any reports whether at least one block is transformed.
func (d *Decision) Any() bool {
	for _, b := range d.Blocks {
		if b.Transform {
			return true
		}
	}
	return false
}

// Decide runs the four gates over the request's data blocks. Each block is
// raw JSON. A route with no codec configured short-circuits before any
// parsing, so OFF traffic pays nothing.
func Decide(p RoutePolicy, blocks [][]byte, est tokens.Estimator, model string) (*Decision, error) {
	d := &Decision{Codec: p.Codec, Blocks: make([]BlockDecision, len(blocks))}
	for i := range d.Blocks {
		d.Blocks[i].Index = i
	}
	reject := func(g Gate, reason string) *Decision {
		for i := range d.Blocks {
			if d.Blocks[i].FailedGate == GateNone {
				d.Blocks[i].FailedGate, d.Blocks[i].Reason = g, reason
			}
			d.Blocks[i].Transform = false
		}
		return d
	}
	if p.Codec == "" {
		return reject(GateOptIn, "route has no codec"), nil
	}
	c, ok := codec.Get(p.Codec)
	if !ok {
		return nil, fmt.Errorf("policy: unknown codec %q", p.Codec)
	}
	opts := codec.Options{Mode: p.SchemaMode}

	candidates := 0
	for i, raw := range blocks {
		b := &d.Blocks[i]
		// Gate 1: structural.
		v, err := canonical.Parse(raw)
		if err != nil {
			b.FailedGate, b.Reason = GateStructural, err.Error()
			continue
		}
		if b.Canonical, err = canonical.Marshal(v); err != nil {
			b.FailedGate, b.Reason = GateStructural, err.Error()
			continue
		}
		if err := c.Check(v, opts); err != nil {
			b.FailedGate, b.Reason = GateStructural, err.Error()
			continue
		}
		// Gate 2: opt-in.
		if p.State == Off {
			b.FailedGate, b.Reason = GateOptIn, "route is OFF"
			continue
		}
		if !c.Lossless(p.SchemaMode) && !(p.SchemaMode == codec.Union && p.Lossy.AllowUnion) {
			b.FailedGate, b.Reason = GateOptIn, "lossy mode without LossyOptimizationPolicy"
			continue
		}
		// Gate 3: minimum size, byte pre-check before the estimator.
		if len(b.Canonical) < p.MinPayloadTokens*tokens.MinBytesPerToken {
			b.FailedGate, b.Reason = GateMinSize, "below byte pre-check"
			continue
		}
		b.JSONTokens = est.Estimate(string(b.Canonical), model)
		if b.JSONTokens < p.MinPayloadTokens {
			b.FailedGate, b.Reason = GateMinSize, fmt.Sprintf("%d < %d tokens", b.JSONTokens, p.MinPayloadTokens)
			continue
		}
		// Encode is authoritative for eligibility.
		b.Encoded, err = c.Encode(b.Canonical, opts)
		if errors.Is(err, codec.ErrIneligible) {
			b.FailedGate, b.Reason = GateStructural, err.Error()
			continue
		}
		if err != nil {
			return nil, err
		}
		b.EncTokens = est.Estimate(string(b.Encoded), model)
		// A block that does not shrink on its own only adds cost.
		if b.EncTokens >= b.JSONTokens {
			b.FailedGate, b.Reason = GateNetSavings, "encoding is not smaller"
			continue
		}
		b.Transform = true
		candidates++
	}
	if candidates == 0 {
		return d, nil
	}

	// Gate 4: net savings over all transformed blocks, primer included once.
	d.PrimerTokens = est.Estimate(c.Primer(), model)
	for _, b := range d.Blocks {
		if b.Transform {
			d.JSONTokens += b.JSONTokens
			d.EncTokens += b.EncTokens
		}
	}
	d.NetSavings = float64(d.JSONTokens-d.EncTokens-d.PrimerTokens) / float64(d.JSONTokens)
	if d.NetSavings < p.MinNetSavings {
		return reject(GateNetSavings, fmt.Sprintf("net savings %.1f%% < %.1f%%", 100*d.NetSavings, 100*p.MinNetSavings)), nil
	}
	d.ApplyToProduction = p.State == Enabled || p.State == Manual
	d.ShadowOnly = p.State == Shadow
	return d, nil
}
