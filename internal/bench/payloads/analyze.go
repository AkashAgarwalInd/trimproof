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
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toonx"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

// Codecs are the gateway codecs each payload is judged against.
var Codecs = []string{"toonx", "toon", "tabular"}

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
		o.PrimerTokens = est.Estimate(codec.PrimerFor(c, [][]byte{b.Encoded}), Model)
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

// Result is the analysis of one stored payload.
type Result struct {
	Entry
	AsSent map[string]Outcome // by codec
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
	fmt.Fprintf(w, "Each response body is judged by `policy.Decide` exactly as sent, as one tool result, under the default route policy with the codec ENABLED: strict schema, minimum %d tokens, minimum net savings %.0f%% with the primer counted. Token counts are o200k. Net savings are (JSON − encoded − primer) / JSON against compact canonical JSON.\n",
		p.MinPayloadTokens, 100*p.MinNetSavings)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Overall")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "| | %s |\n|---|%s\n", strings.Join(Codecs, " | "), strings.Repeat("---:|", len(Codecs)))
	row := func(label string, f func(c string) string) {
		cells := make([]string, len(Codecs))
		for i, c := range Codecs {
			cells[i] = f(c)
		}
		fmt.Fprintf(w, "| %s | %s |\n", label, strings.Join(cells, " | "))
	}
	row("payloads eligible", func(c string) string { return share(rs, c) })
	row("median net savings, eligible payloads", func(c string) string { return pctOr(median(savings(rs, c))) })
	row("token-weighted net savings, eligible payloads", func(c string) string { return pctOr(weighted(rs, c, false)) })
	row("token-weighted net savings, all payloads (ineligible ones save 0)", func(c string) string { return pctOr(weighted(rs, c, true)) })
	row("median savings vs pretty-printed JSON, eligible payloads", func(c string) string { return pctOr(median(prettySavings(rs, c))) })
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## By API")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "| API | payloads | %s |\n|---|---:|%s\n", strings.Join(Codecs, " eligible | ")+" eligible", strings.Repeat("---:|", len(Codecs)))
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
		cells := make([]string, len(Codecs))
		for i, c := range Codecs {
			cells[i] = fmt.Sprint(count(g, c))
		}
		fmt.Fprintf(w, "| %s | %d | %s |\n", a, len(g), strings.Join(cells, " | "))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Per payload")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Each codec cell is the net saving when the payload is eligible (**bold**), else the net saving where encoding was attempted, then the rejecting gate.")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "| API | payload | KB | JSON tok | %s |\n|---|---|---:|---:|%s\n", strings.Join(Codecs, " | "), strings.Repeat("---|", len(Codecs)))
	for _, r := range rs {
		cells := make([]string, len(Codecs))
		jt := 0
		for i, c := range Codecs {
			o := r.AsSent[c]
			jt = max(jt, o.JSONTokens)
			cells[i] = outcomeCell(o)
		}
		fmt.Fprintf(w, "| %s | %s | %.1f | %s | %s |\n", r.API, r.Name, float64(r.Bytes)/1024, tok(jt), strings.Join(cells, " | "))
	}
}

func outcomeCell(o Outcome) string {
	if o.Eligible {
		return "**" + pctOr(o.Savings) + "**"
	}
	s := gateCell(o)
	if !math.IsNaN(o.Savings) {
		s = pctOr(o.Savings) + " · " + s
	}
	return s
}

func count(rs []Result, c string) int {
	n := 0
	for _, r := range rs {
		if r.AsSent[c].Eligible {
			n++
		}
	}
	return n
}

func share(rs []Result, c string) string {
	if len(rs) == 0 {
		return "—"
	}
	n := count(rs, c)
	return fmt.Sprintf("%d / %d (%.0f%%)", n, len(rs), 100*float64(n)/float64(len(rs)))
}

func savings(rs []Result, c string) []float64 {
	var xs []float64
	for _, r := range rs {
		if o := r.AsSent[c]; o.Eligible {
			xs = append(xs, o.Savings)
		}
	}
	return xs
}

// weighted is the token-weighted net saving over eligible payloads, or with
// all set over every payload that reached the size gate, where an
// ineligible payload is sent as JSON and saves nothing.
func weighted(rs []Result, c string, all bool) float64 {
	var j, e int
	for _, r := range rs {
		o := r.AsSent[c]
		switch {
		case o.Eligible:
			j += o.JSONTokens
			e += o.EncTokens + o.PrimerTokens
		case all:
			n := r.JSONTokens()
			j += n
			e += n
		}
	}
	if j == 0 {
		return math.NaN()
	}
	return float64(j-e) / float64(j)
}

// JSONTokens is the payload's compact canonical JSON size in tokens, from
// whichever codec measured it; 0 when no codec got past the structural gate.
func (r Result) JSONTokens() int {
	n := 0
	for _, o := range r.AsSent {
		n = max(n, o.JSONTokens)
	}
	return n
}

func prettySavings(rs []Result, c string) []float64 {
	var xs []float64
	for _, r := range rs {
		if o := r.AsSent[c]; o.Eligible && o.PrettyTokens > 0 {
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
