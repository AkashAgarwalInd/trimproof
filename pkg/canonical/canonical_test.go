package canonical

import (
	"testing"
)

func TestCanonicalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"b":1, "a":[true,false,null]}`, `{"a":[true,false,null],"b":1}`},
		{`{"z":{"y":1,"x":2}}`, `{"z":{"x":2,"y":1}}`},
		{`"<a>&b"`, `"<a>&b"`},
		{`" \u0001\/"`, "\" \\u0001/\""},
		{` [ ] `, `[]`},
		{`{}`, `{}`},
		{`"é\"\\"`, `"é\"\\"`},
	}
	for _, c := range cases {
		got, err := Canonicalize([]byte(c.in))
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if string(got) != c.want {
			t.Errorf("Canonicalize(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

// Numbers keep their exact text, compared as raw bytes.
func TestNumberTextPreserved(t *testing.T) {
	for _, n := range []string{
		"9007199254740993", "-0", "88.0", "1e400", "0.1000",
		"123456789012345678901234567890", "1E+2", "-1.5e-10",
	} {
		got, err := Canonicalize([]byte(`[` + n + `]`))
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		if string(got) != "["+n+"]" {
			t.Errorf("number %s became %s", n, got)
		}
	}
}

func TestRejects(t *testing.T) {
	for _, in := range []string{
		`{"a":1,"a":2}`, `[1] [2]`, `[1,]`, `{"a":1}x`, "\"\xff\"", ``, `01`, `NaN`,
	} {
		if _, err := Canonicalize([]byte(in)); err == nil {
			t.Errorf("Canonicalize(%q) accepted invalid input", in)
		}
	}
}

func TestDepthLimit(t *testing.T) {
	deep := make([]byte, 0, 2*(MaxDepth+2))
	for i := 0; i < MaxDepth+1; i++ {
		deep = append(deep, '[')
	}
	for i := 0; i < MaxDepth+1; i++ {
		deep = append(deep, ']')
	}
	if _, err := Canonicalize(deep); err == nil {
		t.Error("expected depth error")
	}
}

func FuzzIdempotent(f *testing.F) {
	for _, s := range []string{`{"b":1,"a":"x"}`, `[1.0,-0,1e400]`, `"\u0000"`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		c1, err := Canonicalize(data)
		if err != nil {
			return
		}
		c2, err := Canonicalize(c1)
		if err != nil {
			t.Fatalf("canonical output does not reparse: %s: %v", c1, err)
		}
		if string(c1) != string(c2) {
			t.Fatalf("not idempotent: %s vs %s", c1, c2)
		}
	})
}
