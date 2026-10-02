package bench

import "testing"

func TestCorrect(t *testing.T) {
	num := Question{Answer: "1234.5", Numeric: true}
	str := Question{Answer: "Acme Corp"}
	cases := []struct {
		q     Question
		reply string
		want  bool
	}{
		{num, "1234.5", true},
		{num, "1,234.50", true},
		{num, "**$1234.5**", true},
		{num, "<think>maybe 12</think>1234.5", true},
		{num, "Order 10012: 1234.5", true},
		{num, "1234.6", false},
		{num, "", false},
		{str, "acme corp.", true},
		{str, "\"Acme Corp\"", true},
		{str, "The customer is Acme Corp", true},
		{str, "Globex Corp", false},
		{Question{Answer: "7", Numeric: true}, "Reasoning about rows...\nAnswer: 7", true},
	}
	for _, c := range cases {
		if got := Correct(c.q, c.reply); got != c.want {
			t.Errorf("Correct(%q, %q) = %v, want %v", c.q.Answer, c.reply, got, c.want)
		}
	}
}

func TestDatasetsRenderInEveryFormat(t *testing.T) {
	for _, d := range Generate([]string{"orders", "logs", "search", "employees"}, []int{30, 120}, 42) {
		if len(d.Questions) < 4 {
			t.Errorf("%s/%d: only %d questions", d.Name, d.Rows, len(d.Questions))
		}
		for _, fm := range append(Formats, "json-compact-2") {
			if _, _, err := Render(fm, d); err != nil {
				t.Errorf("%s/%d %s: %v", d.Name, d.Rows, fm, err)
			}
		}
	}
}

func TestSampleQuestions(t *testing.T) {
	ds := []*Dataset{
		{Name: "a", Questions: []Question{{ID: "a1", Kind: "x"}, {ID: "a2", Kind: "x"}, {ID: "a3", Kind: "y"}}},
		{Name: "b", Questions: []Question{{ID: "b1", Kind: "x"}, {ID: "b2", Kind: "z"}}},
	}
	got := SampleQuestions(ds, 3, 7)
	kinds := map[string]bool{}
	n := 0
	for _, d := range got {
		for _, q := range d.Questions {
			kinds[q.Kind] = true
			n++
		}
	}
	if n != 3 || len(kinds) != 3 {
		t.Fatalf("sample of 3 = %d questions over %d kinds, want 3 over 3", n, len(kinds))
	}
	if len(ds[0].Questions) != 3 || len(SampleQuestions(ds, 0, 7)) != 2 {
		t.Fatal("sampling must not change its input, and 0 keeps all")
	}
	again := SampleQuestions(ds, 3, 7)
	for i := range got {
		if got[i].Name != again[i].Name || len(got[i].Questions) != len(again[i].Questions) {
			t.Fatal("sample is not deterministic")
		}
	}
}

func TestOnlyQuestions(t *testing.T) {
	ds := Generate([]string{"orders", "logs"}, []int{30}, 1000)
	got := OnlyQuestions(ds, []string{ds[1].Questions[0].ID})
	if len(got) != 1 || len(got[0].Questions) != 1 || got[0].Questions[0].ID != ds[1].Questions[0].ID || len(ds[1].Questions) < 2 {
		t.Fatalf("OnlyQuestions kept %+v", got)
	}
	if len(OnlyQuestions(ds, nil)) != len(ds) {
		t.Fatal("an empty list must keep all")
	}
}
