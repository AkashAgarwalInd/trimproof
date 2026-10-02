package bench

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
)

// splitWidth is the most columns, the row number included, that one table gets
// in the toonx-split arm. Wider tables cost models a column count per
// lookup (Amendment 6).
const splitWidth = 8

// splitPrimer is added to the toonx primer when a table was split.
const splitPrimer = ` A wide array may be split into tables part1, part2 and so on: they hold the same rows in the same order, and column # is the row number.`

// splitKey is the row-number column that starts every part. It sorts
// before any letter, so it is always the first column.
const splitKey = "#"

// splitWide returns v with every array of objects that has more than
// splitWidth varying columns replaced by {"part1":[…],"part2":[…],…}: the
// same rows cut into column groups of at most splitWidth columns, each
// starting with the row number. Columns are leaf paths, grouped by their first
// key; fields constant in every row stay in part1. ok reports whether
// anything was split. The input is not changed.
func splitWide(v any) (out any, ok bool) {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			var did bool
			m[k], did = splitWide(x)
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
			a[i], did = splitWide(x)
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
		if _, clash := m[splitKey]; clash {
			return nil, false
		}
		rows[i] = map[string]any{}
		leafPaths(nil, m, rows[i], paths)
	}
	var consts, vary []string
	for id := range paths {
		if isConstant(rows, id) {
			consts = append(consts, id)
		} else {
			vary = append(vary, id)
		}
	}
	if len(vary) <= splitWidth {
		return nil, false
	}
	slices.Sort(vary)
	groups := packGroups(vary, paths, splitWidth-1)
	out := map[string]any{}
	digits := len(strconv.Itoa(len(groups)))
	for g, ids := range groups {
		if g == 0 {
			ids = append(slices.Clone(ids), consts...)
		}
		part := make([]any, len(rows))
		for i, r := range rows {
			obj := map[string]any{splitKey: number(strconv.Itoa(i + 1))}
			for _, id := range ids {
				if x, has := r[id]; has {
					setPath(obj, paths[id], x)
				}
			}
			part[i] = obj
		}
		out[fmt.Sprintf("part%0*d", digits, g+1)] = part
	}
	return out, true
}

// number returns a number literal as canonical.Parse would.
func number(lit string) any {
	v, _ := canonical.Parse([]byte(lit))
	return v
}

// leafPaths adds m's leaves to out under their path IDs, descending into
// non-empty nested objects.
func leafPaths(prefix []string, m map[string]any, out map[string]any, paths map[string][]string) {
	for k, v := range m {
		p := append(slices.Clip(prefix), k)
		if sub, ok := v.(map[string]any); ok && len(sub) > 0 {
			leafPaths(p, sub, out, paths)
			continue
		}
		id := strings.Join(p, "\x00")
		out[id] = v
		paths[id] = p
	}
}

func isConstant(rows []map[string]any, id string) bool {
	if len(rows) < 3 {
		return false
	}
	first, ok := rows[0][id]
	if !ok {
		return false
	}
	want, _ := canonical.Marshal(first)
	for _, r := range rows[1:] {
		v, ok := r[id]
		if !ok {
			return false
		}
		if b, _ := canonical.Marshal(v); string(b) != string(want) {
			return false
		}
	}
	return true
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
	buckets := make([][]string, len(firsts))
	for i, f := range firsts {
		buckets[i] = byFirst[f]
	}
	var groups [][]string
	var cur []string
	for _, b := range buckets {
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
