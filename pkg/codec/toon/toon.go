// Package toon wraps github.com/toon-format/toon-go as a trimproof codec.
//
// toon-go routes every number through float64, which silently changes
// 9007199254740993, turns 1e400 into null and rewrites 88.0 as 88. To keep
// the lossless contract, Check only admits payloads whose number literals
// already survive that trip unchanged and whose strings toon-go can
// represent. Everything else is ineligible and falls back to another codec or
// canonical JSON.
package toon

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	toongo "github.com/toon-format/toon-go"
)

const primer = `Some tool results are encoded in TOON (Token-Oriented Object Notation). "key[N]{a,b}:" introduces an array of N objects with fields a and b, followed by one indented line per object with comma-separated values in that field order. "key[N]: x,y" is an array of primitives. "key: value" is an object field; nested objects are indented. Strings are quoted only when needed.`

// Codec is the TOON codec. The zero value is ready to use.
type Codec struct{}

func init() { codec.Register(Codec{}) }

func (Codec) Name() string    { return "toon" }
func (Codec) Version() string { return "toon-go@863710a7626b" }
func (Codec) Primer() string  { return primer }

// Lossless is false under Union: toon-go would encode missing keys
// differently from nulls, but Union tables are still declared lossy.
func (Codec) Lossless(mode codec.SchemaMode) bool { return mode == codec.Strict }

// Check applies Gate 1 for TOON: an array of uniform objects (nesting allowed)
// whose leaves all round-trip through toon-go exactly.
func (Codec) Check(v any, opts codec.Options) error {
	if _, err := codec.AsTable(v, opts.Mode, false); err != nil {
		return err
	}
	return checkLeaves(v)
}

func checkLeaves(v any) error {
	switch t := v.(type) {
	case json.Number:
		if !FloatExact(string(t)) {
			return fmt.Errorf("%w: number %s does not survive float64 formatting", codec.ErrIneligible, string(t))
		}
	case string:
		for _, r := range t {
			if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
				return fmt.Errorf("%w: string contains control character U+%04X", codec.ErrIneligible, r)
			}
		}
	case []any:
		if len(t) == 0 {
			// toon-go can emit empty arrays, but keep the contract simple.
			return fmt.Errorf("%w: nested empty array", codec.ErrIneligible)
		}
		for _, e := range t {
			if err := checkLeaves(e); err != nil {
				return err
			}
		}
	case map[string]any:
		if len(t) == 0 {
			return fmt.Errorf("%w: nested empty object", codec.ErrIneligible)
		}
		for k, e := range t {
			if err := checkLeaves(k); err != nil {
				return err
			}
			if err := checkLeaves(e); err != nil {
				return err
			}
		}
	}
	return nil
}

// FloatExact reports whether literal equals its own float64 shortest 'f'
// formatting, i.e. toon-go will emit and decode it verbatim.
func FloatExact(literal string) bool {
	f, err := strconv.ParseFloat(literal, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return false
	}
	if f == 0 && math.Signbit(f) {
		return false
	}
	return strconv.FormatFloat(f, 'f', -1, 64) == literal
}

func (c Codec) Encode(canonicalJSON []byte, opts codec.Options) ([]byte, error) {
	v, err := canonical.Parse(canonicalJSON)
	if err != nil {
		return nil, err
	}
	if err := c.Check(v, opts); err != nil {
		return nil, err
	}
	enc, err := toongo.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: toon-go: %v", codec.ErrIneligible, err)
	}
	// Verify the round trip. toon-go is pre-1.0 and has emitted documents its
	// own decoder rejects (an object list item whose first field is nested),
	// so the codec contract is enforced here rather than trusted.
	want, err := canonical.Marshal(v)
	if err != nil {
		return nil, err
	}
	got, err := c.Decode(enc)
	if err != nil || string(got) != string(want) {
		return nil, fmt.Errorf("%w: toon-go round trip failed", codec.ErrIneligible)
	}
	return enc, nil
}

// Decode parses TOON and returns canonical JSON. Numbers come back from
// toon-go as float64 and are formatted with the same shortest 'f' form that
// Check required of the input.
func (Codec) Decode(encoded []byte) ([]byte, error) {
	v, err := toongo.Decode(encoded, toongo.WithStrictMode(true))
	if err != nil {
		return nil, fmt.Errorf("toon: %w", err)
	}
	cv, err := toCanonical(v)
	if err != nil {
		return nil, err
	}
	return canonical.Marshal(cv)
}

func toCanonical(v any) (any, error) {
	switch t := v.(type) {
	case nil, bool, string:
		return t, nil
	case float64:
		return json.Number(strconv.FormatFloat(t, 'f', -1, 64)), nil
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			c, err := toCanonical(e)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			c, err := toCanonical(e)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	case toongo.Object:
		out := make(map[string]any, len(t.Fields))
		for _, f := range t.Fields {
			c, err := toCanonical(f.Value)
			if err != nil {
				return nil, err
			}
			out[f.Key] = c
		}
		return out, nil
	}
	return nil, fmt.Errorf("toon: unexpected decoded type %T", v)
}
