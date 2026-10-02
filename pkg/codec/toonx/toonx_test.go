package toonx

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec/toon"
	toongo "github.com/toon-format/toon-go"
)

func encode(t *testing.T, in string) string {
	t.Helper()
	enc, err := Codec{}.Encode([]byte(in), codec.Options{})
	if err != nil {
		t.Fatalf("Encode(%s): %v", in, err)
	}
	return string(enc)
}

func TestGolden(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		// Flat uniform rows: plain TOON.
		{`[{"id":1,"name":"a"},{"id":2,"name":"b, c"}]`, "[2]{id,name}:\n  1,a\n  2,\"b, c\""},
		// A wrapper object around a table.
		{`{"items":[{"id":1},{"id":2}],"total":2}`, "items[2]{id}:\n  1\n  2\ntotal: 2"},
		// Missing keys are empty cells; "" is a value.
		{`[{"a":1,"b":""},{"a":2}]`, "[2]{a,b}:\n  1,\"\"\n  2,"},
		// Nested objects become path columns when that is shorter; arrays
		// and {} stay JSON.
		{`[{"id":1,"tags":["x"],"user":{"login":"u","site":{}}},{"id":2,"tags":[],"user":{"login":"v","site":{}}},{"id":3,"tags":[],"user":null}]`,
			"[3]{id,tags,user,user.login,user.site}:\n  1,[\"x\"],,u,{}\n  2,[],,v,{}\n  3,[],null,,"},
		// Otherwise nested objects stay JSON cells.
		{`[{"id":1,"user":{"login":"u"}},{"id":2,"user":null}]`, "[2]{id,user}:\n  1,{\"login\":\"u\"}\n  2,null"},
		// A key with a dot is quoted inside a header, bare outside.
		{`{"a.b":[{"c.d":1,"e":{"f":2}}]}`, "a.b[1]{\"c.d\",e.f}:\n  1,2"},
		// Numbers keep their literal text.
		{`[{"n":88.0,"big":9007199254740993,"e":1e400}]`, "[1]{big,e,n}:\n  9007199254740993,1e400,88.0"},
		// Mixed arrays are lists with JSON items.
		{`{"x":[1,{"a":1},[2]],"y":[],"z":{}}`, "x[3]:\n  - 1\n  - {\"a\":1}\n  - [2]\ny[0]:\nz: {}"},
		// A field with one value in every row is written once.
		{`[{"id":1,"admin":false,"u":{"t":"User"}},{"id":2,"admin":false,"u":{"t":"User"}},{"id":3,"admin":false,"u":{"t":"User"}}]`,
			"[3]{id}:\n  all rows: admin=false,u.t=User\n  1\n  2\n  3"},
		// A shared URL start is written once, cut at a '/'; null stays null.
		{`[{"id":1,"url":"https://api.github.com/users/ann"},{"id":2,"url":"https://api.github.com/users/bo"},{"id":3,"url":null},{"id":4,"url":"https://api.github.com/users/"}]`,
			"[4]{id,url}:\n  starts with: url=\"https://api.github.com/users/\"\n  1,ann\n  2,bo\n  3,null\n  4,\"\""},
		// A rest of only digits would read as a number: cut one '/' earlier.
		{`[{"id":1,"u":"https://api.tvmaze.com/shows/266"},{"id":2,"u":"https://api.tvmaze.com/shows/7"},{"id":3,"u":"https://api.tvmaze.com/shows/8"}]`,
			"[3]{id,u}:\n  starts with: u=\"https://api.tvmaze.com/\"\n  1,shows/266\n  2,shows/7\n  3,shows/8"},
		// Fewer than 3 rows, or no gain: nothing is factored.
		{`[{"a":"same","id":1},{"a":"same","id":2}]`, "[2]{a,id}:\n  same,1\n  same,2"},
		{`[{"u":"https://a.io/x","v":"http://x"},{"u":"https://b.io/x","v":"http://y"},{"u":"https://c.io/x","v":"http://z"}]`,
			"[3]{u,v}:\n  \"https://a.io/x\",\"http://x\"\n  \"https://b.io/x\",\"http://y\"\n  \"https://c.io/x\",\"http://z\""},
	} {
		if got := encode(t, c.in); got != c.want {
			t.Errorf("Encode(%s) =\n%s\nwant\n%s", c.in, got, c.want)
		}
	}
}

// On a flat array of uniform objects with no constant field (and no URLs,
// which the generator never makes) toonx writes exactly what TOON writes, so
// results measured with TOON on such data carry over.
func TestFlatTablesMatchTOON(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	strs := []string{"a", "b c", "x,y", "", " pad", "-1", "true", "07", "null", "日本", "q\"t", "a:b", "[x]", "line\nbreak"}
	nums := []string{"0", "1", "-3", "2.5", "1000000", "0.125"}
	for trial := 0; trial < 500; trial++ {
		keys := []string{"id", "name", "price", "ok", "Note_2"}[:1+r.IntN(5)]
		var in []any
		for range 1 + r.IntN(6) {
			row := map[string]any{}
			for _, k := range keys {
				switch r.IntN(4) {
				case 0:
					row[k] = strs[r.IntN(len(strs))]
				case 1:
					row[k] = canonicalNumber(nums[r.IntN(len(nums))])
				case 2:
					row[k] = r.IntN(2) == 0
				default:
					row[k] = nil
				}
			}
			in = append(in, row)
		}
		js, _ := canonical.Marshal(in)
		if hasConstant(in) {
			continue
		}
		want, err := toongo.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := encode(t, string(js)); got != string(want) {
			t.Fatalf("%s\ntoonx:\n%s\ntoon:\n%s", js, got, want)
		}
	}
}

// hasConstant reports whether rows has minFactorRows rows and a field with
// the same value in all of them.
func hasConstant(rows []any) bool {
	if len(rows) < minFactorRows {
		return false
	}
	for k, v := range rows[0].(map[string]any) {
		want, _ := canonical.Marshal(v)
		same := true
		for _, r := range rows[1:] {
			if b, _ := canonical.Marshal(r.(map[string]any)[k]); string(b) != string(want) {
				same = false
			}
		}
		if same {
			return true
		}
	}
	return false
}

func canonicalNumber(s string) any {
	v, _ := canonical.Parse([]byte(s))
	return v
}

func TestEligibility(t *testing.T) {
	for _, in := range []string{`[]`, `{}`, `1`, `"s"`, `null`} {
		v, _ := canonical.Parse([]byte(in))
		if err := (Codec{}).Check(v, codec.Options{}); !errors.Is(err, codec.ErrIneligible) {
			t.Errorf("Check(%s) = %v, want ineligible", in, err)
		}
	}
	// Shapes TOON cannot encode, or not losslessly.
	for _, in := range []string{
		`[{"a":1},{"b":2}]`, `[{"nested":{"x":1}}]`, `[{"a":1},{"a":"x"}]`, `[{"ctl":"\u0001"}]`,
		`[{"id":9007199254740993,"neg":-0,"f":88.0,"big":1e400}]`, `[{"a":[]},{"a":{}}]`,
		`{"a":{"b":{"c":[{"d":{"e":1}},{"d":null}]}}}`, `[[1,2],[3]]`, `[{},{"a":1}]`,
		`[{"":1,"x y":{"":2}}]`,
	} {
		encode(t, in)
	}
}

func TestDecodeRejects(t *testing.T) {
	for _, in := range []string{
		"",
		"a: 1\na: 2",                    // duplicate key
		"[2]{a}:\n  1",                  // too few rows
		"[1]{a,a}:\n  1,2",              // duplicate column
		"[1]{a,a.b}:\n  1,2",            // value and path under it
		"[1]{a}:\n  1,2",                // arity
		"[1]{a}:\n  ",                   // empty row
		"a: \"x\"",                      // needless quotes
		"a: 1,2",                        // unquoted comma
		"a: -x",                         // unquoted dash
		"[1]{a}:\n  {\"b\": 1}",         // non-canonical JSON cell
		"[2]: 1,",                       // empty primitive element
		"[1]: [1]",                      // container in primitive array
		"a:\nb: 1",                      // empty nested object
		"a[1]:\n  1",                    // list item without dash
		"a: 1\n  b: 2",                  // stray indentation
		"[01]: 1",                       // bad length
		"[1]{\"a\"}:\n  1",              // needlessly quoted column
		"[1]{a}:\n  all rows: a=1\n  2", // constant and column conflict
		"[1]{a}:\n  all rows: b\n  2",   // no '='
		"[1]{a}:\n  all rows: b=\n  2",  // empty value
		"[1]{a}:\n  starts with: b=\"http://x/\"\n  y",   // prefix of no column
		"[1]{a}:\n  starts with: a=1\n  2",               // prefix not a string
		"[1]{a}:\n  starts with: a=\"h/\"\n  1",          // number in a prefixed column
		"[1]{a}:\n  starts with: a=\"h/\",a=\"g/\"\n  x", // prefix given twice
	} {
		if out, err := (Codec{}).Decode([]byte(in)); err == nil {
			t.Errorf("Decode(%q) accepted: %s", in, out)
		}
	}
}

// Random nested documents must round-trip exactly or be ineligible; with
// toonx no generated document should be ineligible.
func TestRandomRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for trial := 0; trial < 3000; trial++ {
		v := randValue(r, 0)
		if m, ok := v.(map[string]any); !ok || len(m) == 0 {
			v = map[string]any{"root": v}
		}
		js, _ := canonical.Marshal(v)
		if _, err := (Codec{}).Encode(js, codec.Options{}); err != nil {
			t.Fatalf("Encode(%s): %v", js, err)
		}
	}
}

func randValue(r *rand.Rand, depth int) any {
	keys := []string{"a", "b", "a.b", "", "x y", "_id", "c", "-k", "日"}
	scalars := []any{nil, true, false, "", "s", "a,b", " x", "-", "1", "null", "{", "q\"\\", "\t", "\u0001",
		canonicalNumber("0"), canonicalNumber("-1.50"), canonicalNumber("1e3")}
	if depth > 3 || r.IntN(3) == 0 {
		return scalars[r.IntN(len(scalars))]
	}
	if r.IntN(2) == 0 {
		var a []any
		obj := r.IntN(2) == 0
		for range r.IntN(4) {
			if obj {
				m := map[string]any{}
				for range r.IntN(4) {
					m[keys[r.IntN(len(keys))]] = randValue(r, depth+1)
				}
				a = append(a, m)
			} else {
				a = append(a, randValue(r, depth+1))
			}
		}
		if a == nil {
			a = []any{}
		}
		return a
	}
	m := map[string]any{}
	for range r.IntN(4) {
		m[keys[r.IntN(len(keys))]] = randValue(r, depth+1)
	}
	return m
}

func ExampleCodec_Encode() {
	enc, _ := Codec{}.Encode([]byte(`{"total":2,"items":[{"id":1,"user":{"login":"ann"}},{"id":2,"user":{"login":"bo"},"draft":true}]}`), codec.Options{})
	fmt.Println(string(enc))
	// Output:
	// items[2]{draft,id,user.login}:
	//   ,1,ann
	//   true,2,bo
	// total: 2
}

func TestPrimerFor(t *testing.T) {
	base := toon.Codec{}.Primer()
	flat := encode(t, `[{"a":1,"b":"x"},{"a":2,"b":"y"}]`)
	if got := (Codec{}).PrimerFor([][]byte{[]byte(flat)}); got != base {
		t.Fatalf("flat table primer differs from TOON's:\n%s", got)
	}
	nested := encode(t, `[{"a":1,"u":{"x":1,"y":2}},{"u":{"x":3,"y":4},"t":[1]},{"u":{"x":5,"y":6}}]`)
	got := (Codec{}).PrimerFor([][]byte{[]byte(flat), []byte(nested)})
	if got != base+primerAbsent+primerPaths+primerJSON {
		t.Fatalf("primer:\n%s\nfor\n%s", got, nested)
	}
	factored := encode(t, `[{"a":"constant","u":"https://example.com/users/1"},{"a":"constant","u":"https://example.com/users/2"},{"a":"constant","u":"https://example.com/users/3"}]`)
	if got := (Codec{}).PrimerFor([][]byte{[]byte(factored)}); got != base+primerConst+primerPrefix {
		t.Fatalf("factored primer:\n%s\nfor\n%s", got, factored)
	}
	list := encode(t, `{"x":[1,[2]]}`)
	if got := (Codec{}).PrimerFor([][]byte{[]byte(list)}); got != base+primerJSON+primerList {
		t.Fatalf("list primer:\n%s", got)
	}
	if (Codec{}).Primer() != base+primerAbsent+primerPaths+primerJSON+primerList+primerConst+primerPrefix+primerSplit {
		t.Fatal("full primer must describe every extension")
	}
	if (Codec{RowKey: true, Mark: true}).Primer() != base+primerAbsent+primerPaths+primerJSON+primerList+primerConst+primerMark+primerSplit+primerKeyed {
		t.Fatal("full primer must describe every extension")
	}
}

// BenchmarkEncode measures Encode (both table layouts plus the verifying
// decode) and PrimerFor on ~30 KB of nested API-like rows.
func BenchmarkEncode(b *testing.B) {
	var rows []any
	for i := 0; len(rows) < 200; i++ {
		rows = append(rows, map[string]any{
			"id": canonicalNumber(fmt.Sprint(1000 + i)), "title": fmt.Sprintf("Issue number %d, with a comma", i),
			"state": []string{"open", "closed"}[i%2], "labels": []any{map[string]any{"name": "bug"}},
			"user": map[string]any{"login": fmt.Sprintf("user%d", i%17), "id": canonicalNumber(fmt.Sprint(i * 7)), "site_admin": false},
		})
	}
	js, _ := canonical.Marshal(map[string]any{"total_count": canonicalNumber("200"), "items": rows})
	b.SetBytes(int64(len(js)))
	b.ResetTimer()
	for range b.N {
		enc, err := Codec{}.Encode(js, codec.Options{})
		if err != nil {
			b.Fatal(err)
		}
		_ = Codec{}.PrimerFor([][]byte{enc})
	}
}

// Random tables with constant fields and URL columns must round-trip, and
// both factorings must occur.
func TestFactoringRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	urls := []string{"https://api.x.io/u/", "https://api.x.io/u/1", "https://api.x.io/u/a,b", "https://api.x.io/u/x:y",
		"https://api.x.io/u/null", "https://api.x.io/u/-q", "https://api.x.io/u/07", "https://api.x.io/u/ s", "https://api.x.io/v/2"}
	vals := []any{nil, false, "User", "a,b", "", canonicalNumber("3"), []any{}, map[string]any{}}
	consts, prefixes := 0, 0
	for trial := 0; trial < 2000; trial++ {
		c := vals[r.IntN(len(vals))]
		var rows []any
		for range 1 + r.IntN(8) {
			row := map[string]any{"id": canonicalNumber(fmt.Sprint(r.IntN(100)))}
			if r.IntN(8) > 0 {
				row["c"] = c
			}
			switch r.IntN(5) {
			case 0:
			case 1:
				row["url"] = nil
			default:
				row["url"] = urls[r.IntN(len(urls))]
			}
			if r.IntN(2) == 0 {
				row["u"] = map[string]any{"c": c, "html": urls[r.IntN(len(urls))]}
			}
			rows = append(rows, row)
		}
		js, _ := canonical.Marshal(map[string]any{"items": rows})
		enc := encode(t, string(js))
		if strings.Contains(enc, "\n  all rows: ") {
			consts++
		}
		if strings.Contains(enc, "\n  starts with: ") {
			prefixes++
		}
	}
	if consts < 100 || prefixes < 100 {
		t.Fatalf("factored %d tables by constants and %d by prefix; want both often", consts, prefixes)
	}
}

// NoFactor writes version 1's output: no "all rows:" or "starts with:" line.
func TestNoFactor(t *testing.T) {
	in := `[{"id":1,"admin":false,"url":"https://x.io/u/a"},{"id":2,"admin":false,"url":"https://x.io/u/b"},{"id":3,"admin":false,"url":"https://x.io/u/c"}]`
	enc, err := Codec{NoFactor: true}.Encode([]byte(in), codec.Options{})
	if want := "[3]{admin,id,url}:\n  false,1,\"https://x.io/u/a\"\n  false,2,\"https://x.io/u/b\"\n  false,3,\"https://x.io/u/c\""; err != nil || string(enc) != want {
		t.Fatalf("NoFactor encode = %q, %v", enc, err)
	}
	if encode(t, in) == string(enc) {
		t.Fatal("the default codec should factor this table")
	}
}
