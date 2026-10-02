package codec

import (
	"encoding/json"
	"slices"
)

// Kind is the JSON kind of a value, used for per-column consistency checks.
type Kind int

const (
	KindNull Kind = iota
	KindBool
	KindNumber
	KindString
	KindArray
	KindObject
)

// KindOf returns the JSON kind of a canonical.Value.
func KindOf(v any) Kind {
	switch v.(type) {
	case nil:
		return KindNull
	case bool:
		return KindBool
	case json.Number:
		return KindNumber
	case string:
		return KindString
	case []any:
		return KindArray
	default:
		return KindObject
	}
}

// Table is a validated array of objects.
type Table struct {
	Columns []string // sorted
	Rows    []map[string]any
}

// AsTable applies the shared part of Gate 1: v must be a non-empty
// array of non-empty objects with non-empty keys. Under Strict every row has the same key set.
// Under Union the columns are the union of all keys. With primitivesOnly,
// nested containers are rejected. Columns must be kind-consistent; null is
// compatible with any kind.
func AsTable(v any, mode SchemaMode, primitivesOnly bool) (*Table, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, ineligible("top-level value is not an array")
	}
	if len(arr) == 0 {
		return nil, ineligible("empty array")
	}
	rows := make([]map[string]any, len(arr))
	colSet := map[string]struct{}{}
	for i, e := range arr {
		obj, ok := e.(map[string]any)
		if !ok {
			return nil, ineligible("row %d is not an object", i)
		}
		if len(obj) == 0 {
			return nil, ineligible("row %d is an empty object", i)
		}
		rows[i] = obj
		if _, empty := obj[""]; empty {
			return nil, ineligible("row %d has an empty key", i)
		}
		if i == 0 || mode == Union {
			for k := range obj {
				colSet[k] = struct{}{}
			}
		}
	}
	cols := make([]string, 0, len(colSet))
	for k := range colSet {
		cols = append(cols, k)
	}
	slices.Sort(cols)

	kinds := make(map[string]Kind, len(cols))
	for i, row := range rows {
		if mode == Strict && len(row) != len(cols) {
			return nil, ineligible("row %d has %d keys, want %d", i, len(row), len(cols))
		}
		for _, c := range cols {
			val, present := row[c]
			if !present {
				if mode == Strict {
					return nil, ineligible("row %d is missing key %q", i, c)
				}
				continue
			}
			k := KindOf(val)
			if primitivesOnly && (k == KindArray || k == KindObject) {
				return nil, ineligible("row %d key %q holds a nested container", i, c)
			}
			if k == KindNull {
				continue
			}
			if prev, seen := kinds[c]; seen && prev != k {
				return nil, ineligible("column %q mixes kinds", c)
			}
			kinds[c] = k
		}
	}
	return &Table{Columns: cols, Rows: rows}, nil
}
