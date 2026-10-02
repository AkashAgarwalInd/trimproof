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
  part1[4]{"#",a,b,c,d,e,id,name}:
    all rows: r.z=null,same=1
    1,0,0,0,0,0,10,n0
    2,1,1,1,1,1,11,n1
    3,2,2,2,2,2,12,n2
    4,3,3,3,3,3,13,n3
  part2[4]{"#",r.eyes,r.heart,user.id,user.login,user.x,user.y}:
    1,0,0,0,u0,0,0
    2,1,1,1,u1,1,1
    3,2,2,2,u2,2,2
    4,3,3,3,u3,3,3
total: 4`
	if enc != want {
		t.Fatalf("got\n%s\nwant\n%s", enc, want)
	}
	if got := (Codec{}).PrimerFor([][]byte{[]byte(enc)}); !strings.HasSuffix(got, primerSplit) {
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
