// Package canonical implements context-mesh's canonical JSON form (spec §2.1):
// compact output, object keys sorted by byte order at every depth, numbers kept
// as their exact source text, and no HTML escaping.
//
// Parsing is strict: duplicate object keys, invalid UTF-8 and trailing data
// are rejected, so canonicalization is a function rather than a best effort.
package canonical

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"unicode/utf8"
)

// MaxDepth bounds container nesting to keep parsing and encoding stack-safe.
const MaxDepth = 512

// Value is a parsed JSON value. Its dynamic type is one of: nil, bool,
// json.Number, string, []any, or map[string]any.
type Value = any

// Parse decodes a single JSON document. Numbers are returned as json.Number
// with their original lexical form.
func Parse(data []byte) (Value, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("canonical: invalid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseValue(dec, 0)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("canonical: trailing data after JSON value")
	}
	return v, nil
}

func parseValue(dec *json.Decoder, depth int) (Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("canonical: %w", err)
	}
	switch t := tok.(type) {
	case json.Delim:
		if depth >= MaxDepth {
			return nil, fmt.Errorf("canonical: nesting exceeds %d", MaxDepth)
		}
		switch t {
		case '{':
			obj := map[string]any{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, fmt.Errorf("canonical: %w", err)
				}
				key := kt.(string)
				if _, dup := obj[key]; dup {
					return nil, fmt.Errorf("canonical: duplicate key %q", key)
				}
				v, err := parseValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				obj[key] = v
			}
			if _, err := dec.Token(); err != nil {
				return nil, fmt.Errorf("canonical: %w", err)
			}
			return obj, nil
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := parseValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, fmt.Errorf("canonical: %w", err)
			}
			return arr, nil
		}
		return nil, fmt.Errorf("canonical: unexpected delimiter %v", t)
	default:
		// nil, bool, json.Number, string
		return t, nil
	}
}

// Canonicalize parses data and re-encodes it in canonical form.
func Canonicalize(data []byte) ([]byte, error) {
	v, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return Marshal(v)
}

// Marshal encodes a Value in canonical form.
func Marshal(v Value) ([]byte, error) {
	var buf bytes.Buffer
	if err := AppendValue(&buf, v, 0); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// AppendValue writes v in canonical form to buf.
func AppendValue(buf *bytes.Buffer, v Value, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("canonical: nesting exceeds %d", MaxDepth)
	}
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		if !IsNumberLiteral(string(t)) {
			return fmt.Errorf("canonical: invalid number literal %q", string(t))
		}
		buf.WriteString(string(t))
	case string:
		AppendString(buf, t)
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := AppendValue(buf, e, depth+1); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		buf.WriteByte('{')
		for i, k := range SortedKeys(t) {
			if i > 0 {
				buf.WriteByte(',')
			}
			AppendString(buf, k)
			buf.WriteByte(':')
			if err := AppendValue(buf, t[k], depth+1); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("canonical: unsupported type %T", v)
	}
	return nil
}

// SortedKeys returns the object's keys in canonical (byte) order.
func SortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

const hexDigits = "0123456789abcdef"

// AppendString writes s as a JSON string with minimal escaping: quote,
// backslash and control characters only. Everything else, including '<', '>',
// '&' and U+2028/U+2029, is emitted as literal UTF-8.
func AppendString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c != '"' && c != '\\' {
			continue
		}
		buf.WriteString(s[start:i])
		switch c {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		default:
			buf.WriteString(`\u00`)
			buf.WriteByte(hexDigits[c>>4])
			buf.WriteByte(hexDigits[c&0xF])
		}
		start = i + 1
	}
	buf.WriteString(s[start:])
	buf.WriteByte('"')
}

// IsNumberLiteral reports whether s matches the JSON number grammar
// (RFC 8259 §6). Lexical form is otherwise unconstrained: "88.0", "-0" and
// "1e400" are all valid and preserved verbatim.
func IsNumberLiteral(s string) bool {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	if i >= len(s) {
		return false
	}
	switch {
	case s[i] == '0':
		i++
	case s[i] >= '1' && s[i] <= '9':
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	default:
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		d := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == d {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		d := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == d {
			return false
		}
	}
	return i == len(s)
}
