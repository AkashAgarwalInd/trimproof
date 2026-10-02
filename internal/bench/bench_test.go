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
