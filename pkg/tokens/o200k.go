package tokens

import (
	"math"
	"unicode"
	"unicode/utf8"
)

// The o200k_base pre-tokenizer pattern, as used by tiktoken:
//
//	[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]*[\p{Ll}\p{Lm}\p{Lo}\p{M}]+(?i:'s|'t|'re|'ve|'m|'ll|'d)?
//	|[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]+[\p{Ll}\p{Lm}\p{Lo}\p{M}]*(?i:'s|'t|'re|'ve|'m|'ll|'d)?
//	|\p{N}{1,3}
//	| ?[^\s\p{L}\p{N}]+[\r\n/]*
//	|\s*[\r\n]+
//	|\s+(?!\S)
//	|\s+
//
// nextPiece implements it by hand with the backtracking semantics of the
// regexp2 engine tiktoken-go uses (alternatives tried in order, greedy
// quantifiers). Counts are exact and about 17x faster than the regex-based
// Go tokenizers. tiktoken-go is kept as a test-only reference: the
// differential tests and FuzzO200k in o200k_test.go run in CI, so any drift
// (for example a Go Unicode table update) fails the build.

// ASCII character classes for the fast path.
const (
	cUpper = 1 << iota
	cLower
	cDigit
	cSpace
	cNewline // \r or \n (also cSpace)
)

var asciiClass = func() (t [utf8.RuneSelf]uint8) {
	for c := 'A'; c <= 'Z'; c++ {
		t[c] = cUpper
	}
	for c := 'a'; c <= 'z'; c++ {
		t[c] = cLower
	}
	for c := '0'; c <= '9'; c++ {
		t[c] = cDigit
	}
	for _, c := range "\t\v\f " {
		t[c] = cSpace
	}
	t['\r'], t['\n'] = cSpace|cNewline, cSpace|cNewline
	return t
}()

// dec decodes the rune at byte offset i (i < len(s)).
func dec(s string, i int) (rune, int) {
	if c := s[i]; c < utf8.RuneSelf {
		return rune(c), 1
	}
	return utf8.DecodeRuneInString(s[i:])
}

// isUpperish: [\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]
func isUpperish(r rune) bool {
	if r < utf8.RuneSelf {
		return asciiClass[r]&cUpper != 0
	}
	return unicode.In(r, unicode.Lu, unicode.Lt, unicode.Lm, unicode.Lo, unicode.M)
}

// isLowerish: [\p{Ll}\p{Lm}\p{Lo}\p{M}]
func isLowerish(r rune) bool {
	if r < utf8.RuneSelf {
		return asciiClass[r]&cLower != 0
	}
	return unicode.In(r, unicode.Ll, unicode.Lm, unicode.Lo, unicode.M)
}

// isBoth: in both classes above, i.e. [\p{Lm}\p{Lo}\p{M}].
func isBoth(r rune) bool {
	return r >= utf8.RuneSelf && unicode.In(r, unicode.Lm, unicode.Lo, unicode.M)
}

// isLetterOrMark reports whether r can start the letter alternatives' body.
func isLetterOrMark(r rune) bool {
	if r < utf8.RuneSelf {
		return asciiClass[r]&(cUpper|cLower) != 0
	}
	return unicode.IsLetter(r) || unicode.Is(unicode.M, r)
}

func isLetter(r rune) bool {
	if r < utf8.RuneSelf {
		return asciiClass[r]&(cUpper|cLower) != 0
	}
	return unicode.IsLetter(r)
}

func isNumber(r rune) bool {
	if r < utf8.RuneSelf {
		return asciiClass[r]&cDigit != 0
	}
	return unicode.IsNumber(r)
}

func isSpace(r rune) bool {
	if r < utf8.RuneSelf {
		return asciiClass[r]&cSpace != 0
	}
	return unicode.IsSpace(r)
}

func isNewline(r rune) bool { return r == '\r' || r == '\n' }

// isLead: [^\r\n\p{L}\p{N}]
func isLead(r rune) bool { return !isNewline(r) && !isLetter(r) && !isNumber(r) }

// isPunct: [^\s\p{L}\p{N}]
func isPunct(r rune) bool { return !isSpace(r) && !isLetter(r) && !isNumber(r) }

// nextPiece returns the byte length of the pre-token starting at s[i:]
// (always at least one rune). s must be valid UTF-8.
func nextPiece(s string, i int) int {
	r0, n0 := dec(s, i)
	// The letter alternatives need a letter or mark at i, or a lead rune
	// at i followed by one.
	letters := isLetterOrMark(r0)
	if !letters && isLead(r0) && i+n0 < len(s) {
		r1, _ := dec(s, i+n0)
		letters = isLetterOrMark(r1)
	}
	if letters {
		if e := matchLetters(s, i, r0, n0, true); e > 0 {
			return e - i
		}
		if e := matchLetters(s, i, r0, n0, false); e > 0 {
			return e - i
		}
	}
	// \p{N}{1,3}
	if isNumber(r0) {
		j := i + n0
		for k := 1; k < 3 && j < len(s); k++ {
			r, n := dec(s, j)
			if !isNumber(r) {
				break
			}
			j += n
		}
		return j - i
	}
	// ` ?[^\s\p{L}\p{N}]+[\r\n/]*`
	j := i
	if r0 == ' ' {
		j++
	}
	if j < len(s) {
		if r, _ := dec(s, j); isPunct(r) {
			for j < len(s) {
				r, n := dec(s, j)
				if !isPunct(r) {
					break
				}
				j += n
			}
			for j < len(s) && (s[j] == '\r' || s[j] == '\n' || s[j] == '/') {
				j++
			}
			return j - i
		}
	}
	// Whitespace alternatives over the run [i, end).
	if !isSpace(r0) {
		// Unreachable for valid input: every rune is a letter, number,
		// whitespace or punctuation. Consume one rune to make progress.
		return n0
	}
	end, lastNL, prev, runes := i, -1, i, 0
	for end < len(s) {
		r, n := dec(s, end)
		if !isSpace(r) {
			break
		}
		if isNewline(r) {
			lastNL = end
		}
		prev, end, runes = end, end+n, runes+1
	}
	// `\s*[\r\n]+`: \s* backs off to the last newline in the run, then
	// [\r\n]+ takes exactly that one (no newline follows it in the run).
	if lastNL >= 0 {
		return lastNL + 1 - i
	}
	// `\s+(?!\S)`: the whole run at end of text, else all but the last
	// rune so the next pre-token can take it as its leading space.
	if end == len(s) || runes == 1 {
		return end - i // runes == 1: falls through to `\s+`
	}
	return prev - i
}

// matchLetters implements the first or second alternative:
//
//	[^\r\n\p{L}\p{N}]? U* L+ contraction?    (first)
//	[^\r\n\p{L}\p{N}]? U+ L* contraction?    (second)
//
// with U = [\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}] and L = [\p{Ll}\p{Lm}\p{Lo}\p{M}].
// It returns the end offset of the match, or 0.
func matchLetters(s string, i int, r0 rune, n0 int, first bool) int {
	try := func(p int) int {
		// Maximal U run [p, q0), remembering its last rune that is also in L.
		q0, lastBoth := p, -1
		for q0 < len(s) {
			r, n := dec(s, q0)
			if !isUpperish(r) {
				break
			}
			if isBoth(r) {
				lastBoth = q0
			}
			q0 += n
		}
		q := q0 // where L starts
		if first {
			// U* backs off until L+ can match at least one rune.
			if q0 < len(s) {
				if r, _ := dec(s, q0); isLowerish(r) {
					goto lower
				}
			}
			if lastBoth < 0 {
				return 0
			}
			q = lastBoth
		} else if q0 == p {
			return 0
		}
	lower:
		e := q
		for e < len(s) {
			r, n := dec(s, e)
			if !isLowerish(r) {
				break
			}
			e += n
		}
		return e + contraction(s, e)
	}
	if isLead(r0) && i+n0 < len(s) {
		if e := try(i + n0); e > 0 {
			return e
		}
	}
	return try(i)
}

// contraction matches (?i:'s|'t|'re|'ve|'m|'ll|'d) at s[e:] and returns its
// length in bytes, or 0.
func contraction(s string, e int) int {
	if e+1 >= len(s) || s[e] != '\'' {
		return 0
	}
	c1, n1 := dec(s, e+1)
	switch unicode.ToLower(c1) {
	case 's', 't', 'm', 'd':
		return 1 + n1
	}
	if e+1+n1 >= len(s) {
		return 0
	}
	c2, n2 := dec(s, e+1+n1)
	c1, c2 = unicode.ToLower(c1), unicode.ToLower(c2)
	if c1 == 'r' && c2 == 'e' || c1 == 'v' && c2 == 'e' || c1 == 'l' && c2 == 'l' {
		return 1 + n1 + n2
	}
	return 0
}

// countO200k returns the o200k_base token count of s, which must be valid
// UTF-8.
func countO200k(s string, ranks map[string]int) int {
	n := 0
	for i := 0; i < len(s); {
		k := nextPiece(s, i)
		n += bpeCount(s[i:i+k], ranks)
		i += k
	}
	return n
}

// bpeCount returns the number of tokens byte-pair merging produces for
// piece (the same merge order as tiktoken).
func bpeCount(piece string, ranks map[string]int) int {
	if len(piece) <= 1 {
		return len(piece)
	}
	if _, ok := ranks[piece]; ok {
		return 1
	}
	type part struct{ start, rank int }
	parts := make([]part, len(piece)+1)
	for i := range parts {
		parts[i] = part{i, math.MaxInt}
	}
	rank := func(i, skip int) int {
		if i+skip+2 < len(parts) {
			if r, ok := ranks[piece[parts[i].start:parts[i+skip+2].start]]; ok {
				return r
			}
		}
		return math.MaxInt
	}
	for i := 0; i < len(parts)-2; i++ {
		parts[i].rank = rank(i, 0)
	}
	for len(parts) > 1 {
		minRank, minIdx := math.MaxInt, -1
		for i := 0; i < len(parts)-1; i++ {
			if parts[i].rank < minRank {
				minRank, minIdx = parts[i].rank, i
			}
		}
		if minIdx < 0 {
			break
		}
		i := minIdx
		parts[i].rank = rank(i, 1)
		if i > 0 {
			parts[i-1].rank = rank(i-1, 1)
		}
		parts = append(parts[:i+1], parts[i+2:]...)
	}
	return len(parts) - 1
}
