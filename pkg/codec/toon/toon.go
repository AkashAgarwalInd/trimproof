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

// Check applies Gate 1 for TOON: any JSON object or array whose leaves all
// round-trip through toon-go exactly. Rows need not share keys: TOON writes
// non-uniform rows as a list, where an absent key stays absent.
func (Codec) Check(v any, _ codec.Options) error {
	switch v.(type) {
	case []any, map[string]any:
	default:
		return fmt.Errorf("%w: top-level value is not an object or array", codec.ErrIneligible)
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
		for _, e := range t {
			if err := checkLeaves(e); err != nil {
				return err
			}
		}
	case map[string]any:
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
	want, err := canonical.Marshal(v) // canonicalJSON need not be canonical here
	if err != nil {
		return nil, err
	}
	return c.EncodeValue(v, want, opts)
}

// EncodeValue implements codec.ValueEncoder.
func (c Codec) EncodeValue(v any, canonicalJSON []byte, _ codec.Options) ([]byte, error) {
	enc, err := toongo.Marshal(ordered(v))
	if err != nil {
		return nil, fmt.Errorf("%w: toon-go: %v", codec.ErrIneligible, err)
	}
	// Verify the round trip. toon-go is pre-1.0 and has emitted documents its
	// own decoder rejects (an object list item whose first field is nested),
	// so the codec contract is enforced here rather than trusted.
	got, err := c.Decode(enc)
	if err != nil || string(got) != string(canonicalJSON) {
		return nil, fmt.Errorf("%w: toon-go round trip failed", codec.ErrIneligible)
	}
	return enc, nil
}

// ordered converts objects to toon-go Objects with primitive fields first,
// then arrays, then objects, each group in key order. toon-go writes a list
// item whose first field is a nested object in a form its decoder rejects;
// leading with a primitive avoids that. Decoding sorts keys again, so the
// order is free, and rows of primitives keep the sorted order toon-go uses
// for maps.
func ordered(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = ordered(e)
		}
		return out
	case map[string]any:
		var groups [3][]toongo.Field
		for _, k := range canonical.SortedKeys(t) {
			g := 0
			switch t[k].(type) {
			case []any:
				g = 1
			case map[string]any:
				g = 2
			}
			groups[g] = append(groups[g], toongo.Field{Key: k, Value: ordered(t[k])})
		}
		return toongo.NewObject(append(append(groups[0], groups[1]...), groups[2]...)...)
	}
	return v
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
