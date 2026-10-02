// Package audit implements Tier 2 (spec §9): an observational, bounded,
// drop-on-full audit pipeline with key-based redaction.
package audit

import (
	"strings"

	"github.com/AkashAgarwalInd/context-mesh/pkg/canonical"
	"github.com/AkashAgarwalInd/context-mesh/pkg/provider"
)

// Redacted replaces every redacted value, whatever its type.
const Redacted = "[REDACTED]"

// DefaultRedactKeys are always redacted (case-insensitive, ignoring "_"
// and "-").
var DefaultRedactKeys = []string{
	"password", "passwd", "secret", "token", "access_token", "refresh_token", "api_key", "apikey",
	"authorization", "cookie", "ssn", "credit_card", "card_number", "cvv", "iban", "private_key",
}

func normKey(k string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(k))
}

// Redactor walks JSON trees and redacts by key.
type Redactor struct {
	keys map[string]bool
}

// NewRedactor combines the defaults with extra route keys.
func NewRedactor(extra []string) *Redactor {
	r := &Redactor{keys: map[string]bool{}}
	for _, k := range append(append([]string{}, DefaultRedactKeys...), extra...) {
		r.keys[normKey(k)] = true
	}
	return r
}

// RedactJSON returns data with every value under a sensitive key replaced
// by "[REDACTED]" at any depth. String values that themselves hold JSON
// (tool results are JSON inside a JSON string) are redacted recursively.
// Unparseable input is replaced entirely: failing closed is the only safe
// option for unknown content.
func (r *Redactor) RedactJSON(data []byte) []byte {
	v, err := canonical.Parse(data)
	if err != nil {
		b, _ := provider.Marshal(Redacted)
		return b
	}
	out, err := canonical.Marshal(r.redact(v, 0))
	if err != nil {
		b, _ := provider.Marshal(Redacted)
		return b
	}
	return out
}

func (r *Redactor) redact(v any, depth int) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			if r.keys[normKey(k)] {
				out[k] = Redacted
			} else {
				out[k] = r.redact(e, depth)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = r.redact(e, depth)
		}
		return out
	case string:
		if depth < 4 && provider.LooksLikeJSONData(t) {
			if inner, err := canonical.Parse([]byte(t)); err == nil {
				if b, err := canonical.Marshal(r.redact(inner, depth+1)); err == nil {
					return string(b)
				}
			}
		}
		return t
	}
	return v
}
