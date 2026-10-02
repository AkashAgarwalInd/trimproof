package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/AkashAgarwalInd/context-mesh/pkg/codec"
	"github.com/AkashAgarwalInd/context-mesh/pkg/policy"
	"github.com/AkashAgarwalInd/context-mesh/pkg/validator"
)

// Route is a resolved policy plus its compiled Tier 1 validator (nil when
// the route does not validate).
type Route struct {
	Policy    policy.RoutePolicy
	Validator *validator.Validator
}

// Registry is the file-based policy registry (OSS core). It is keyed by
// tenant and route; tenant "*" is a fallback for any tenant.
type Registry struct {
	mu     sync.RWMutex
	routes map[string]*Route
}

func key(tenant, route string) string { return tenant + "\x00" + route }

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{routes: map[string]*Route{}} }

// Put adds or replaces a route, compiling its validator.
func (r *Registry) Put(p policy.RoutePolicy, outputSchema []byte, validate bool) error {
	rt := &Route{Policy: p}
	if validate {
		rules, err := validator.Rules(p.Rules)
		if err != nil {
			return err
		}
		v, err := validator.New(validator.Config{ToolSchemas: p.ToolSchemas, OutputSchema: outputSchema, Rules: rules})
		if err != nil {
			return err
		}
		rt.Validator = v
	}
	if p.Codec != "" {
		if _, ok := codec.Get(p.Codec); !ok {
			return fmt.Errorf("registry: route %s/%s: unknown codec %q", p.TenantID, p.RouteID, p.Codec)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[key(p.TenantID, p.RouteID)] = rt
	return nil
}

// Lookup resolves (tenant, route), falling back to ("*", route). Unknown
// routes get a pass-through default policy.
func (r *Registry) Lookup(tenant, route string) *Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rt, ok := r.routes[key(tenant, route)]; ok {
		return rt
	}
	if rt, ok := r.routes[key("*", route)]; ok {
		return rt
	}
	p := policy.Defaults()
	p.TenantID, p.RouteID = tenant, route
	return &Route{Policy: p}
}

// SetState moves a route through the promotion state machine. The route
// keeps every other setting.
func (r *Registry) SetState(tenant, route string, s policy.PromotionState) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.routes[key(tenant, route)]
	if !ok {
		return false
	}
	cp := *rt
	cp.Policy.State = s
	r.routes[key(tenant, route)] = &cp
	return true
}

// File format for LoadRegistry.
type fileRoute struct {
	TenantID           string                     `json:"tenant_id"`
	RouteID            string                     `json:"route_id"`
	Version            string                     `json:"version"`
	Codec              string                     `json:"codec"`
	SchemaMode         string                     `json:"schema_mode"`
	State              string                     `json:"state"`
	MinPayloadTokens   *int                       `json:"min_payload_tokens"`
	MinNetSavings      *float64                   `json:"min_net_savings"`
	ShadowSampleRate   *float64                   `json:"shadow_sample_rate"`
	AuditSampleRate    *float64                   `json:"audit_sample_rate"`
	AllowFallbackRetry bool                       `json:"allow_fallback_retry"`
	AllowUnion         bool                       `json:"allow_union"`
	Validate           bool                       `json:"validate"`
	ToolSchemas        map[string]json.RawMessage `json:"tool_schemas"`
	OutputSchema       json.RawMessage            `json:"output_schema"`
	Rules              []string                   `json:"rules"`
	RedactKeys         []string                   `json:"redact_keys"`
	RawPayloadLogging  bool                       `json:"enable_raw_payload_logging"`
}

// LoadRegistry reads a JSON policy file: {"routes":[...]}.
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f struct {
		Routes []fileRoute `json:"routes"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("registry: %s: %w", path, err)
	}
	reg := NewRegistry()
	for _, fr := range f.Routes {
		p := policy.Defaults()
		p.TenantID, p.RouteID, p.Version, p.Codec = fr.TenantID, fr.RouteID, fr.Version, fr.Codec
		if p.TenantID == "" {
			p.TenantID = "*"
		}
		switch strings.ToUpper(fr.SchemaMode) {
		case "", "STRICT":
		case "UNION":
			p.SchemaMode = codec.Union
		default:
			return nil, fmt.Errorf("registry: bad schema_mode %q", fr.SchemaMode)
		}
		switch strings.ToUpper(fr.State) {
		case "", "OFF":
		case "SHADOW":
			p.State = policy.Shadow
		case "ENABLED":
			p.State = policy.Enabled
		case "MANUAL":
			p.State = policy.Manual
		default:
			return nil, fmt.Errorf("registry: bad state %q", fr.State)
		}
		setIf(&p.MinPayloadTokens, fr.MinPayloadTokens)
		setIf(&p.MinNetSavings, fr.MinNetSavings)
		setIf(&p.ShadowSampleRate, fr.ShadowSampleRate)
		setIf(&p.AuditSampleRate, fr.AuditSampleRate)
		p.AllowFallbackRetry, p.Lossy.AllowUnion = fr.AllowFallbackRetry, fr.AllowUnion
		p.Rules, p.RedactKeys, p.EnableRawPayloadLogging = fr.Rules, fr.RedactKeys, fr.RawPayloadLogging
		if len(fr.ToolSchemas) > 0 {
			p.ToolSchemas = map[string][]byte{}
			for k, v := range fr.ToolSchemas {
				p.ToolSchemas[k] = v
			}
		}
		if err := reg.Put(p, fr.OutputSchema, fr.Validate); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func setIf[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}
