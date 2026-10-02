package toonx

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// maxWidth is the most columns, the row number included, that one table
// gets. On wider tables models count commas to find a column, which costs
// them thousands of output tokens and causes misreads.
const maxWidth = 8

// rowNumber is the column that starts every part of a split table. It sorts
// before any letter, so it is always the first column.
const rowNumber = "#"

// split returns v with every array of objects that has more than maxWidth
// varying columns replaced by {"part1":[…],"part2":[…],…}: the same rows
// cut into column groups of at most maxWidth columns, each starting with
// the row number. Columns are leaf paths, grouped by their first key with
// top-level fields first; fields constant in every row stay in part1. ok
// reports whether anything was split. v is not changed.
func split(v any) (out any, ok bool) {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			var did bool
			m[k], did = split(x)
			ok = ok || did
		}
		return m, ok
	case []any:
		if parts, did := splitTable(t); did {
			return parts, true
		}
		a := make([]any, len(t))
		for i, x := range t {
			var did bool
			a[i], did = split(x)
			ok = ok || did
		}
		return a, ok
	}
	return v, false
}

func splitTable(a []any) (map[string]any, bool) {
	if len(a) < 2 {
		return nil, false
	}
	rows := make([]map[string]any, len(a))
	paths := map[string][]string{}
	for i, x := range a {
		m, isObj := x.(map[string]any)
		if !isObj || len(m) == 0 {
			return nil, false
		}
		if _, clash := m[rowNumber]; clash {
			return nil, false
		}
		rows[i] = map[string]any{}
		collect(nil, m, true, rows[i], paths)
	}
	var consts, vary []string
	for id := range paths {
		if _, ok := constant(rows, id); ok {
			consts = append(consts, id)
		} else {
			vary = append(vary, id)
		}
	}
	if len(vary) <= maxWidth {
		return nil, false
	}
	slices.Sort(vary)
	groups := packGroups(vary, paths, maxWidth-1)
	out := map[string]any{}
	for g, ids := range groups {
		if g == 0 {
			ids = append(slices.Clone(ids), consts...)
		}
		part := make([]any, len(rows))
		for i, r := range rows {
			obj := map[string]any{rowNumber: json.Number(strconv.Itoa(i + 1))}
			for _, id := range ids {
				if x, has := r[id]; has {
					setPath(obj, paths[id], x)
				}
			}
			part[i] = obj
		}
		out[partName(g, len(groups))] = part
	}
	return out, true
}

// partName is "part" and the 1-based group number, zero-padded so the parts
// sort in order.
func partName(g, n int) string {
	return fmt.Sprintf("part%0*d", len(strconv.Itoa(n)), g+1)
}

// packGroups cuts sorted column IDs into groups of at most size: top-level
// fields first, then the leaves of each nested object, keeping columns under
// the same first key together where they fit.
func packGroups(ids []string, paths map[string][]string, size int) [][]string {
	byFirst := map[string][]string{}
	var firsts []string
	for _, id := range ids {
		first := ""
		if p := paths[id]; len(p) > 1 {
			first = p[0]
		}
		if byFirst[first] == nil {
			firsts = append(firsts, first)
		}
		byFirst[first] = append(byFirst[first], id)
	}
	slices.Sort(firsts) // "" (top-level fields) sorts first
	var groups [][]string
	var cur []string
	for _, f := range firsts {
		b := byFirst[f]
		if len(cur)+len(b) <= size {
			cur = append(cur, b...)
			continue
		}
		if len(cur) > 0 {
			groups = append(groups, cur)
		}
		for len(b) > size {
			groups = append(groups, b[:size])
			b = b[size:]
		}
		cur = slices.Clone(b)
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	return groups
}

func setPath(m map[string]any, p []string, v any) {
	for _, k := range p[:len(p)-1] {
		sub, ok := m[k].(map[string]any)
		if !ok {
			sub = map[string]any{}
			m[k] = sub
		}
		m = sub
	}
	m[p[len(p)-1]] = v
}

// join undoes split: every object that is exactly part1…partN (N ≥ 2) of
// equally long arrays of rows numbered 1, 2, … in column "#" becomes one
// array of the merged rows. It sets f.split when it joins anything, and
// fails when two parts hold the same field of a row.
func join(v any, f *features) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		if parts := splitParts(t); parts != nil {
			f.split = true
			return joinParts(parts)
		}
		for k, x := range t {
			var err error
			if t[k], err = join(x, f); err != nil {
				return nil, err
			}
		}
	case []any:
		for i, x := range t {
			var err error
			if t[i], err = join(x, f); err != nil {
				return nil, err
			}
		}
	}
	return v, nil
}

// splitParts returns m's parts in order when m has the shape split writes,
// else nil.
func splitParts(m map[string]any) [][]any {
	if len(m) < 2 {
		return nil
	}
	parts := make([][]any, len(m))
	for g := range parts {
		a, ok := m[partName(g, len(m))].([]any)
		if !ok || len(a) < 2 || len(a) != len(m[partName(0, len(m))].([]any)) {
			return nil
		}
		for i, x := range a {
			r, ok := x.(map[string]any)
			if !ok || r[rowNumber] != json.Number(strconv.Itoa(i+1)) {
				return nil
			}
		}
		parts[g] = a
	}
	return parts
}

var errOverlap = errors.New("toonx: split parts hold the same field")

func joinParts(parts [][]any) ([]any, error) {
	rows := make([]any, len(parts[0]))
	for i := range rows {
		row := map[string]any{}
		for _, p := range parts {
			r := p[i].(map[string]any)
			for k, x := range r {
				if k == rowNumber {
					continue
				}
				if err := mergeField(row, k, x); err != nil {
					return nil, err
				}
			}
		}
		if len(row) == 0 {
			return nil, errOverlap // split never writes a row of only "#"
		}
		rows[i] = row
	}
	return rows, nil
}

// mergeField sets m[k] = x, merging nested objects that two parts share.
// Objects are copied first: the decoder gives every row the same object for
// an "all rows:" value.
func mergeField(m map[string]any, k string, x any) error {
	old, exists := m[k]
	if !exists {
		m[k] = copyObjects(x)
		return nil
	}
	a, aok := old.(map[string]any)
	b, bok := x.(map[string]any)
	if !aok || !bok || len(a) == 0 || len(b) == 0 {
		return fmt.Errorf("%w: %s", errOverlap, strings.TrimSpace(k))
	}
	for kk, xx := range b {
		if err := mergeField(a, kk, xx); err != nil {
			return err
		}
	}
	return nil
}

// copyObjects returns x with every nested object copied.
func copyObjects(x any) any {
	m, ok := x.(map[string]any)
	if !ok {
		return x
	}
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = copyObjects(v)
	}
	return c
}
