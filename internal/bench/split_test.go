package bench

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec/toonx"
)

// unsplit merges every {"part1":…} object back into one array of rows.
func unsplit(v any) any {
	switch t := v.(type) {
	case map[string]any:
		if _, ok := t["part1"]; ok || t["part01"] != nil {
			var rows []map[string]any
			for _, k := range canonical.SortedKeys(t) {
				for i, x := range t[k].([]any) {
					if len(rows) <= i {
						rows = append(rows, map[string]any{})
					}
					merge(rows[i], x.(map[string]any))
				}
			}
			out := make([]any, len(rows))
			for i, r := range rows {
				delete(r, splitKey)
				out[i] = r
			}
			return out
		}
		m := map[string]any{}
		for k, x := range t {
			m[k] = unsplit(x)
		}
		return m
	case []any:
		a := make([]any, len(t))
		for i, x := range t {
			a[i] = unsplit(x)
		}
		return a
	}
	return v
}

func merge(dst, src map[string]any) map[string]any {
	for k, v := range src {
		if sub, ok := v.(map[string]any); ok {
			if d, ok := dst[k].(map[string]any); ok {
				merge(d, sub)
				continue
			}
		}
		if sub, ok := v.(map[string]any); ok {
			v = merge(map[string]any{}, sub) // copy: never alias the input
		}
		dst[k] = v
	}
	return dst
}

func TestSplitWide(t *testing.T) {
	var rows []string
	for i := range 4 {
		rows = append(rows, fmt.Sprintf(`{"id":%d,"name":"n%d","a":%d,"b":%d,"c":%d,"d":%d,"e":%d,"same":1,`+
			`"user":{"login":"u%d","id":%d,"x":%d,"y":%d},"r":{"heart":%d,"eyes":%d,"z":null}}`,
			10+i, i, i, i, i, i, i, i, i, i, i, i, i))
	}
	raw := `{"total":4,"items":[` + strings.Join(rows, ",") + `]}`
	v, err := canonical.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	sv, ok := splitWide(v)
	if !ok {
		t.Fatal("not split")
	}
	want, _ := canonical.Marshal(v)
	got, _ := canonical.Marshal(unsplit(sv))
	if string(got) != string(want) {
		t.Fatalf("not lossless:\n%s\n%s", got, want)
	}
	b, _ := canonical.Marshal(sv)
	enc, err := toonx.Codec{}.Encode(b, codec.Options{Mode: codec.Strict})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(enc), "\n") {
		if h, _, ok := strings.Cut(l, "]{"); ok && strings.Count(l, ",")+1 > splitWidth {
			t.Errorf("table %s wider than %d: %s", h, splitWidth, l)
		}
		if strings.Contains(l, "]{") && !strings.Contains(l, `]{"#",`) && !strings.Contains(l, "]{#,") {
			t.Errorf("table does not start with the key: %s", l)
		}
	}
	if !strings.Contains(string(enc), ",same=1") {
		t.Errorf("constant not hoisted:\n%s", enc)
	}
}

func TestSplitNarrowAndRowKey(t *testing.T) {
	v, _ := canonical.Parse([]byte(`[{"a":1,"b":2},{"a":3,"b":4}]`))
	if _, ok := splitWide(v); ok {
		t.Fatal("split a narrow table")
	}
	var rows []string
	for i := range 3 {
		rows = append(rows, fmt.Sprintf(`{"c0":%d,"c1":%d,"c2":%d,"c3":%d,"c4":%d,"c5":%d,"c6":%d,"c7":%d,"c8":%d}`, i, i, i, i, i, i, i, i, i))
	}
	v, _ = canonical.Parse([]byte("[" + strings.Join(rows, ",") + "]"))
	sv, ok := splitWide(v)
	if !ok {
		t.Fatal("not split")
	}
	parts := sv.(map[string]any)
	keys := canonical.SortedKeys(parts)
	if !slices.Equal(keys, []string{"part1", "part2"}) {
		t.Fatalf("parts %v", keys)
	}
	first := parts["part2"].([]any)[2].(map[string]any)
	if b, _ := canonical.Marshal(first[splitKey]); string(b) != "3" { // row 3
		t.Fatalf("row key %s, want 3", b)
	}
	want, _ := canonical.Marshal(v)
	got, _ := canonical.Marshal(unsplit(sv))
	if string(got) != string(want) {
		t.Fatalf("not lossless:\n%s\n%s", got, want)
	}
}
