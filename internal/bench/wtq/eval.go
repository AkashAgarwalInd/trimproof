package wtq

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// This file ports the official WikiTableQuestions evaluator (evaluator.py,
// v1.0.2). A prediction is a set of values; it is correct when it has as
// many distinct values as the target and every target value matches one of
// them. A target matches a prediction when their normalized strings are
// equal, or both are numbers within 1e-6, or both are equal dates.

// Correct scores reply against raw target answers, using them as their own
// canonical forms. Item.Correct also uses the dataset's CoreNLP forms.
func Correct(answers []string, reply string) bool { return correct(answers, answers, reply) }

func correct(answers, canon []string, reply string) bool {
	if len(canon) != len(answers) {
		canon = answers
	}
	targets := make([]value, len(answers))
	for i := range answers {
		targets[i] = toValue(answers[i], canon[i])
	}
	targets = dedupe(targets)

	// Quotes can be part of an answer (Swingin', 5h 29' 10"), so the reply
	// is tried as is and then with surrounding quotes removed.
	got := clean(reply)
	return matches(targets, answers, got) || matches(targets, answers, strings.Trim(got, `"' `))
}

func matches(targets []value, answers []string, got string) bool {
	if got == "" {
		return false
	}
	if check(targets, predict([]string{got})) {
		return true
	}
	if len(targets) > 1 {
		for _, sep := range []string{"|", "\n", ", ", ","} {
			if strings.Contains(got, sep) && check(targets, predict(strings.Split(got, sep))) {
				return true
			}
		}
		return false
	}
	return embedded(targets[0], answers[0], got)
}

// embedded accepts a short reply that states the single target value
// inside a sentence ("The answer is Italy"). Long replies are not searched,
// so a model can't pass by listing candidates.
func embedded(t value, raw, got string) bool {
	if len(got) > len(raw)+30 {
		return false
	}
	if t.kind == number {
		nums := numRe.FindAllString(got, -1)
		if len(nums) == 0 {
			return false
		}
		v := toPrediction(nums[len(nums)-1])
		return v.kind == number && math.Abs(v.amount-t.amount) < 1e-6
	}
	g, w := " "+normalize(got)+" ", normalize(raw)
	if w == "" {
		return false
	}
	for i := strings.Index(g, w); i >= 0; {
		end := i + len(w)
		if !isWord(g[i-1]) && (end >= len(g) || !isWord(g[end])) {
			return true
		}
		j := strings.Index(g[i+1:], w)
		if j < 0 {
			break
		}
		i += 1 + j
	}
	return false
}

func isWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c >= 0x80
}

var (
	thinkRe     = regexp.MustCompile(`(?s)<think>.*?</think>`)
	numRe       = regexp.MustCompile(`-?\d[\d,]*(?:\.\d+)?`)
	thousandsRe = regexp.MustCompile(`^-?\d{1,3}(?:,\d{3})+(?:\.\d+)?$`)
	spaceRe     = regexp.MustCompile(`\s+`)
)

// clean strips reasoning tags, a leading "Answer:" and surrounding
// markdown from a reply, as the synthetic benchmark does.
func clean(s string) string {
	s = thinkRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 && strings.Contains(strings.ToLower(s), "answer") {
		s = strings.TrimSpace(s[i+1:])
	}
	s = strings.TrimPrefix(s, "Answer:")
	s = strings.TrimPrefix(s, "answer:")
	return strings.Trim(s, " \t\r\n*`")
}

type kind int

const (
	str kind = iota
	number
	date
)

type value struct {
	kind   kind
	norm   string
	amount float64
	ymd    [3]int
}

func (v value) key() string {
	switch v.kind {
	case number:
		return "n" + strconv.FormatFloat(v.amount, 'g', -1, 64)
	case date:
		return "d" + strconv.Itoa(v.ymd[0]) + "-" + strconv.Itoa(v.ymd[1]) + "-" + strconv.Itoa(v.ymd[2])
	}
	return "s" + v.norm
}

func (t value) match(p value) bool {
	if t.norm == p.norm {
		return true
	}
	switch {
	case t.kind == number && p.kind == number:
		return math.Abs(t.amount-p.amount) < 1e-6
	case t.kind == date && p.kind == date:
		return t.ymd == p.ymd
	}
	return false
}

func check(targets, preds []value) bool {
	if len(targets) != len(preds) {
		return false
	}
	for _, t := range targets {
		ok := false
		for _, p := range preds {
			if t.match(p) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func predict(items []string) []value {
	var vs []value
	for _, it := range items {
		it = strings.TrimSpace(it)
		it = strings.TrimLeft(it, "-*• ")
		if it = strings.TrimSpace(it); it != "" {
			vs = append(vs, toPrediction(it))
		}
	}
	return dedupe(vs)
}

// toPrediction reads a predicted item. Unlike the official evaluator it
// also accepts thousands separators ("1,000"), which models write often.
func toPrediction(s string) value {
	if thousandsRe.MatchString(strings.TrimSpace(s)) {
		return toValue(s, strings.ReplaceAll(s, ",", ""))
	}
	return toValue(s, s)
}

// toValue is the evaluator's to_value: canon decides the type, the raw
// string the normalized form.
func toValue(raw, canon string) value {
	if amount, ok := parseNumber(canon); ok {
		return value{kind: number, norm: normalize(raw), amount: amount}
	}
	if ymd, ok := parseDate(canon); ok {
		if ymd[1] == -1 && ymd[2] == -1 {
			return value{kind: number, norm: normalize(raw), amount: float64(ymd[0])}
		}
		return value{kind: date, norm: normalize(raw), ymd: ymd}
	}
	return value{kind: str, norm: normalize(raw)}
}

// parseNumber accepts what Python's int() or float() accept, minus
// infinities and NaN.
func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	l := strings.ToLower(s)
	if s == "" || strings.Contains(l, "x") || strings.Contains(l, "_") || strings.Contains(l, "inf") || strings.Contains(l, "nan") {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, false
	}
	return f, true
}

// parseDate reads yyyy-mm-dd, with xx (or xxxx) for unknown fields.
func parseDate(s string) ([3]int, bool) {
	parts := strings.Split(strings.ToLower(s), "-")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var ymd [3]int
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "xx" || (i == 0 && p == "xxxx") {
			ymd[i] = -1
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		ymd[i] = n
	}
	y, m, d := ymd[0], ymd[1], ymd[2]
	if y == -1 && m == -1 && d == -1 || m != -1 && (m < 1 || m > 12) || d != -1 && (d < 1 || d > 31) {
		return [3]int{}, false
	}
	return ymd, true
}

func dedupe(vs []value) []value {
	seen := map[string]bool{}
	out := vs[:0:0]
	for _, v := range vs {
		if k := v.key(); !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}

var (
	quoteRepl = strings.NewReplacer("‘", "'", "’", "'", "´", "'", "`", "'", "“", `"`, "”", `"`,
		"‐", "-", "‑", "-", "‒", "-", "–", "-", "—", "-", "−", "-")
)

// normalize is the evaluator's string normalization.
func normalize(x string) string {
	// Remove diacritics.
	var b strings.Builder
	for _, r := range norm.NFKD.String(x) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	x = quoteRepl.Replace(b.String())
	for {
		old := x
		x = stripCitations(strings.TrimSpace(x))
		x = stripParens(strings.TrimSpace(x))
		x = strings.TrimSpace(x)
		if len(x) >= 2 && x[0] == '"' && x[len(x)-1] == '"' && !strings.Contains(x[1:len(x)-1], `"`) {
			x = x[1 : len(x)-1]
		}
		if x == old {
			break
		}
	}
	x = strings.TrimSuffix(x, ".")
	return strings.TrimSpace(strings.ToLower(spaceRe.ReplaceAllString(x, " ")))
}

// stripCitations removes the evaluator's trailing citation run: the
// earliest suffix made only of [..] groups (digits only when the group
// starts the string) and the marks •♦†‡*#+.
func stripCitations(x string) string {
	ok := make([]bool, len(x)+1)
	ok[len(x)] = true
	for i := len(x) - 1; i >= 0; i-- {
		if x[i] == '[' {
			if j := strings.IndexByte(x[i+1:], ']'); j >= 0 {
				inner := x[i+1 : i+1+j]
				if i > 0 || inner != "" && strings.Trim(inner, "0123456789") == "" {
					ok[i] = ok[i+1+j+1]
				}
			}
			continue
		}
		if r, n := utf8.DecodeRuneInString(x[i:]); strings.ContainsRune("•♦†‡*#+", r) {
			ok[i] = ok[i+n]
		}
	}
	for i, k := range ok {
		if k {
			return x[:i]
		}
	}
	return x
}

// stripParens removes the evaluator's trailing " (...)" run: the earliest
// suffix, not at the start, made only of such groups.
func stripParens(x string) string {
	ok := make([]bool, len(x)+1)
	ok[len(x)] = true
	for i := len(x) - 2; i >= 0; i-- {
		if x[i] == ' ' && x[i+1] == '(' {
			if j := strings.IndexByte(x[i+2:], ')'); j >= 0 {
				ok[i] = ok[i+2+j+1]
			}
		}
	}
	for i := 1; i < len(ok); i++ {
		if ok[i] {
			return x[:i]
		}
	}
	return x
}
