package wtq

import (
	"slices"
	"testing"
)

func TestCorrect(t *testing.T) {
	for _, c := range []struct {
		answers []string
		reply   string
		want    bool
	}{
		{[]string{"Italy"}, "Italy", true},
		{[]string{"Italy"}, "italy.", true},
		{[]string{"Italy"}, "**Italy**", true},
		{[]string{"Italy"}, "Answer: Italy", true},
		{[]string{"Italy"}, "<think>maybe Spain</think>Italy", true},
		{[]string{"Italy"}, "The answer is Italy.", true},
		{[]string{"Italy"}, "Spain", false},
		{[]string{"Italy"}, "Italyland", false},
		{[]string{"Italy"}, "It is not entirely clear from the table, but probably Spain or Italy", false},
		{[]string{"1000"}, "1,000", true},
		{[]string{"1000"}, "1000.0", true},
		{[]string{"3"}, "3.0", true},
		{[]string{"3"}, "There are 3 riders.", true},
		{[]string{"3"}, "4", false},
		{[]string{"17 years"}, "17 years", true},
		{[]string{"Café Ñandú"}, "cafe nandu", true},
		{[]string{"Rider 1 (ITA)"}, "Rider 1", true},
		{[]string{"Gold[1]"}, "gold", true},
		{[]string{`"Hello"`}, "hello", true},
		{[]string{"1990–1994"}, "1990-1994", true},
		{[]string{"2001-xx-xx"}, "2001", true},
		{[]string{"A0", "A1"}, "A1, A0", true},
		{[]string{"A0", "A1"}, "A0|A1", true},
		{[]string{"A0", "A1"}, "A0\nA1", true},
		{[]string{"A0", "A1"}, "A0", false},
		{[]string{"A0", "A1"}, "A0, A1, A2", false},
		{[]string{"Italy"}, "", false},
	} {
		if got := Correct(c.answers, c.reply); got != c.want {
			t.Errorf("Correct(%q, %q) = %v, want %v", c.answers, c.reply, got, c.want)
		}
	}
}

func TestCanon(t *testing.T) {
	// The dataset's canonical form makes "100,000" a number.
	it := Item{Answers: []string{"100,000"}, Canon: []string{"100000.0"}}
	for reply, want := range map[string]bool{"100000": true, "100,000": true, "100 000": false, "10000": false} {
		if got := it.Correct(reply); got != want {
			t.Errorf("Correct(%q) = %v, want %v", reply, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"  Hello   World. ":   "hello world",
		"Smith (born 1950)":   "smith",
		"(1950)":              "(1950)",
		"Paris†":              "paris",
		"Paris [a] [12]":      "paris",
		"[12]":                "",
		"[a]":                 "[a]",
		"“Quoted”":            "quoted",
		"Zoë – Müller":        "zoe - muller",
		"a (b (c) d)":         "a (b (c) d)",
		"Team (x) (y)":        "team",
		`"a" and "b"`:         `"a" and "b"`,
		"UCI ProTour\nPoints": "uci protour points",
	} {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoad(t *testing.T) {
	all, err := Load("testdata", 0, 15, 1)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range all {
		ids = append(ids, it.ID)
	}
	// nu-0's table has 3 rows and is filtered out.
	if want := []string{"nu-1", "nu-2", "nu-3", "nu-4", "nu-5"}; !slices.Equal(ids, want) {
		t.Fatalf("ids %v, want %v", ids, want)
	}
	byID := map[string]Item{}
	for _, it := range all {
		byID[it.ID] = it
	}
	if it := byID["nu-3"]; !slices.Equal(it.Columns, []string{"Name", "col1", "Name_2", "Points Total", "Name_3"}) || it.Rows != 16 {
		t.Fatalf("columns %q rows %d", it.Columns, it.Rows)
	}
	if got := byID["nu-3"].Cells[2][4]; got != `C\2` {
		t.Fatalf("unescaped cell %q", got)
	}
	if got := byID["nu-5"].Answers; !slices.Equal(got, []string{"Team |1"}) {
		t.Fatalf("answers %q", got)
	}
	if got := byID["nu-1"].Cells[0][2]; got != "Team |1" {
		t.Fatalf("pipe cell %q", got)
	}
	if !byID["nu-4"].Correct("1200") || !byID["nu-3"].Correct("A1 | A0") {
		t.Fatal("canonical answers not used")
	}

	// Seeded samples are deterministic, sorted by ID and vary with the seed.
	a, _ := Load("testdata", 3, 15, 7)
	b, _ := Load("testdata", 3, 15, 7)
	if len(a) != 3 || !slices.EqualFunc(a, b, func(x, y Item) bool { return x.ID == y.ID }) {
		t.Fatal("sample not deterministic")
	}
	if !slices.IsSortedFunc(a, func(x, y Item) int { return cmpID(x, y) }) {
		t.Fatal("sample not sorted by ID")
	}
	differs := false
	for seed := uint64(1); seed < 20 && !differs; seed++ {
		c, _ := Load("testdata", 3, 15, seed)
		differs = !slices.EqualFunc(a, c, func(x, y Item) bool { return x.ID == y.ID })
	}
	if !differs {
		t.Fatal("sample ignores the seed")
	}
}

func cmpID(x, y Item) int {
	switch {
	case x.ID < y.ID:
		return -1
	case x.ID > y.ID:
		return 1
	}
	return 0
}

func TestJSON(t *testing.T) {
	it := Item{
		Columns: []string{"Zeta", "Alpha", "Q"},
		Cells:   [][]string{{"1", "<b>&", `say "hi"` + "\n"}},
	}
	want := `[{"Zeta":"1","Alpha":"<b>&","Q":"say \"hi\"\n"}]`
	if got := string(it.JSON()); got != want {
		t.Fatalf("JSON\n got %s\nwant %s", got, want)
	}
}
