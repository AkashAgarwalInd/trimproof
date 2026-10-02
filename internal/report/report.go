// Package report summarizes a gateway's state from its files: route
// policies, the promotion transition log, shadow evaluation samples and the
// audit log. It reads the same evidence the gateway's promoter uses, so the
// numbers match what the gateway decides on.
package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/audit"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/tabular" // policy files name codecs
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toon"
	"github.com/AkashAgarwalInd/trimproof/pkg/eval"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/server"
)

// Files names the gateway's files. Missing pair, transition and audit files
// are treated as empty.
type Files struct {
	Policies, Pairs, Transitions, Audit string
}

// Report is the whole summary.
type Report struct {
	Generated  time.Time       `json:"generated"`
	Thresholds eval.Thresholds `json:"thresholds"`
	Routes     []Route         `json:"routes"`
}

// Route is one route's summary.
type Route struct {
	TenantID      string `json:"tenant_id"`
	RouteID       string `json:"route_id"`
	PolicyVersion string `json:"policy_version,omitempty"`
	Codec         string `json:"codec,omitempty"`
	State         string `json:"state"`
	Configured    bool   `json:"configured"` // present in the policy file
	// Promotion evidence: samples since the route's last applied transition,
	// as the promoter counts them.
	Since    *time.Time  `json:"since,omitempty"`
	Samples  int         `json:"samples"`                  // all samples recorded since then, valid or not
	Legacy   int         `json:"legacy_samples,omitempty"` // two-arm samples from the earlier rule; not evidence
	Stats    *eval.Stats `json:"stats,omitempty"`
	Lower    float64     `json:"lower_bound"`
	Upper    float64     `json:"upper_bound"`
	NextLook int         `json:"next_look,omitempty"` // valid samples at the next decision
	// Outlook is what the promotion rule says on the current evidence. The
	// gateway acts on it only at the next look.
	Outlook     string            `json:"outlook,omitempty"`
	Transitions []eval.Transition `json:"transitions,omitempty"`
	Audit       *AuditSummary     `json:"audit,omitempty"`
}

// AuditSummary counts audited requests. The audit log is sampled at the
// route's audit_sample_rate (failures and fallbacks are always logged), so
// counts are of logged requests, not all traffic.
type AuditSummary struct {
	Requests       int            `json:"requests"`
	Encoded        int            `json:"encoded"`
	FellBack       int            `json:"fell_back"`
	Tier1Failures  int            `json:"tier1_failures"`
	ParseErrors    int            `json:"parse_errors"`
	Errors         int            `json:"errors"` // non-2xx responses
	GateRejections map[string]int `json:"gate_rejections,omitempty"`
	MeanEstSavings float64        `json:"mean_est_savings"` // over encoded requests
	InputTokens    map[string]int `json:"input_tokens,omitempty"`
}

// Build reads the files and summarizes every route found in any of them.
func Build(f Files, t eval.Thresholds) (*Report, error) {
	reg, err := server.LoadRegistry(f.Policies)
	if err != nil {
		return nil, err
	}
	configured := map[[2]string]bool{}
	for _, p := range reg.Routes() {
		configured[[2]string{p.TenantID, p.RouteID}] = true
	}
	// The same replay the gateway runs at start-up gives current states and
	// the start of each route's evidence window.
	applied, err := eval.ReplayTransitions(f.Transitions, reg)
	if err != nil {
		return nil, err
	}
	since := map[[2]string]time.Time{}
	for _, tr := range applied {
		since[[2]string{tr.TenantID, tr.RouteID}] = tr.Time
	}
	history, err := eval.ReadTransitions(f.Transitions)
	if err != nil {
		return nil, err
	}
	pairs, err := readPairs(f.Pairs)
	if err != nil {
		return nil, err
	}
	audits, err := readAudit(f.Audit)
	if err != nil {
		return nil, err
	}

	keys := map[[2]string]bool{}
	for k := range configured {
		keys[k] = true
	}
	byRoute := map[[2]string][]eval.EvaluationPair{}
	for _, p := range pairs {
		k := [2]string{p.TenantID, p.RouteID}
		byRoute[k] = append(byRoute[k], p)
		keys[k] = true
	}
	trs := map[[2]string][]eval.Transition{}
	for _, tr := range history {
		k := [2]string{tr.TenantID, tr.RouteID}
		trs[k] = append(trs[k], tr)
		keys[k] = true
	}
	aud := map[[2]string]*AuditSummary{}
	for _, e := range audits {
		k := [2]string{e.TenantID, e.RouteID}
		keys[k] = true
		s := aud[k]
		if s == nil {
			s = &AuditSummary{GateRejections: map[string]int{}, InputTokens: map[string]int{}}
			aud[k] = s
		}
		s.add(e)
	}

	r := &Report{Generated: time.Now().UTC(), Thresholds: t}
	for k := range keys {
		rt := reg.Lookup(k[0], k[1])
		p := rt.Policy
		out := Route{TenantID: k[0], RouteID: k[1], Configured: configured[k], Transitions: trs[k], Audit: aud[k]}
		if out.Configured {
			out.PolicyVersion, out.Codec, out.State = p.Version, p.Codec, p.State.String()
		} else {
			out.State = "not in policy file"
		}
		var samples []eval.EvaluationPair
		s := since[k]
		if !s.IsZero() {
			out.Since = &s
		}
		for _, sp := range byRoute[k] {
			if s.IsZero() || sp.Time.After(s) {
				samples = append(samples, sp)
			}
		}
		out.Samples = len(samples)
		for _, sp := range samples {
			if sp.Kind != eval.Paired {
				out.Legacy++
			}
		}
		if len(samples) > 0 {
			all := eval.ComputeStats(samples, 0, p.OutputPriceRatio)
			recent := eval.ComputeStats(samples, t.Window, p.OutputPriceRatio)
			out.Stats = &all
			out.Lower, out.Upper = all.Bounds(t.Z)
			if p.State == policy.Shadow {
				out.NextLook = t.NextLook(all.N)
			}
			if p.State == policy.Shadow || p.State == policy.Enabled {
				next, reason := eval.NextState(p.State, p.MinNetSavings, all, recent, t)
				if reason == "" {
					reason = "no change"
				}
				if next != p.State {
					reason = fmt.Sprintf("%s → %s: %s", p.State, next, reason)
				}
				out.Outlook = reason
			}
		}
		if out.Audit != nil {
			out.Audit.finish()
		}
		r.Routes = append(r.Routes, out)
	}
	sort.Slice(r.Routes, func(i, j int) bool {
		a, b := r.Routes[i], r.Routes[j]
		if a.TenantID != b.TenantID {
			return a.TenantID < b.TenantID
		}
		return a.RouteID < b.RouteID
	})
	return r, nil
}

func (s *AuditSummary) add(e audit.Entry) {
	s.Requests++
	if e.Sent == "encoded" {
		s.Encoded++
		s.MeanEstSavings += e.EstSavings
	}
	if e.FellBack {
		s.FellBack++
	}
	if e.Tier1 != nil && !e.Tier1.OK {
		s.Tier1Failures++
	}
	if e.ParseError != "" {
		s.ParseErrors++
	}
	if e.Status < 200 || e.Status > 299 {
		s.Errors++
	}
	for _, g := range e.Gates {
		if g.FailedGate != "" {
			s.GateRejections[g.FailedGate]++
		}
	}
	if e.Usage != nil {
		s.InputTokens[e.Sent] += e.Usage.InputTokens
	}
}

func (s *AuditSummary) finish() {
	if s.Encoded > 0 {
		s.MeanEstSavings /= float64(s.Encoded)
	}
}

func readPairs(path string) ([]eval.EvaluationPair, error) {
	if path == "" {
		return nil, nil
	}
	ps, err := eval.ReadPairs(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return ps, err
}

func readAudit(path string) ([]audit.Entry, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []audit.Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var e audit.Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// WriteJSON writes the report as indented JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteText writes a human-readable report.
func (r *Report) WriteText(w io.Writer) error {
	t := r.Thresholds
	fmt.Fprintf(w, "trimproof report  %s\n", r.Generated.Format(time.RFC3339))
	fmt.Fprintf(w, "promotion rule: looks at %d samples then every %d, bounds ±%.1f SE, ε=%.2f, max %d samples\n",
		t.NMin, t.LookEvery, t.Z, t.Epsilon, t.MaxSamples)
	if len(r.Routes) == 0 {
		fmt.Fprintln(w, "\nno routes")
	}
	for _, rt := range r.Routes {
		fmt.Fprintf(w, "\n%s/%s  state %s", rt.TenantID, rt.RouteID, rt.State)
		if rt.Codec != "" {
			fmt.Fprintf(w, "  codec %s", rt.Codec)
		}
		if rt.PolicyVersion != "" {
			fmt.Fprintf(w, "  policy %s", rt.PolicyVersion)
		}
		fmt.Fprintln(w)
		if rt.Since != nil {
			fmt.Fprintf(w, "  evidence since  %s (last transition)\n", rt.Since.Format(time.RFC3339))
		}
		if s := rt.Stats; s != nil {
			fmt.Fprintf(w, "  samples         %d valid of %d", s.N, rt.Samples)
			if rt.Legacy > 0 {
				fmt.Fprintf(w, " (%d two-arm samples from the earlier rule, not counted)", rt.Legacy)
			}
			if rt.NextLook > 0 {
				fmt.Fprintf(w, "; next look at %d", rt.NextLook)
			}
			fmt.Fprintln(w)
			if s.N > 0 {
				fmt.Fprintf(w, "  agreement       codec %.3f, noise floor %.3f, difference %+.3f [%+.3f, %+.3f]\n",
					s.AgreeTreatment, s.AgreeControl, s.Diff, rt.Lower, rt.Upper)
				fmt.Fprintf(w, "  savings         %.1f%% net (input %.1f%%, output %+.1f%%)", 100*s.MeasuredSavings, 100*s.InputSavings, 100*s.OutputChange)
				if s.LatencyRatio > 0 {
					fmt.Fprintf(w, ", codec latency ×%.2f", s.LatencyRatio)
				}
				fmt.Fprintln(w)
				if s.B+s.C > 0 {
					fmt.Fprintf(w, "  tier 1          %d codec-only failures vs %d JSON-only (McNemar p=%.3f)\n", s.B, s.C, s.McNemarP)
				}
			}
		} else if rt.State == "SHADOW" {
			fmt.Fprintln(w, "  samples         none yet")
		}
		if rt.Outlook != "" {
			fmt.Fprintf(w, "  outlook         %s\n", rt.Outlook)
		}
		for i, tr := range rt.Transitions {
			label := "  transitions     "
			if i > 0 {
				label = "                  "
			}
			if n := len(rt.Transitions); n > 5 && i < n-5 {
				if i == 0 {
					fmt.Fprintf(w, "%s(%d earlier)\n", label, n-5)
				}
				continue
			}
			fmt.Fprintf(w, "%s%s  %s → %s  %s\n", label, tr.Time.Format(time.RFC3339), tr.From, tr.To, tr.Reason)
		}
		if a := rt.Audit; a != nil {
			fmt.Fprintf(w, "  audit           %d logged requests, %d encoded", a.Requests, a.Encoded)
			if a.Encoded > 0 {
				fmt.Fprintf(w, " (mean est. savings %.1f%%)", 100*a.MeanEstSavings)
			}
			fmt.Fprintf(w, ", %d fallbacks, %d Tier 1 failures, %d errors\n", a.FellBack, a.Tier1Failures, a.Errors)
			if len(a.GateRejections) > 0 {
				var gs []string
				for g, n := range a.GateRejections {
					gs = append(gs, fmt.Sprintf("%s %d", g, n))
				}
				sort.Strings(gs)
				fmt.Fprintf(w, "  gate rejections %s (data blocks)\n", strings.Join(gs, ", "))
			}
		} else {
			fmt.Fprintln(w, "  audit           no entries yet (the audit log keeps a sample of requests, set by audit_sample_rate)")
		}
	}
	return nil
}
