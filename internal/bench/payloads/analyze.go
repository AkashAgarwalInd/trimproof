package payloads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/tabular"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toon"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

// Codecs are the gateway codecs each payload is judged against.
var Codecs = []string{"toon", "tabular"}

// Model names the o200k tokenizer; uncalibrated, so counts are raw o200k.
const Model = "gpt-4o"

// Outcome is policy.Decide's verdict on one payload sent as one tool result,
// under the default route policy with the codec ENABLED.
type Outcome struct {
	Eligible     bool
	Gate         string // rejecting gate, "none" when eligible
	Reason       string
	JSONTokens   int // compact canonical JSON, the Gate 4 baseline
	EncTokens    int
	PrimerTokens int
	// Savings is (JSON − encoded − primer) / JSON, NaN when the payload
	// never reached encoding.
	Savings float64
	// PrettyTokens counts the payload pretty-printed with a 2-space indent,
	// for context; 0 when it did not parse.
	PrettyTokens int
}

// Evaluate runs raw through the gateway's four gates for codecName.
func Evaluate(raw []byte, codecName string, est tokens.Estimator) (Outcome, error) {
	p := policy.Defaults()
	p.Codec, p.State = codecName, policy.Enabled
	d, err := policy.Decide(p, [][]byte{raw}, est, Model)
	if err != nil {
		return Outcome{}, err
	}
	b := d.Blocks[0]
	o := Outcome{Eligible: b.Transform, Gate: b.FailedGate.String(), Reason: b.Reason,
		JSONTokens: b.JSONTokens, EncTokens: b.EncTokens, Savings: math.NaN()}
	if c, ok := codec.Get(codecName); ok {
		o.PrimerTokens = est.Estimate(c.Primer(), Model)
	}
	if b.JSONTokens > 0 && b.EncTokens > 0 {
		o.Savings = float64(b.JSONTokens-b.EncTokens-o.PrimerTokens) / float64(b.JSONTokens)
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, raw, "", "  ") == nil {
		o.PrettyTokens = est.Estimate(pretty.String(), Model)
	}
	return o, nil
}

// Inner is the largest array of objects inside a top-level object, as a
// client would send it if it unwrapped the response before returning it as
// a tool result.
type Inner struct {
	Path string // dotted key path, e.g. "message.items"
	Rows int
	JSON []byte
}

// InnerArray finds the largest (by bytes) non-empty array of objects up to
// three object levels below a top-level object. ok is false when raw is not
// an object or holds no such array.
func InnerArray(raw []byte) (in Inner, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return in, false
	}
	if _, isObj := v.(map[string]any); !isObj {
		return in, false
	}
	var walk func(v any, path string, depth int)
	walk = func(v any, path string, depth int) {
		switch t := v.(type) {
		case map[string]any:
			if depth == 3 {
				return
			}
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				p := k
				if strings.ContainsAny(k, ". ") {
					p = fmt.Sprintf("[%q]", k)
				}
				if path != "" && p[0] != '[' {
					p = path + "." + p
				} else {
					p = path + p
				}
				walk(t[k], p, depth+1)
			}
		case []any:
			if len(t) == 0 || !allObjects(t) {
				return
			}
			if b, err := json.Marshal(t); err == nil && len(b) > len(in.JSON) {
				in = Inner{Path: path, Rows: len(t), JSON: b}
			}
		}
	}
	walk(v, "", 0)
	return in, in.JSON != nil
}

func allObjects(xs []any) bool {
	for _, x := range xs {
		if _, ok := x.(map[string]any); !ok {
			return false
		}
	}
	return true
}

// Result is the analysis of one stored payload.
type Result struct {
	Entry
	AsSent map[string]Outcome // by codec
	Inner  *Inner             // set when the top level is an object wrapping an array of objects
	InnerO map[string]Outcome // by codec, for Inner
}

// Results analyzes every payload recorded in dir's manifest, in manifest
// order.
func Results(dir string, est tokens.Estimator) (Manifest, []Result, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return m, nil, err
	}
	var rs []Result
	for _, e := range m.Entries {
		if e.File == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.File))
		if err != nil {
			return m, nil, err
		}
		r, err := analyzeOne(e, raw, est)
		if err != nil {
			return m, nil, fmt.Errorf("%s: %w", e.File, err)
		}
		rs = append(rs, r)
	}
	return m, rs, nil
}

func analyzeOne(e Entry, raw []byte, est tokens.Estimator) (Result, error) {
	r := Result{Entry: e, AsSent: map[string]Outcome{}}
	for _, c := range Codecs {
		o, err := Evaluate(raw, c, est)
		if err != nil {
			return r, err
		}
		r.AsSent[c] = o
	}
	if in, ok := InnerArray(raw); ok {
		r.Inner, r.InnerO = &in, map[string]Outcome{}
		for _, c := range Codecs {
			o, err := Evaluate(in.JSON, c, est)
			if err != nil {
				return r, err
			}
			r.InnerO[c] = o
		}
	}
	return r, nil
}

// Analyze writes a markdown report for the payloads stored in dir.
func Analyze(dir string, w io.Writer) error {
	m, rs, err := Results(dir, tokens.NewCalibrated(nil))
	if err != nil {
		return err
	}
	WriteReport(w, m, rs)
	return nil
}

// WriteReport renders the analysis as markdown.
func WriteReport(w io.Writer, m Manifest, rs []Result) {
	p := policy.Defaults()
	fmt.Fprintln(w, "# Real API payloads through trimproof's gates")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Fetched %s from %d public, keyless endpoints; %d stored", m.FetchedAt.Format("2006-01-02 15:04 MST"), len(m.Entries), len(rs))
	var failed []string
	for _, e := range m.Entries {
		if e.File == "" {
			failed = append(failed, fmt.Sprintf("%s/%s (%s)", e.API, e.Name, e.Error))
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(w, ", %d failed: %s", len(failed), strings.Join(failed, "; "))
	}
	fmt.Fprintln(w, ".")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Each payload is judged by `policy.Decide`, as one tool result, under the default route policy with the codec ENABLED: strict schema, minimum %d tokens, minimum net savings %.0f%% with the primer counted. Token counts are o200k.\n",
		p.MinPayloadTokens, 100*p.MinNetSavings)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "\"As sent\" is the response body unchanged. \"Inner array\" applies only to responses whose top level is an object: it is the largest array of objects inside, as sent by a client that unwraps the response first. The gateway itself never unwraps.")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Overall")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| | toon | tabular |")
	fmt.Fprintln(w, "|---|---:|---:|")
	row := func(label string, f func(c string) string) {
		fmt.Fprintf(w, "| %s | %s | %s |\n", label, f("toon"), f("tabular"))
	}
	row("payloads eligible as sent", func(c string) string { return share(rs, c, false) })
	row("eligible as sent or via inner array", func(c string) string { return share(rs, c, true) })
	row("median net savings, eligible as sent", func(c string) string { return pctOr(median(savings(rs, c, false))) })
	row("mean net savings, eligible as sent", func(c string) string { return pctOr(mean(savings(rs, c, false))) })
	row("token-weighted net savings, eligible as sent", func(c string) string { return pctOr(weighted(rs, c, false)) })
	row("median net savings, eligible as sent or via inner array", func(c string) string { return pctOr(median(savings(rs, c, true))) })
	row("token-weighted net savings, eligible as sent or via inner array", func(c string) string { return pctOr(weighted(rs, c, true)) })
	row("median savings vs pretty-printed JSON, eligible as sent or via inner array", func(c string) string { return pctOr(median(prettySavings(rs, c))) })
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## By API")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| API | payloads | toon eligible | tabular eligible | toon eligible incl. inner array | median toon savings when eligible |")
	fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|")
	var apis []string
	byAPI := map[string][]Result{}
	for _, r := range rs {
		if _, seen := byAPI[r.API]; !seen {
			apis = append(apis, r.API)
		}
		byAPI[r.API] = append(byAPI[r.API], r)
	}
	for _, a := range apis {
		g := byAPI[a]
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %s |\n", a, len(g), count(g, "toon", false), count(g, "tabular", false),
			count(g, "toon", true), pctOr(median(savings(g, "toon", true))))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Per payload, as sent")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| API | payload | KB | toon | toon gate: reason | JSON tok | TOON tok | toon net | tabular | tabular gate: reason | tabular net |")
	fmt.Fprintln(w, "|---|---|---:|---|---|---:|---:|---:|---|---|---:|")
	for _, r := range rs {
		t, tb := r.AsSent["toon"], r.AsSent["tabular"]
		fmt.Fprintf(w, "| %s | %s | %.1f | %s | %s | %s | %s | %s | %s | %s | %s |\n", r.API, r.Name, float64(r.Bytes)/1024,
			yes(t.Eligible), gateCell(t), tok(t.JSONTokens), tok(t.EncTokens), pctOr(t.Savings),
			yes(tb.Eligible), gateCell(tb), pctOr(tb.Savings))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Inner arrays (top-level objects only)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| API | payload | path | rows | toon | toon gate: reason | JSON tok | TOON tok | toon net | tabular | tabular gate: reason | tabular net |")
	fmt.Fprintln(w, "|---|---|---|---:|---|---|---:|---:|---:|---|---|---:|")
	for _, r := range rs {
		if r.Inner == nil {
			continue
		}
		t, tb := r.InnerO["toon"], r.InnerO["tabular"]
		fmt.Fprintf(w, "| %s | %s | `%s` | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n", r.API, r.Name, r.Inner.Path, r.Inner.Rows,
			yes(t.Eligible), gateCell(t), tok(t.JSONTokens), tok(t.EncTokens), pctOr(t.Savings),
			yes(tb.Eligible), gateCell(tb), pctOr(tb.Savings))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Net savings are (JSON − encoded − primer) / JSON against compact canonical JSON. A figure is shown wherever encoding was attempted, including payloads that then failed the net-savings gate.")
}

// outcome returns the result for codec c: as sent, or, when inner is set
// and the payload was not eligible as sent, via its inner array.
func outcome(r Result, c string, inner bool) (Outcome, bool) {
	if o := r.AsSent[c]; o.Eligible || !inner || r.Inner == nil {
		return o, o.Eligible
	}
	o := r.InnerO[c]
	return o, o.Eligible
}

func count(rs []Result, c string, inner bool) int {
	n := 0
	for _, r := range rs {
		if _, ok := outcome(r, c, inner); ok {
			n++
		}
	}
	return n
}

func share(rs []Result, c string, inner bool) string {
	if len(rs) == 0 {
		return "—"
	}
	n := count(rs, c, inner)
	return fmt.Sprintf("%d / %d (%.0f%%)", n, len(rs), 100*float64(n)/float64(len(rs)))
}

func savings(rs []Result, c string, inner bool) []float64 {
	var xs []float64
	for _, r := range rs {
		if o, ok := outcome(r, c, inner); ok {
			xs = append(xs, o.Savings)
		}
	}
	return xs
}

func weighted(rs []Result, c string, inner bool) float64 {
	var j, e int
	for _, r := range rs {
		if o, ok := outcome(r, c, inner); ok {
			j += o.JSONTokens
			e += o.EncTokens + o.PrimerTokens
		}
	}
	if j == 0 {
		return math.NaN()
	}
	return float64(j-e) / float64(j)
}

func prettySavings(rs []Result, c string) []float64 {
	var xs []float64
	for _, r := range rs {
		if o, ok := outcome(r, c, true); ok && o.PrettyTokens > 0 {
			xs = append(xs, float64(o.PrettyTokens-o.EncTokens-o.PrimerTokens)/float64(o.PrettyTokens))
		}
	}
	return xs
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	t := 0.0
	for _, x := range xs {
		t += x
	}
	return t / float64(len(xs))
}

func pctOr(x float64) string {
	if math.IsNaN(x) {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", 100*x)
}

func tok(n int) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprint(n)
}

func yes(b bool) string {
	if b {
		return "**yes**"
	}
	return "no"
}

func gateCell(o Outcome) string {
	if o.Eligible {
		return "—"
	}
	r := strings.ReplaceAll(o.Reason, "|", "\\|")
	if len(r) > 60 {
		r = r[:57] + "..."
	}
	return o.Gate + ": " + r
}
