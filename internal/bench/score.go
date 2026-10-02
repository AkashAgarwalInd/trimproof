package bench

import (
	"math/big"
	"regexp"
	"strings"
)

var (
	numRe   = regexp.MustCompile(`-?\d[\d,]*(?:\.\d+)?`)
	thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)
)

// Clean strips reasoning tags, markdown emphasis, quotes and trailing
// punctuation from a model reply.
func Clean(s string) string {
	s = thinkRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 && strings.Contains(strings.ToLower(s), "answer") {
		s = strings.TrimSpace(s[i+1:]) // "Reasoning...\nAnswer: x" -> last line
	}
	s = strings.TrimPrefix(s, "Answer:")
	s = strings.TrimPrefix(s, "answer:")
	s = strings.Trim(s, " \t\r\n*`\"'.")
	return s
}

// Correct scores a reply against the ground truth.
func Correct(q Question, reply string) bool {
	if q.Check != nil {
		return q.Check(reply)
	}
	got := Clean(reply)
	if q.Numeric {
		want, ok := new(big.Rat).SetString(q.Answer)
		if !ok {
			return false
		}
		if r, ok := new(big.Rat).SetString(strings.NewReplacer(",", "", "$", "", "€", "", "£", "").Replace(got)); ok {
			return r.Cmp(want) == 0
		}
		// Fall back to the last number in a short reply ("Order 10012: 45.5").
		if len(got) > 80 {
			return false
		}
		ms := numRe.FindAllString(got, -1)
		if len(ms) == 0 {
			return false
		}
		r, ok := new(big.Rat).SetString(strings.ReplaceAll(ms[len(ms)-1], ",", ""))
		return ok && r.Cmp(want) == 0
	}
	g, w := strings.ToLower(got), strings.ToLower(q.Answer)
	if g == w {
		return true
	}
	// Accept the exact value embedded in a short reply.
	return len(g) <= len(w)+30 && strings.Contains(g, w)
}
