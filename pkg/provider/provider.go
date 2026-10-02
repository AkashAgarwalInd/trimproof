// Package provider defines the adapter interface between provider wire
// formats and the IR (spec §4).
package provider

import (
	"bytes"
	"encoding/json"

	"github.com/AkashAgarwalInd/context-mesh/pkg/ir"
)

// Adapter converts between a provider's wire format and the IR.
type Adapter interface {
	Name() string
	ParseRequest(wire []byte) (*ir.Request, error)
	// RenderRequest returns the wire body to forward. If nothing was
	// transformed and no primer is set, it returns req.Raw unchanged.
	RenderRequest(req *ir.Request) ([]byte, error)
	// ParseResponse parses a non-streaming response. req supplies context
	// such as whether structured output was requested.
	ParseResponse(req *ir.Request, wire []byte) (*ir.Response, error)
}

// Marshal encodes v as compact JSON without HTML escaping.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// RawString returns the JSON encoding of s.
func RawString(s string) json.RawMessage {
	b, _ := Marshal(s)
	return b
}

// LooksLikeJSONData reports whether text is a JSON array or object, the
// only shapes the gates can act on. Scalars are not data blocks.
func LooksLikeJSONData(text string) bool {
	t := bytes.TrimSpace([]byte(text))
	if len(t) == 0 || (t[0] != '[' && t[0] != '{') {
		return false
	}
	return json.Valid(t)
}
