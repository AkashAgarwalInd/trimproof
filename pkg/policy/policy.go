// Package policy holds per-route policy types and the four-gate
// representation decision engine.
package policy

import "github.com/AkashAgarwalInd/trimproof/pkg/codec"

// PromotionState is a route's position in the promotion state machine.
type PromotionState int

const (
	Off PromotionState = iota
	Shadow
	Enabled
	Manual
)

func (s PromotionState) String() string {
	switch s {
	case Shadow:
		return "SHADOW"
	case Enabled:
		return "ENABLED"
	case Manual:
		return "MANUAL"
	}
	return "OFF"
}

// LossyOptimizationPolicy opts a route into lossy transforms. All fields
// default to false.
type LossyOptimizationPolicy struct {
	AllowUnion bool
}

// RoutePolicy is the versioned per-tenant, per-route configuration.
type RoutePolicy struct {
	TenantID, RouteID, Version string

	Codec              string // "toon" | "tabular" | "" (none)
	SchemaMode         codec.SchemaMode
	State              PromotionState
	MinPayloadTokens   int
	MinNetSavings      float64 // fraction, vs compact canonical JSON incl. primer
	OutputPriceRatio   float64 // output-token price / input-token price; weights output tokens in measured savings
	ShadowSampleRate   float64
	AuditSampleRate    float64
	AllowFallbackRetry bool
	// AutoDetectData treats JSON in user messages as data blocks: a text
	// part that is wholly a JSON array or object, or fenced json code blocks.
	AutoDetectData bool
	Lossy          LossyOptimizationPolicy

	ToolSchemas             map[string][]byte
	Rules                   []string
	RedactKeys              []string
	EnableRawPayloadLogging bool
}

// Defaults returns conservative defaults: no codec, OFF, 200-token minimum,
// 15% minimum net savings, output tokens priced at 4× input.
func Defaults() RoutePolicy {
	return RoutePolicy{
		MinPayloadTokens: 200,
		MinNetSavings:    0.15,
		OutputPriceRatio: 4,
		ShadowSampleRate: 0.05,
		AuditSampleRate:  0.01,
	}
}
