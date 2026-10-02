package validator

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sync"

	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
)

var (
	ruleMu   sync.RWMutex
	ruleRegs = map[string]Rule{}
)

// RegisterRule makes a rule available to route policies by name.
func RegisterRule(r Rule) {
	ruleMu.Lock()
	defer ruleMu.Unlock()
	if _, dup := ruleRegs[r.Name()]; dup {
		panic("validator: duplicate rule " + r.Name())
	}
	ruleRegs[r.Name()] = r
}

// Rules resolves registered rule names.
func Rules(names []string) ([]Rule, error) {
	ruleMu.RLock()
	defer ruleMu.RUnlock()
	out := make([]Rule, 0, len(names))
	for _, n := range names {
		r, ok := ruleRegs[n]
		if !ok {
			return nil, fmt.Errorf("validator: unknown rule %q", n)
		}
		out = append(out, r)
	}
	return out, nil
}

// MaxAmount rejects calls to Tool whose numeric Field exceeds Max, using
// exact decimal arithmetic. A missing field is a violation (default-deny);
// make the field required in the schema to get a clearer message.
type MaxAmount struct {
	RuleName string
	Tool     string
	Field    string
	Max      string // decimal literal, e.g. "500.00"
}

func (m MaxAmount) Name() string { return m.RuleName }

func (m MaxAmount) Check(call ir.ToolCall, args any, _ SecurityContext) []Violation {
	if call.Name != m.Tool {
		return nil
	}
	limit, ok := new(big.Rat).SetString(m.Max)
	if !ok {
		return []Violation{{Message: fmt.Sprintf("rule misconfigured: bad limit %q", m.Max)}}
	}
	obj, _ := args.(map[string]any)
	raw, present := obj[m.Field]
	if !present {
		return []Violation{{Message: fmt.Sprintf("%s is required by rule", m.Field)}}
	}
	var lit string
	switch t := raw.(type) {
	case json.Number:
		lit = t.String()
	case string: // amounts are often sent as decimal strings
		lit = t
	default:
		return []Violation{{Message: fmt.Sprintf("%s is not a number", m.Field)}}
	}
	val, ok := new(big.Rat).SetString(lit)
	if !ok {
		return []Violation{{Message: fmt.Sprintf("%s=%q is not a decimal", m.Field, lit)}}
	}
	if val.Cmp(limit) > 0 {
		return []Violation{{Message: fmt.Sprintf("%s=%s exceeds limit %s", m.Field, lit, m.Max)}}
	}
	return nil
}
