package toonx

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
)

func wideRows(n int) string {
	var rows []string
	for i := range n {
		rows = append(rows, fmt.Sprintf(`{"id":%d,"name":"n%d","a":%d,"b":%d,"c":%d,"d":%d,"e":%d,"same":1,`+
			`"user":{"login":"u%d","id":%d,"x":%d,"y":%d},"r":{"heart":%d,"eyes":%d,"z":null}}`,
			10+i, i, i, i, i, i, i, i, i, i, i, i, i))
	}
	return "[" + strings.Join(rows, ",") + "]"
}

func TestSplitWide(t *testing.T) {
	in := `{"total":4,"items":` + wideRows(4) + `}`
	enc := encode(t, in) // Encode fails unless the round trip is exact
	want := `items:
  part1[4]{"#id",a,b,c,d,e,name}:
    all rows: r.z=null,same=1
    10,0,0,0,0,0,n0
    11,1,1,1,1,1,n1
    12,2,2,2,2,2,n2
    13,3,3,3,3,3,n3
  part2[4]{"#id",r.eyes,r.heart,user.id,user.login,user.x,user.y}:
    10,0,0,0,u0,0,0
    11,1,1,1,u1,1,1
    12,2,2,2,u2,2,2
    13,3,3,3,u3,3,3
total: 4`
	if enc != want {
		t.Fatalf("got\n%s\nwant\n%s", enc, want)
	}
	// Version 3: parts start with the row number, and id is a column.
	v3 := Codec{NoRowKey: true, NoMark: true}
	old, err := v3.Encode([]byte(in), codec.Options{})
	if err != nil || !strings.Contains(string(old), `part1[4]{"#",a,b,c,d,e,id,name}:`) || !strings.Contains(string(old), "\n    4,3,3,3,3,3,13,n3\n") {
		t.Fatalf("version 3:\n%s\n%v", old, err)
	}
	if got := v3.PrimerFor([][]byte{old}); !strings.HasSuffix(got, primerSplit) {
		t.Fatalf("version 3 primer:\n%s", got)
	}
	if got := (Codec{}).PrimerFor([][]byte{[]byte(enc)}); !strings.HasSuffix(got, primerKeyed) {
		t.Fatalf("primer lacks the split sentence:\n%s", got)
	}
	whole, err := Codec{NoSplit: true}.Encode([]byte(in), codec.Options{})
	if err != nil || strings.Contains(string(whole), "part1") {
		t.Fatalf("NoSplit:\n%s\n%v", whole, err)
	}
}

func TestSplitNarrow(t *testing.T) {
	// 8 varying columns stay one table; 9 are split.
	row := func(n, i int) string {
		var f []string
		for c := range n {
			f = append(f, fmt.Sprintf(`"c%d":%d`, c, i))
		}
		return "{" + strings.Join(f, ",") + "}"
	}
	for n, wantSplit := range map[int]bool{8: false, 9: true} {
		enc := encode(t, "["+row(n, 1)+","+row(n, 2)+","+row(n, 3)+"]")
		if strings.Contains(enc, "part1[3]") != wantSplit {
			t.Errorf("%d columns:\n%s", n, enc)
		}
	}
}

// Random wide tables must round-trip, including absent fields, nested
// objects that span parts, and rows that miss a whole part.
func TestSplitRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	vals := []any{nil, true, "x", "a,b", "", canonicalNumber("3"), []any{}, map[string]any{}, "https://x.io/u/5"}
	splits := 0
	for trial := 0; trial < 1000; trial++ {
		var rows []any
		for range 2 + r.IntN(5) {
			row := map[string]any{}
			for c := range 3 + r.IntN(12) {
				if r.IntN(4) == 0 {
					continue
				}
				v := vals[r.IntN(len(vals))]
				switch r.IntN(3) {
				case 0:
					row[fmt.Sprintf("f%d", c)] = v
				case 1:
					sub, _ := row["u"].(map[string]any)
					if sub == nil {
						sub = map[string]any{}
						row["u"] = sub
					}
					sub[fmt.Sprintf("g%d", c)] = v
				default:
					row[fmt.Sprintf("n%d", c)] = map[string]any{"v": v, "w": canonicalNumber(fmt.Sprint(c))}
				}
			}
			if len(row) == 0 {
				row["k"] = "v"
			}
			switch trial % 4 { // row keys: unique, repeated, mixed types
			case 0:
				row["id"] = canonicalNumber(fmt.Sprint(len(rows) * 7))
			case 1:
				row["user_id"] = canonicalNumber(fmt.Sprint(len(rows) % 2))
				row["name"] = fmt.Sprintf("n%d", len(rows))
			case 2:
				row["key"] = vals[r.IntN(len(vals))]
			}
			rows = append(rows, row)
		}
		js, _ := canonical.Marshal(map[string]any{"items": rows})
		enc, err := Codec{}.Encode(js, codec.Options{})
		if err != nil {
			t.Fatalf("Encode(%s): %v", js, err)
		}
		if strings.Contains(string(enc), "part1[") {
			splits++
		}
	}
	if splits < 100 {
		t.Fatalf("only %d of 1000 tables were split", splits)
	}
}

// Data that already looks like split parts would be joined on decode, so
// Encode must refuse it rather than change it.
func TestSplitLookalike(t *testing.T) {
	in := `{"part1":[{"#":1,"a":1},{"#":2,"a":2}],"part2":[{"#":1,"b":1},{"#":2,"b":2}]}`
	if _, err := (Codec{}).Encode([]byte(in), codec.Options{}); err == nil {
		t.Fatal("encoded data that decodes as split parts")
	}
}

func TestSplitDecodeRejects(t *testing.T) {
	for _, doc := range []string{
		"part1[2]{\"#\",a}:\n  1,1\n  2,2\npart2[2]{\"#\",a}:\n  1,3\n  2,4", // same field in two parts
		"part1[2]{\"#\"}:\n  1\n  2\npart2[2]{\"#\"}:\n  1\n  2",             // rows of only "#"
	} {
		if got, err := (Codec{}).Decode([]byte(doc)); err == nil {
			t.Errorf("decoded %q as %s", doc, got)
		}
	}
	// Not split parts: kept as they are.
	for _, doc := range []string{
		"part1[2]{\"#\",a}:\n  1,1\n  3,2\npart2[2]{\"#\",b}:\n  1,1\n  2,2", // numbering off
		"part1[2]{\"#\",a}:\n  1,1\n  2,2\npart3[2]{\"#\",b}:\n  1,1\n  2,2", // no part2
	} {
		got, err := (Codec{}).Decode([]byte(doc))
		if err != nil || !strings.Contains(string(got), `"part1"`) {
			t.Errorf("%q: %s %v", doc, got, err)
		}
	}
}

// An "all rows:" object in one part is shared by every decoded row; joining
// another part's fields into it must not leak between rows.
func TestSplitJoinSharedConstant(t *testing.T) {
	doc := "part1[3]{\"#\",a}:\n  all rows: u={\"k\":1}\n  1,1\n  2,2\n  3,3\npart2[3]{\"#\",u.x}:\n  1,7\n  2,8\n  3,9"
	got, err := (Codec{}).Decode([]byte(doc))
	want := `[{"a":1,"u":{"k":1,"x":7}},{"a":2,"u":{"k":1,"x":8}},{"a":3,"u":{"k":1,"x":9}}]`
	if err != nil || string(got) != want {
		t.Fatalf("got %s %v\nwant %s", got, err, want)
	}
}

func TestRowKey(t *testing.T) {
	wide := func(extra func(i int) string) string {
		var rows []string
		for i := range 3 {
			rows = append(rows, fmt.Sprintf(`{%s"c0":%d,"c1":%d,"c2":%d,"c3":%d,"c4":%d,"c5":%d,"c6":%d,"c7":%d,"c8":%d}`,
				extra(i), i, i, i, i, i, i, i, i, i))
		}
		return "[" + strings.Join(rows, ",") + "]"
	}
	for _, c := range []struct {
		extra func(i int) string
		lead  string
	}{
		{func(i int) string { return fmt.Sprintf(`"name":"n%d","id":"x%d",`, i, i) }, `"#id"`},        // id beats name
		{func(i int) string { return fmt.Sprintf(`"name":"n%d","user_id":%d,`, i, i) }, `"#user_id"`}, // ends in id beats name
		{func(i int) string { return fmt.Sprintf(`"title":"t%d","name":"n%d",`, i, i) }, `"#name"`},
		{func(i int) string { return `"id":1,"name":"same",` }, `"#"`},                    // both repeat
		{func(i int) string { return fmt.Sprintf(`"title":"t%d","x":%d,`, i, i) }, `"#"`}, // no key-like field
	} {
		enc := encode(t, wide(c.extra))
		if !strings.Contains(enc, "part1[3]{"+c.lead+",") {
			t.Errorf("want lead %s:\n%s", c.lead, enc)
		}
	}
}

func TestKeyedDecode(t *testing.T) {
	ok := "part1[2]{\"#id\",a}:\n  7,1\n  9,2\npart2[2]{\"#id\",b}:\n  7,3\n  9,4"
	got, err := (Codec{}).Decode([]byte(ok))
	if want := `[{"a":1,"b":3,"id":7},{"a":2,"b":4,"id":9}]`; err != nil || string(got) != want {
		t.Fatalf("got %s %v, want %s", got, err, want)
	}
	// Not split parts: kept as they are.
	for _, doc := range []string{
		"part1[2]{\"#id\",a}:\n  7,1\n  9,2\npart2[2]{\"#id\",b}:\n  9,3\n  7,4", // keys differ between parts
		"part1[2]{\"#id\",a}:\n  7,1\n  7,2\npart2[2]{\"#id\",b}:\n  7,3\n  7,4", // key repeats
		"part1[2]{\"#id\",a}:\n  7,1\n  9,2\npart2[2]{\"#k\",b}:\n  7,3\n  9,4",  // different lead columns
	} {
		got, err := (Codec{}).Decode([]byte(doc))
		if err != nil || !strings.Contains(string(got), `"part1"`) {
			t.Errorf("%q: %s %v", doc, got, err)
		}
	}
	// The key field may not also be a column.
	bad := "part1[2]{\"#id\",id}:\n  7,1\n  9,2\npart2[2]{\"#id\",b}:\n  7,3\n  9,4"
	if got, err := (Codec{}).Decode([]byte(bad)); err == nil {
		t.Errorf("decoded %s", got)
	}
}

func TestMark(t *testing.T) {
	doc := "[3]{u}:\n  starts with: u=\"https://x.io/\"\n  ~a\n  b\n  ~c"
	if got, err := (Codec{}).Decode([]byte(doc)); err == nil {
		t.Errorf("decoded a prefixed value without ~: %s", got)
	}
	got, err := Codec{NoMark: true}.Decode([]byte(doc))
	if want := `[{"u":"https://x.io/~a"},{"u":"https://x.io/b"},{"u":"https://x.io/~c"}]`; err != nil || string(got) != want {
		t.Errorf("NoMark: %s %v", got, err)
	}
}
