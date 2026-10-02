// Package eval implements shadow paired evaluation and the per-route
// promotion state machine (spec §8), the product's differentiator.
package eval

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/AkashAgarwalInd/context-mesh/pkg/canonical"
	"github.com/AkashAgarwalInd/context-mesh/pkg/ir"
)

// Agreement holds the per-pair metrics (spec §8.2).
type Agreement struct {
	// Agree is the primary binary metric used for promotion: same tool
	// calls (name + canonical args) and the same structured output or
	// normalized text.
	Agree bool `json:"agree"`
	// ExactMatch: canonical JSON of the whole response content is equal.
	ExactMatch bool `json:"exact_match"`
	// ToolCallsEqual: same multiset of (name, canonical args).
	ToolCallsEqual bool `json:"tool_calls_equal"`
	// FieldAgreement is the Jaccard similarity of (path, value) leaves over
	// tool-call args and structured output. 1 when both have none.
	FieldAgreement float64 `json:"field_agreement"`
	TextEqual      bool    `json:"text_equal"`
}

// Comparator compares the two arms of a pair.
type Comparator interface {
	Compare(a, b *ir.Response) Agreement
}

// DefaultComparator implements exact, field-level and tool-call metrics.
type DefaultComparator struct{}

func (DefaultComparator) Compare(a, b *ir.Response) Agreement {
	var ag Agreement
	ka, kb := callKeys(a), callKeys(b)
	ag.ToolCallsEqual = slices.Equal(ka, kb)
	ag.TextEqual = normText(a.Text) == normText(b.Text)
	sa, sb := canon(a.Structured), canon(b.Structured)
	structEqual := sa == sb
	ag.ExactMatch = ag.ToolCallsEqual && structEqual && ag.TextEqual
	if a.Structured != nil || b.Structured != nil {
		ag.Agree = ag.ToolCallsEqual && structEqual
	} else {
		ag.Agree = ag.ToolCallsEqual && ag.TextEqual
	}
	ag.FieldAgreement = jaccard(leaves(a), leaves(b))
	return ag
}

func callKeys(r *ir.Response) []string {
	out := make([]string, len(r.ToolCalls))
	for i, c := range r.ToolCalls {
		out[i] = c.Name + "\x00" + canon(c.Args)
	}
	slices.Sort(out)
	return out
}

func canon(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	c, err := canonical.Canonicalize(raw)
	if err != nil {
		return "\x00invalid:" + string(raw)
	}
	return string(c)
}

func normText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func leaves(r *ir.Response) map[string]bool {
	out := map[string]bool{}
	add := func(prefix string, raw json.RawMessage) {
		v, err := canonical.Parse(raw)
		if err != nil {
			out[prefix+"=\x00invalid"] = true
			return
		}
		walkLeaves(prefix, v, out)
	}
	for _, c := range r.ToolCalls {
		add("call:"+c.Name, c.Args)
	}
	if r.Structured != nil {
		add("out", r.Structured)
	}
	return out
}

func walkLeaves(path string, v any, out map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			walkLeaves(path+"/"+k, e, out)
		}
	case []any:
		for i, e := range t {
			walkLeaves(path+"/"+itoa(i), e, out)
		}
	default:
		b, _ := canonical.Marshal(t)
		out[path+"="+string(b)] = true
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}
