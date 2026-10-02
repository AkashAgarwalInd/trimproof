package tokens

import (
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	tiktoken "github.com/pkoukk/tiktoken-go"
	loader "github.com/pkoukk/tiktoken-go-loader"
)

// The reference implementation: tiktoken-go's regex pre-tokenizer.
var (
	refOnce sync.Once
	ref     *tiktoken.Tiktoken
)

func reference(t testing.TB) *tiktoken.Tiktoken {
	refOnce.Do(func() {
		tiktoken.SetBpeLoader(loader.NewOfflineLoader())
		var err error
		if ref, err = tiktoken.GetEncoding("o200k_base"); err != nil {
			t.Fatal(err)
		}
	})
	return ref
}

var shared BPE

// checkSame fails when the fast counter disagrees with tiktoken, showing
// the first differing pre-token.
func checkSame(t testing.TB, s string) {
	t.Helper()
	want := len(reference(t).EncodeOrdinary(s))
	if got := shared.Count(s); got != want {
		var pieces []string
		if utf8.ValidString(s) {
			for i := 0; i < len(s); {
				k := nextPiece(s, i)
				pieces = append(pieces, s[i:i+k])
				i += k
			}
		}
		t.Fatalf("Count(%q) = %d, tiktoken = %d\npieces: %q", s, got, want, pieces)
	}
}

func TestO200kMatchesTiktoken(t *testing.T) {
	for _, s := range []string{
		"", " ", "  ", "\n", "\r\n", " \n ", "\t\t", "a", "A", "1", "-",
		"hello world", "Hello World", "HELLO world", "  hello", "hello  ", "hello\n\nworld",
		"I'm sure they'll say it's what we've done; they'd", "DON'T STOP", "rock'n'roll", "o'",
		"1234567", "3.14159", "-42", "1e10", "$1,234.56", "50%",
		`{"id":1,"name":"Alice","tags":["a","b"],"price":12.5,"ok":true,"none":null}`,
		"[{\"order_id\":10001,\"status\":\"refunded\"}]",
		"orders[2]{id,status}:\n  1,paid\n  2,refunded\n",
		"path/to/file.go\r\n//comment\n", "a\\nb", "\"quoted\"", "x  \n  y", "  \n\n  \t  z",
		"CamelCaseIdentifier", "snake_case_identifier", "XMLHttpRequest", "iPhone",
		"naïve café résumé", "Straße", "ǅungla", "ⅫⅪ", "²³¹", "١٢٣٤", "٣.١٤",
		"日本語のテキスト", "中文文本，标点。", "한국어 텍스트", "मनुष्य हिन्दी", "العربية مع تشكيل: مُحَمَّد",
		"emoji 😀👍🏽 family 👨‍👩‍👧", " nbsp em　ideographic", "zero​width",
		"é combining", "́leading mark", " ́", "á̂b", "ʰello", "ǈx",
		"<|endoftext|> special", "tab\tseparated\tvalues", "\x85next line", "trailing space ",
		"invalid \xff\xfe bytes", "\xc3", "mixed \xe2\x82 cut",
	} {
		checkSame(t, s)
	}
}

// TestO200kRandom compares on random strings drawn from a palette chosen to
// stress every alternative's boundaries.
func TestO200kRandom(t *testing.T) {
	palette := []rune("aAzZ09 '\t\n\r/.,-_{}[]\"\\:sStTrReEvVmMlLdD" +
		"éÉßǅʰ́̂ⅫⅪ²١日本ア한मिि😀  　​\u0085")
	r := rand.New(rand.NewPCG(1, 2))
	n := 20000
	if testing.Short() {
		n = 2000
	}
	for i := 0; i < n; i++ {
		var b strings.Builder
		for j := r.IntN(24); j >= 0; j-- {
			b.WriteRune(palette[r.IntN(len(palette))])
		}
		checkSame(t, b.String())
	}
}

func FuzzO200k(f *testing.F) {
	for _, s := range []string{"hello world", `{"a":[1,2.5,"x"]}`, "I'm \n\n  ok", "日本 é 😀", "\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { checkSame(t, s) })
}

func BenchmarkCount(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; sb.Len() < 100<<10; i++ {
		sb.WriteString(`{"order_id":10001,"customer":"Customer 17","status":"refunded","amount":12.5,"note":"left at the front door"},`)
	}
	sb.WriteString("]")
	s := sb.String()
	b.Run("fast", func(b *testing.B) {
		shared.Count("warm up") // load the vocabulary outside the timer
		b.SetBytes(int64(len(s)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			shared.Count(s)
		}
	})
	b.Run("tiktoken", func(b *testing.B) {
		b.SetBytes(int64(len(s)))
		enc := reference(b)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			enc.EncodeOrdinary(s)
		}
	})
}
